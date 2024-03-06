package hostdb

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/rhp"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.uber.org/zap"
	"lukechampine.com/frand"
)

const (
	benchmarkInterval  = 2 * time.Hour
	benchmarkBatchSize = 1 << 26 // 64 MiB
)

// benchmarkHost runs an up/download benchmark on a host.
func (hdb *HostDB) benchmarkHost(host *HostDBEntry) {
	// Resolve the host's used subnets and update the timestamp if they
	// changed. We only update the timestamp if resolving the ipNets was
	// successful.
	ipNets, err := utils.LookupIPNets(host.NetAddress)
	if err == nil && !utils.EqualIPNets(ipNets, host.IPNets) {
		host.IPNets = ipNets
		host.LastIPChange = time.Now()
	}

	// Update historic interactions of the host if necessary.
	hdb.mu.Lock()
	hdb.updateHostHistoricInteractions(host)
	limits := hdb.priceLimits
	hdb.mu.Unlock()

	key := hdb.w.Key(host.Network)
	var height uint64
	if host.Network == "zen" {
		height = hdb.sZen.tip.Height
	} else {
		height = hdb.s.tip.Height
	}

	timestamp := time.Now()
	var success bool
	var ul, dl float64
	var ttfb time.Duration
	var errMsg string
	err = func() error {
		// Do some checks first.
		settings := host.Settings
		if (settings == rhpv2.HostSettings{}) {
			return errors.New("couldn't fetch host settings")
		}
		pt := host.PriceTable
		if (pt == rhpv3.HostPriceTable{}) {
			return errors.New("couldn't fetch price table")
		}
		err := checkGouging(&settings, &pt, limits)
		if err != nil {
			return err
		}

		// Create a context and set up its cancelling.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		connCloseChan := make(chan struct{})
		go func() {
			select {
			case <-hdb.tg.StopChan():
			case <-connCloseChan:
			}
			cancel()
		}()
		defer close(connCloseChan)

		h, _, _ := net.SplitHostPort(host.NetAddress)
		addr := net.JoinHostPort(h, host.Settings.SiaMuxPort)
		numSectors := benchmarkBatchSize / rhpv2.SectorSize
		var uploadCost, downloadCost types.Currency

		// Check if we have a contract with this host and if it has enough money in it.
		if host.Revision.WindowStart <= height ||
			host.Revision.ValidRenterPayout().Cmp(hdb.benchmarkCost(host)) < 0 {
			var rev rhpv2.ContractRevision
			var txnSet []types.Transaction
			err = rhp.WithTransportV2(ctx, settings.NetAddress, host.PublicKey, func(t *rhpv2.Transport) error {
				renterTxnSet, err := hdb.prepareContractFormation(host)
				if err != nil {
					return utils.AddContext(err, "couldn't prepare contract")
				}

				rev, txnSet, err = rhp.RPCFormContract(ctx, t, key, renterTxnSet)
				if err != nil {
					hdb.w.Release(renterTxnSet)
					return utils.AddContext(err, "couldn't form contract")
				}

				return nil
			})
			if err != nil {
				return err
			}

			if host.Network == "zen" {
				_, err := hdb.cmZen.AddPoolTransactions(txnSet)
				if err != nil {
					hdb.w.Release(txnSet)
					return utils.AddContext(err, "invalid transaction set")
				}
				hdb.syncerZen.BroadcastTransactionSet(txnSet)
			} else {
				_, err := hdb.cm.AddPoolTransactions(txnSet)
				if err != nil {
					hdb.w.Release(txnSet)
					return utils.AddContext(err, "invalid transaction set")
				}
				hdb.syncer.BroadcastTransactionSet(txnSet)
			}

			host.Revision = rev.Revision
			hdb.log.Info("successfully formed contract", zap.String("host", host.NetAddress), zap.Stringer("id", rev.Revision.ParentID))
		} else {
			// Fetch the latest revision.
			err = rhp.WithTransportV3(ctx, addr, host.PublicKey, func(t *rhpv3.Transport) error {
				rev, err := rhp.RPCLatestRevision(ctx, t, host.Revision.ParentID)
				if err != nil {
					return utils.AddContext(err, "unable to get latest revision")
				}
				host.Revision = rev
				return nil
			})
		}

		// Fetch a valid price table.
		err = rhp.WithTransportV3(ctx, addr, host.PublicKey, func(t *rhpv3.Transport) error {
			pt, err = rhp.RPCPriceTable(ctx, t, func(pt rhpv3.HostPriceTable) (rhpv3.PaymentMethod, error) {
				payment, ok := rhpv3.PayByContract(&host.Revision, pt.UpdatePriceTableCost, rhpv3.Account(key.PublicKey()), key)
				if !ok {
					return nil, errors.New("insufficient balance")
				}
				return &payment, nil
			})
			if err != nil {
				return utils.AddContext(err, "unable to get price table")
			}
			host.PriceTable = pt

			// Fund the account.
			uploadCost, _, _, err = rhp.UploadSectorCost(pt, host.Revision.WindowEnd)
			if err != nil {
				return utils.AddContext(err, "unable to estimate costs")
			}
			downloadCost, err = rhp.ReadSectorCost(pt, rhpv2.SectorSize)
			if err != nil {
				return utils.AddContext(err, "unable to estimate costs")
			}
			amount := uploadCost.Add(downloadCost).Mul64(uint64(numSectors))
			amount = amount.Add(pt.FundAccountCost)
			payment, ok := rhpv3.PayByContract(&host.Revision, amount, rhpv3.Account{}, key)
			if !ok {
				return errors.New("insufficient balance")
			}
			if err := rhp.RPCFundAccount(ctx, t, &payment, rhpv3.Account(key.PublicKey()), pt.UID); err != nil {
				return utils.AddContext(err, "unable to fund account")
			}

			return nil
		})
		if err != nil {
			return err
		}

		// Run an upload benchmark.
		var data [rhpv2.SectorSize]byte
		roots := make([]types.Hash256, numSectors)
		var start time.Time
		err = rhp.WithTransportV3(ctx, addr, host.PublicKey, func(t *rhpv3.Transport) error {
			start = time.Now()
			for i := 0; i < numSectors; i++ {
				frand.Read(data[:256])
				payment := rhpv3.PayByEphemeralAccount(rhpv3.Account(key.PublicKey()), uploadCost, host.PriceTable.HostBlockHeight+6, key)
				root, _, err := rhp.RPCAppendSector(ctx, t, key, pt, &host.Revision, &payment, &data)
				if err != nil {
					return utils.AddContext(err, "unable to upload sector")
				}
				roots[i] = root
			}
			return nil
		})
		if err != nil {
			return err
		}
		ul = float64(benchmarkBatchSize) / time.Since(start).Seconds()

		// Run a download benchmark.
		err = rhp.WithTransportV3(ctx, addr, host.PublicKey, func(t *rhpv3.Transport) error {
			start = time.Now()
			for i := 0; i < numSectors; i++ {
				payment := rhpv3.PayByEphemeralAccount(rhpv3.Account(key.PublicKey()), downloadCost, host.PriceTable.HostBlockHeight+6, key)
				buf := bytes.NewBuffer(data[:])
				_, _, err := rhp.RPCReadSector(ctx, t, buf, pt, &payment, 0, rhpv2.SectorSize, roots[i])
				if err != nil {
					return utils.AddContext(err, "unable to download sector")
				}
				if i == 0 {
					ttfb = time.Since(start)
				}
			}
			if err != nil {
				return err
			}
			dl = float64(benchmarkBatchSize) / time.Since(start).Seconds()

			return nil
		})
		return err
	}()
	if err != nil && strings.Contains(err.Error(), "canceled") {
		// Shutting down.
		return
	}
	if err != nil && strings.Contains(err.Error(), "insufficient balance") {
		// Not the host's fault.
		hdb.mu.Lock()
		delete(hdb.scanMap, host.PublicKey)
		hdb.benchmarking = false
		hdb.mu.Unlock()
		return
	}
	if err == nil {
		success = true
		hdb.IncrementSuccessfulInteractions(host)
	} else {
		errMsg = err.Error()
		hdb.IncrementFailedInteractions(host)
	}

	benchmark := HostBenchmark{
		Timestamp:     timestamp,
		Success:       success,
		Error:         errMsg,
		UploadSpeed:   ul,
		DownloadSpeed: dl,
		TTFB:          ttfb,
	}
	if host.Network == "zen" {
		err = hdb.sZen.updateBenchmarks(host, benchmark)
	} else {
		err = hdb.s.updateBenchmarks(host, benchmark)
	}
	if err != nil {
		hdb.log.Error("couldn't update benchmarks", zap.Error(err))
	}

	// Delete the host from scanMap.
	hdb.mu.Lock()
	delete(hdb.scanMap, host.PublicKey)
	hdb.benchmarking = false
	hdb.mu.Unlock()
}

// calculateBenchmarkInterval calculates a benchmark interval depending on
// how many previous benchmarks have been failed.
func (s *hostDBStore) calculateBenchmarkInterval(host *HostDBEntry) time.Duration {
	if host.LastBenchmark.Timestamp.IsZero() {
		return benchmarkInterval // 2 hours
	}

	num := s.lastFailedBenchmarks(host)
	if num > 13 && !host.ScanHistory[len(host.ScanHistory)-1].Success {
		return math.MaxInt64 // never
	}
	if num > 11 {
		return benchmarkInterval * 84 // 7 days
	}
	if num > 9 {
		return benchmarkInterval * 36 // 3 days
	}
	if num > 7 {
		return benchmarkInterval * 12 // 24 hours
	}
	if num > 5 {
		return benchmarkInterval * 4 // 8 hours
	}
	if num > 3 {
		return benchmarkInterval * 2 // 4 hours
	}
	return math.MaxInt64
}

// benchmarkCost estimates the cost of running a single benchmark.
func (s *HostDB) benchmarkCost(host *HostDBEntry) types.Currency {
	if (host.Settings == rhpv2.HostSettings{}) ||
		(host.PriceTable == rhpv3.HostPriceTable{}) ||
		(host.Revision.ParentID == types.FileContractID{}) {
		return types.ZeroCurrency
	}

	numSectors := benchmarkBatchSize / rhpv2.SectorSize
	uploadCost, _, _, err := rhp.UploadSectorCost(host.PriceTable, host.Revision.WindowEnd)
	if err != nil {
		return types.ZeroCurrency
	}
	downloadCost, err := rhp.ReadSectorCost(host.PriceTable, rhpv2.SectorSize)
	if err != nil {
		return types.ZeroCurrency
	}
	uploadCost = uploadCost.Mul64(uint64(numSectors))
	downloadCost = downloadCost.Mul64(uint64(numSectors))
	return host.PriceTable.UpdatePriceTableCost.
		Add(host.PriceTable.FundAccountCost).
		Add(host.PriceTable.LatestRevisionCost).
		Add(uploadCost).
		Add(downloadCost)
}

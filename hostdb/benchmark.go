package hostdb

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/rhp"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
)

const (
	benchmarkInterval  = 2 * time.Hour
	contractPeriod     = 7 * 144 // 7 days
	benchmarkBatchSize = 1 << 30 // 1 GiB
)

// benchmarkHost runs an up/download benchmark on a host.
func (hdb *HostDB) benchmarkHost(host HostDBEntry) {
	// Resolve the host's used subnets and update the timestamp if they
	// changed. We only update the timestamp if resolving the ipNets was
	// successful.
	ipNets, err := utils.LookupIPNets(host.NetAddress)
	if err == nil && !utils.EqualIPNets(ipNets, host.IPNets) {
		host.IPNets = ipNets
		host.LastIPChange = time.Now()
	}
	if err != nil {
		hdb.log.Println("[ERROR] failed to look up IP nets:", err)
	}

	// Update historic interactions of the host if necessary.
	hdb.mu.Lock()
	hdb.updateHostHistoricInteractions(&host)
	hdb.mu.Unlock()

	key := hdb.w.Key()

	timestamp := time.Now()
	var success bool
	var ul, dl float64
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
		err := checkGouging(hdb.s.tip.Height, &settings, &pt)
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

		// Check if we have a contract with this host.
		if host.Revision.WindowStart <= hdb.s.tip.Height {
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

			_, err := hdb.cm.AddPoolTransactions(txnSet)
			if err != nil {
				hdb.w.Release(txnSet)
				return utils.AddContext(err, "invalid transaction set")
			}
			hdb.syncer.BroadcastTransactionSet(txnSet)

			host.Revision = rev.Revision
			hdb.log.Printf("[DEBUG] successfully formed contract with %s: %s\n", host.NetAddress, rev.Revision.ParentID)
			success = true //TODO
		}

		return nil
	}()
	if err != nil && strings.Contains(errMsg, "operation was canceled") {
		// Shutting down.
		return
	}
	if err != nil && strings.Contains(errMsg, "insufficient balance") {
		// Not the host's fault.
		hdb.mu.Lock()
		delete(hdb.scanMap, host.PublicKey)
		hdb.mu.Unlock()
		return
	}
	if err == nil {
		hdb.IncrementSuccessfulInteractions(&host)
	} else {
		errMsg = err.Error()
		hdb.IncrementFailedInteractions(&host)
		hdb.log.Printf("[DEBUG] benchmark of %s failed: %v\n", host.NetAddress, err)
	}

	benchmark := HostBenchmark{
		Timestamp:     timestamp,
		Success:       success,
		Error:         errMsg,
		UploadSpeed:   ul,
		DownloadSpeed: dl,
	}
	err = hdb.s.updateBenchmarks(host, benchmark)
	if err != nil {
		hdb.log.Println("[ERROR] couldn't update benchmarks:", err)
	}

	// Delete the host from scanMap.
	hdb.mu.Lock()
	delete(hdb.scanMap, host.PublicKey)
	hdb.mu.Unlock()
}

// calculateBenchmarkInterval calculates a benchmark interval depending on
// how many previous benchmarks have been failed.
func (s *hostDBStore) calculateBenchmarkInterval(host *HostDBEntry) time.Duration {
	if host.LastBenchmark.Timestamp.IsZero() {
		return benchmarkInterval // 2 hours
	}

	num := s.lastFailedBenchmarks(host)
	if num > 10 {
		return benchmarkInterval * 12 // 24 hours
	}
	if num > 5 {
		return benchmarkInterval * 4 // 8 hours
	}
	if num > 3 {
		return benchmarkInterval * 2 // 4 hours
	}
	return benchmarkInterval // 2 hours
}

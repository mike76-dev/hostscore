package hostdb

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/rhp"
	walletutil "github.com/mike76-dev/hostscore/wallet"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	rhpv4utils "go.sia.tech/coreutils/rhp/v4"
	"go.uber.org/zap"
	"lukechampine.com/frand"
)

const (
	benchmarkInterval        = 2 * time.Hour
	benchmarkBatchSize       = 1 << 26 // 64 MiB
	revisionSubmissionBuffer = 144
)

type signer struct {
	w *walletutil.WalletManager
}

func (s signer) SignHash(h types.Hash256) types.Signature {
	return s.w.Key().SignHash(h)
}

func (s signer) SignV2Inputs(txn *types.V2Transaction, toSign []int) {
	s.w.SignV2Inputs(txn, toSign)
}

func (s signer) FundV2Transaction(txn *types.V2Transaction, amount types.Currency) (types.ChainIndex, []int, error) {
	return s.w.FundV2Transaction(txn, amount, true)
}

func (s signer) ReleaseInputs(txns []types.V2Transaction) {
	s.w.ReleaseInputs(nil, txns)
}

func (s signer) RecommendedFee() types.Currency {
	return s.w.RecommendedFee()
}

// benchmarkHost runs an up/download benchmark on a host.
func (hdb *HostDB) benchmarkHost(host *HostDBEntry) {
	store, ok := hdb.stores[host.Network]
	if !ok {
		panic("wrong host network")
	}

	limits := hdb.priceLimits
	timestamp := time.Now()
	var success bool
	var ul, dl float64
	var ttfb time.Duration
	var errMsg string

	err := func() error {
		// Do some checks first.
		if !host.V2 {
			return errors.New("V1 hosts not allowed anymore")
		}

		if (host.V2Settings == rhpv4.HostSettings{}) {
			return errors.New("host settings unavailable")
		}

		if count := store.checkSubnets(host.IPNets); count > 5 {
			return errors.New("too many hosts in the same subnet")
		}

		if err := checkGougingV2(&host.V2Settings, limits); err != nil {
			return err
		}

		// Check if we have a contract with this host and if it has enough money in it.
		if err := hdb.fetchSettingsV2(host); err != nil {
			return err
		}

		if err := hdb.formContractV2(host); err != nil {
			return err
		}

		// Use the channel to prevent other threads from running benchmarks
		// at the same time.
		for {
			hdb.mu.Lock()
			if !hdb.benchmarking {
				hdb.benchmarking = true
				hdb.mu.Unlock()
				break
			}
			hdb.mu.Unlock()
			select {
			case <-hdb.tg.StopChan():
			case <-time.After(time.Second):
			}
		}
		defer func() {
			hdb.mu.Lock()
			hdb.benchmarking = false
			hdb.mu.Unlock()
		}()

		// Fund the ephemeral account.
		if err := hdb.fundAccountV2(host); err != nil {
			return err
		}

		// Run the upload benchmark.
		var roots []types.Hash256
		var err error
		roots, ul, err = hdb.runUploadBenchmarkV2(host)
		if err != nil {
			return err
		}

		// Run the download benchmark.
		dl, ttfb, err = hdb.runDownloadBenchmarkV2(host, roots)
		if err != nil {
			return err
		}

		return nil
	}()
	if err != nil && strings.Contains(err.Error(), "canceled") {
		// Shutting down.
		return
	}
	if err != nil && strings.Contains(err.Error(), "not enough funds") && !strings.Contains(err.Error(), "not enough funds (5)") {
		// Not the host's fault.
		hdb.mu.Lock()
		delete(hdb.scanMap, host.PublicKey)
		hdb.benchmarkThreads--
		hdb.mu.Unlock()
		return
	}
	if err == nil {
		success = true
		host.Interactions.Successes++
	} else {
		errMsg = err.Error()
		// If we are offline it probably wasn't the host's fault.
		if !hdb.online(host.Network) {
			host.Interactions.Failures++
		}
	}

	benchmark := HostBenchmark{
		Timestamp:     timestamp,
		Success:       success,
		Error:         errMsg,
		UploadSpeed:   ul,
		DownloadSpeed: dl,
		TTFB:          ttfb,
	}
	if err := store.updateBenchmarks(host, benchmark); err != nil {
		hdb.log.Error("couldn't update benchmarks", zap.Error(err))
	}

	// Delete the host from scanMap.
	hdb.mu.Lock()
	delete(hdb.scanMap, host.PublicKey)
	hdb.benchmarkThreads--
	hdb.mu.Unlock()
}

// calculateBenchmarkInterval calculates a benchmark interval depending on
// how many previous benchmarks have been failed.
func (s *hostDBStore) calculateBenchmarkInterval(host *HostDBEntry) time.Duration {
	if host.LastBenchmark.Timestamp.IsZero() {
		return benchmarkInterval // 2 hours
	}

	num := s.lastFailedBenchmarks(host)
	if num > 13 && s.lastFailedScans(host) > 18 {
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
	return benchmarkInterval
}

// benchmarkCostV2 estimates the cost of running a single V2 benchmark.
func benchmarkCostV2(host *HostDBEntry) types.Currency {
	if (host.V2Settings == rhpv4.HostSettings{}) ||
		(host.V2Revision.Parent.ID == types.FileContractID{}) {
		return types.ZeroCurrency
	}

	numSectors := benchmarkBatchSize / rhpv4.SectorSize
	prices := host.V2Settings.Prices
	writeCost := prices.RPCWriteSectorCost(rhpv4.SectorSize)
	readCost := prices.RPCReadSectorCost(rhpv4.SectorSize)
	return writeCost.RenterCost().Add(readCost.RenterCost()).Mul64(uint64(numSectors))
}

// fetchSettingsV2 retrieves the latest settings from the host.
func (hdb *HostDB) fetchSettingsV2(host *HostDBEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	go func() {
		select {
		case <-hdb.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()

	return rhp.WithTransportV4(ctx, host.NetAddress, host.PublicKey, func(t rhpv4utils.TransportClient) error {
		v2Settings, err := rhpv4utils.RPCSettings(ctx, t)
		if err != nil {
			return err
		}

		host.V2Settings = v2Settings
		return nil
	})
}

// formContractV2 checks if there is a contract with the host and if that contract is good
// for benchmarking the host. If so, it fetches the latest revision. If not, a new contract
// is formed.
func (hdb *HostDB) formContractV2(host *HostDBEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	go func() {
		select {
		case <-hdb.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()

	height := hdb.nodes.ChainManager(host.Network).Tip().Height
	rev := host.V2Revision.Revision
	if rev.ProofHeight <= height+revisionSubmissionBuffer || rev.RenterOutput.Value.Cmp(benchmarkCostV2(host)) < 0 {
		// Form a new contract.
		if err := rhp.WithTransportV4(ctx, host.NetAddress, host.PublicKey, func(t rhpv4utils.TransportClient) error {
			cm := hdb.nodes.ChainManager(host.Network)
			w := signer{hdb.nodes.Wallet(host.Network)}
			renterFunds, hostCollateral := calculateFundingV2(host.V2Settings.Prices, cm.RecommendedFee().Mul64(2048))
			res, err := rhpv4utils.RPCFormContract(ctx, t, cm, w, cm.TipState(), host.V2Settings.Prices, host.PublicKey, host.V2Settings.WalletAddress, rhpv4.RPCFormContractParams{
				RenterPublicKey: w.w.Key().PublicKey(),
				RenterAddress:   w.w.Address(),
				Allowance:       renterFunds,
				Collateral:      hostCollateral,
				ProofHeight:     height + contractDuration,
			})
			if err != nil {
				w.ReleaseInputs(res.FormationSet.Transactions)
				return utils.AddContext(err, "couldn't prepare v2 contract")
			}

			go func() {
				_, err = cm.AddV2PoolTransactions(res.FormationSet.Basis, res.FormationSet.Transactions)
				if err != nil {
					return
				}
				hdb.nodes.Syncer(host.Network).BroadcastV2TransactionSet(res.FormationSet.Basis, res.FormationSet.Transactions)
			}()

			host.V2Revision = types.V2FileContractRevision{
				Parent: types.V2FileContractElement{
					ID: res.Contract.ID,
				},
				Revision: res.Contract.Revision,
			}
			return nil
		}); err != nil {
			return err
		}

		hdb.log.Info("successfully formed v2 contract", zap.String("network", host.Network), zap.String("host", host.NetAddress), zap.Stringer("id", host.V2Revision.Parent.ID))
	} else {
		// Fetch the latest revision.
		if err := rhp.WithTransportV4(ctx, host.NetAddress, host.PublicKey, func(t rhpv4utils.TransportClient) error {
			res, err := rhpv4utils.RPCLatestRevision(ctx, t, host.V2Revision.Parent.ID)
			if err != nil {
				return utils.AddContext(err, "unable to get latest v2 revision")
			}
			host.V2Revision = types.V2FileContractRevision{
				Parent: types.V2FileContractElement{
					ID: host.V2Revision.Parent.ID,
				},
				Revision: res.Contract,
			}
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// fundAccountV2 fetches the host's current settings, checks the ephemeral account balance,
// and funds the account, if required.
func (hdb *HostDB) fundAccountV2(host *HostDBEntry) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() {
		select {
		case <-hdb.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()

	cm := hdb.nodes.ChainManager(host.Network)
	w := signer{hdb.nodes.Wallet(host.Network)}
	key := hdb.nodes.Wallet(host.Network).Key()
	rev := rhpv4utils.ContractRevision{
		ID:       host.V2Revision.Parent.ID,
		Revision: host.V2Revision.Revision,
	}

	return rhp.WithTransportV4(ctx, host.NetAddress, host.PublicKey, func(t rhpv4utils.TransportClient) error {
		// Fetch the latest settings.
		settings, err := rhpv4utils.RPCSettings(ctx, t)
		if err != nil {
			return err
		}
		host.V2Settings = settings
		err = checkGougingV2(&settings, hdb.priceLimits)
		if err != nil {
			return err
		}

		// Check the account balance.
		balance, err := rhpv4utils.RPCAccountBalance(ctx, t, rhpv4.Account(key.PublicKey()))
		if err != nil {
			return utils.AddContext(err, "unable to fetch account balance")
		}

		// Fund the account.
		numSectors := benchmarkBatchSize / rhpv4.SectorSize
		upload := settings.Prices.RPCWriteSectorCost(rhpv4.SectorSize)
		download := settings.Prices.RPCReadSectorCost(rhpv4.SectorSize)
		amount := upload.Add(download).RenterCost().Mul64(uint64(numSectors))
		if amount.Cmp(balance) <= 0 {
			return nil
		}
		amount = amount.Sub(balance)
		_, err = rhpv4utils.RPCFundAccounts(ctx, t, cm.TipState(), w, rev, []rhpv4.AccountDeposit{
			{Account: rhpv4.Account(key.PublicKey()), Amount: amount},
		})
		if err != nil {
			return utils.AddContext(err, "unable to fund account")
		}

		return nil
	})
}

// runUploadBenchmarkV2 performs a V2 upload benchmark on the host.
func (hdb *HostDB) runUploadBenchmarkV2(host *HostDBEntry) (roots []types.Hash256, speed float64, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	go func() {
		select {
		case <-hdb.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()

	numSectors := benchmarkBatchSize / rhpv2.SectorSize
	var data [rhpv4.SectorSize]byte
	roots = make([]types.Hash256, numSectors)
	key := hdb.nodes.Wallet(host.Network).Key()
	account := rhpv4.Account(key.PublicKey())
	start := time.Now()
	if err := rhp.WithTransportV4(ctx, host.NetAddress, host.PublicKey, func(t rhpv4utils.TransportClient) error {
		for i := 0; i < numSectors; i++ {
			frand.Read(data[:256])
			res, err := rhpv4utils.RPCWriteSector(ctx, t, host.V2Settings.Prices, account.Token(key, host.PublicKey), bytes.NewReader(data[:]), uint64(len(data)))
			if err != nil {
				return utils.AddContext(err, "unable to upload sector")
			}
			roots[i] = res.Root
		}
		return nil
	}); err != nil {
		return nil, 0, utils.AddContext(err, "v2 upload benchmark failed")
	}

	return roots, float64(benchmarkBatchSize) / time.Since(start).Seconds(), nil
}

// runDownloadBenchmarkV2 performs a v2 download benchmark on the host.
func (hdb *HostDB) runDownloadBenchmarkV2(host *HostDBEntry, roots []types.Hash256) (speed float64, ttfb time.Duration, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	go func() {
		select {
		case <-hdb.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()

	numSectors := benchmarkBatchSize / rhpv2.SectorSize
	var data [rhpv4.SectorSize]byte
	key := hdb.nodes.Wallet(host.Network).Key()
	account := rhpv4.Account(key.PublicKey())
	start := time.Now()
	if err := rhp.WithTransportV4(ctx, host.NetAddress, host.PublicKey, func(t rhpv4utils.TransportClient) error {
		for i := 0; i < numSectors; i++ {
			tw := newTTFBWriter(bytes.NewBuffer(data[:]))
			_, err := rhpv4utils.RPCReadSector(ctx, t, host.V2Settings.Prices, account.Token(key, host.PublicKey), tw, roots[i], 0, rhpv4.SectorSize)
			if err != nil {
				return utils.AddContext(err, "unable to download sector")
			}
			if i == 0 {
				ttfb = time.Since(start)
			}
		}
		if err != nil {
			return utils.AddContext(err, "v2 download benchmark failed")
		}
		return nil
	}); err != nil {
		return 0, 0, err
	}

	return float64(benchmarkBatchSize) / time.Since(start).Seconds(), ttfb, nil
}

// calculateFundingV2 calculates the funding of a V2 benchmarking contract.
func calculateFundingV2(prices rhpv4.HostPrices, txnFee types.Currency) (funding, collateral types.Currency) {
	numBenchmarks := contractDuration / (6 * benchmarkInterval / time.Hour)
	dataSize := benchmarkBatchSize * numBenchmarks
	numSectors := dataSize / rhpv4.SectorSize

	writeCost := prices.RPCWriteSectorCost(rhpv4.SectorSize)
	readCost := prices.RPCReadSectorCost(rhpv4.SectorSize)

	funding = writeCost.RenterCost().Add(readCost.RenterCost()).Mul64(uint64(numSectors))
	funding = funding.Add(prices.ContractPrice).Add(txnFee)

	collateral = writeCost.HostRiskedCollateral().Add(readCost.HostRiskedCollateral()).Mul64(uint64(numSectors))

	return
}

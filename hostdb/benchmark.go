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
	walletutil "github.com/mike76-dev/hostscore/wallet"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	rhpv4utils "go.sia.tech/coreutils/rhp/v4"
	"go.sia.tech/coreutils/wallet"
	"go.uber.org/zap"
	"lukechampine.com/frand"
)

const (
	benchmarkInterval  = 2 * time.Hour
	benchmarkBatchSize = 1 << 26 // 64 MiB
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

// benchmarkHost runs an up/download benchmark on a host.
func (hdb *HostDB) benchmarkHost(host *HostDBEntry) {
	store, ok := hdb.stores[host.Network]
	if !ok {
		panic("wrong host network")
	}

	limits := hdb.priceLimits
	allowHeight := hdb.nodes.ChainManager(host.Network).TipState().Network.HardforkV2.AllowHeight
	requireHeight := hdb.nodes.ChainManager(host.Network).TipState().Network.HardforkV2.RequireHeight
	height := hdb.nodes.ChainManager(host.Network).Tip().Height

	timestamp := time.Now()
	var success bool
	var ul, dl float64
	var ttfb time.Duration
	var errMsg string

	err := func() error {
		// Do some checks first.
		if host.V2 && (host.V2Settings == rhpv4.HostSettings{}) {
			return errors.New("host settings unavailable")
		} else if !host.V2 && (host.Settings == rhpv2.HostSettings{}) {
			return errors.New("host settings unavailable")
		}

		if count := store.checkSubnets(host.IPNets); count > 5 {
			return errors.New("too many hosts in the same subnet")
		}

		var err error
		if host.V2 && height >= allowHeight {
			err = checkGougingV2(&host.V2Settings, limits)
		} else {
			err = checkGougingV1(&host.Settings, nil, limits)
		}
		if err != nil {
			return err
		}

		// Check if we have a contract with this host and if it has enough money in it.
		if host.V2 && height >= allowHeight {
			if err := hdb.fetchSettingsV2(host); err != nil {
				return err
			}
			if err := hdb.formContractV2(host); err != nil {
				return err
			}
		} else if height < requireHeight {
			if err := hdb.formContractV1(host); err != nil {
				return err
			}
		} else {
			return errors.New("V1 hosts not allowed anymore")
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

		if host.V2 && height >= allowHeight {
			// Fund the ephemeral account.
			if err := hdb.fundAccountV2(host); err != nil {
				return err
			}

			// Run the upload benchmark.
			var roots []types.Hash256
			roots, ul, err = hdb.runUploadBenchmarkV2(host)
			if err != nil {
				return err
			}

			// Run the download benchmark.
			dl, ttfb, err = hdb.runDownloadBenchmarkV2(host, roots)
			if err != nil {
				return err
			}
		} else {
			// Fund the ephemeral account.
			uploadCost, downloadCost, err := hdb.fundAccountV1(host)
			if err != nil {
				return err
			}

			// Run the upload benchmark.
			var roots []types.Hash256
			roots, ul, err = hdb.runUploadBenchmarkV1(host, uploadCost)
			if err != nil {
				return err
			}

			// Run the download benchmark.
			dl, ttfb, err = hdb.runDownloadBenchmarkV1(host, downloadCost, roots)
			if err != nil {
				return err
			}
		}

		return nil
	}()
	if err != nil && strings.Contains(err.Error(), "canceled") {
		// Shutting down.
		return
	}
	if err != nil && strings.Contains(err.Error(), "not enough funds") {
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

// benchmarkCost estimates the cost of running a single benchmark.
func benchmarkCost(host *HostDBEntry) types.Currency {
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

// formContractV1 checks if there is a contract with the host and if that contract is good
// for benchmarking the host. If so, it fetches the latest revision. If not, a new contract
// is formed.
func (hdb *HostDB) formContractV1(host *HostDBEntry) error {
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
	if host.Revision.WindowStart <= height+144 || host.Revision.ValidRenterPayout().Cmp(benchmarkCost(host)) < 0 {
		// Form a new contract.
		var rev rhpv2.ContractRevision
		var txnSet []types.Transaction
		w := hdb.nodes.Wallet(host.Network)
		if err := rhp.WithTransportV2(ctx, host.Settings.NetAddress, host.PublicKey, func(t *rhpv2.Transport) error {
			renterTxnSet, err := hdb.prepareContractFormation(host)
			if err != nil {
				return utils.AddContext(err, "couldn't prepare contract")
			}

			rev, txnSet, err = rhp.RPCFormContract(ctx, t, w.Key(), renterTxnSet)
			if err != nil {
				w.ReleaseInputs(renterTxnSet, nil)
				return utils.AddContext(err, "couldn't form contract")
			}

			return nil
		}); err != nil {
			return err
		}

		go func() {
			_, err := hdb.nodes.ChainManager(host.Network).AddPoolTransactions(txnSet)
			if err != nil {
				return
			}
			hdb.nodes.Syncer(host.Network).BroadcastTransactionSet(txnSet)
		}()

		host.Revision = rev.Revision
		hdb.log.Info("successfully formed contract", zap.String("network", host.Network), zap.String("host", host.NetAddress), zap.Stringer("id", rev.Revision.ParentID))
	} else {
		// Fetch the latest revision.
		h, _, _ := net.SplitHostPort(host.NetAddress)
		addr := net.JoinHostPort(h, host.Settings.SiaMuxPort)
		if err := rhp.WithTransportV3(ctx, addr, host.PublicKey, func(t *rhpv3.Transport) error {
			rev, err := rhp.RPCLatestRevision(ctx, t, host.Revision.ParentID)
			if err != nil {
				return utils.AddContext(err, "unable to get latest revision")
			}
			host.Revision = rev
			return nil
		}); err != nil {
			return err
		}
	}

	return nil
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

	return rhp.WithTransportV4(ctx, host.SiamuxAddresses[0], host.PublicKey, func(t rhpv4utils.TransportClient) error {
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
	addr := host.SiamuxAddresses[0]
	rev := host.V2Revision.Revision
	if rev.ProofHeight <= height || rev.RenterOutput.Value.Cmp(benchmarkCostV2(host)) < 0 {
		// Form a new contract.
		if err := rhp.WithTransportV4(ctx, addr, host.PublicKey, func(t rhpv4utils.TransportClient) error {
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
		if err := rhp.WithTransportV4(ctx, addr, host.PublicKey, func(t rhpv4utils.TransportClient) error {
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

// fundAccountV1 fetches the valid price table, checks the ephemeral account balance,
// and funds the account, if required.
func (hdb *HostDB) fundAccountV1(host *HostDBEntry) (uploadCost, downloadCost types.Currency, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() {
		select {
		case <-hdb.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()

	h, _, _ := net.SplitHostPort(host.NetAddress)
	addr := net.JoinHostPort(h, host.Settings.SiaMuxPort)
	key := hdb.nodes.Wallet(host.Network).Key()
	if err := rhp.WithTransportV3(ctx, addr, host.PublicKey, func(t *rhpv3.Transport) error {
		// Fetch a price table.
		pt, err := rhp.RPCPriceTable(ctx, t, func(pt rhpv3.HostPriceTable) (rhpv3.PaymentMethod, error) {
			payment, ok := rhpv3.PayByContract(&host.Revision, pt.UpdatePriceTableCost, rhpv3.Account(key.PublicKey()), key)
			if !ok {
				return nil, wallet.ErrNotEnoughFunds
			}
			return &payment, nil
		})
		if err != nil {
			// Check if we have passed the revision deadline.
			if strings.Contains(err.Error(), "renter is requesting revision after the revision deadline") {
				host.Revision = types.FileContractRevision{}
			}
			return utils.AddContext(err, "unable to get price table")
		}
		host.PriceTable = pt
		err = checkGougingV1(nil, &pt, hdb.priceLimits)
		if err != nil {
			return err
		}

		// Check the account balance.
		payment, ok := rhpv3.PayByContract(&host.Revision, pt.AccountBalanceCost, rhpv3.Account(key.PublicKey()), key)
		if !ok {
			host.Revision = types.FileContractRevision{}
			return wallet.ErrNotEnoughFunds
		}
		balance, err := rhp.RPCAccountBalance(ctx, t, &payment, rhpv3.Account(key.PublicKey()), pt.UID)
		if err != nil {
			return utils.AddContext(err, "unable to fetch account balance")
		}

		// Fund the account.
		uploadCost, _, _, err = rhp.UploadSectorCost(pt, host.Revision.WindowEnd)
		if err != nil {
			return utils.AddContext(err, "unable to estimate costs")
		}
		downloadCost, err = rhp.ReadSectorCost(pt, rhpv2.SectorSize)
		if err != nil {
			return utils.AddContext(err, "unable to estimate costs")
		}
		amount := uploadCost.Add(downloadCost).Mul64(benchmarkBatchSize / rhpv2.SectorSize)
		amount = amount.Add(pt.FundAccountCost)
		if amount.Cmp(balance) <= 0 {
			return nil
		}
		amount = amount.Sub(balance)
		payment, ok = rhpv3.PayByContract(&host.Revision, amount, rhpv3.Account{}, key)
		if !ok {
			host.Revision = types.FileContractRevision{}
			return wallet.ErrNotEnoughFunds
		}
		if err := rhp.RPCFundAccount(ctx, t, &payment, rhpv3.Account(key.PublicKey()), pt.UID); err != nil {
			return utils.AddContext(err, "unable to fund account")
		}

		return nil
	}); err != nil {
		return types.Currency{}, types.Currency{}, err
	}

	return uploadCost, downloadCost, nil
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
	addr := host.SiamuxAddresses[0]
	key := hdb.nodes.Wallet(host.Network).Key()
	rev := rhpv4utils.ContractRevision{
		ID:       host.V2Revision.Parent.ID,
		Revision: host.V2Revision.Revision,
	}

	return rhp.WithTransportV4(ctx, addr, host.PublicKey, func(t rhpv4utils.TransportClient) error {
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
		upload := settings.Prices.RPCAppendSectorsCost(benchmarkBatchSize/rhpv4.SectorSize, host.V2Revision.Revision.ExpirationHeight-cm.Tip().Height)
		download := settings.Prices.RPCReadSectorCost(benchmarkBatchSize)
		amount := upload.Add(download).RenterCost()
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

// runUploadBenchmarkV1 performs an upload benchmark on the host.
func (hdb *HostDB) runUploadBenchmarkV1(host *HostDBEntry, cost types.Currency) (roots []types.Hash256, speed float64, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	go func() {
		select {
		case <-hdb.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()

	h, _, _ := net.SplitHostPort(host.NetAddress)
	addr := net.JoinHostPort(h, host.Settings.SiaMuxPort)
	numSectors := benchmarkBatchSize / rhpv2.SectorSize
	var data [rhpv2.SectorSize]byte
	roots = make([]types.Hash256, numSectors)
	key := hdb.nodes.Wallet(host.Network).Key()
	start := time.Now()
	if err := rhp.WithTransportV3(ctx, addr, host.PublicKey, func(t *rhpv3.Transport) error {
		for i := 0; i < numSectors; i++ {
			frand.Read(data[:256])
			payment := rhpv3.PayByEphemeralAccount(rhpv3.Account(key.PublicKey()), cost, host.PriceTable.HostBlockHeight+6, key)
			root, _, err := rhp.RPCAppendSector(ctx, t, key, host.PriceTable, &host.Revision, &payment, &data)
			if err != nil {
				return utils.AddContext(err, "unable to upload sector")
			}
			roots[i] = root
		}
		return nil
	}); err != nil {
		return nil, 0, utils.AddContext(err, "upload benchmark failed")
	}

	return roots, float64(benchmarkBatchSize) / time.Since(start).Seconds(), nil
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

	addr := host.SiamuxAddresses[0]
	numSectors := benchmarkBatchSize / rhpv2.SectorSize
	var data [rhpv4.SectorSize]byte
	roots = make([]types.Hash256, numSectors)
	key := hdb.nodes.Wallet(host.Network).Key()
	account := rhpv4.Account(key.PublicKey())
	start := time.Now()
	if err := rhp.WithTransportV4(ctx, addr, host.PublicKey, func(t rhpv4utils.TransportClient) error {
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

// runDownloadBenchmarkV1 performs a download benchmark on the host.
func (hdb *HostDB) runDownloadBenchmarkV1(host *HostDBEntry, cost types.Currency, roots []types.Hash256) (speed float64, ttfb time.Duration, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	go func() {
		select {
		case <-hdb.tg.StopChan():
			cancel()
		case <-ctx.Done():
		}
	}()

	h, _, _ := net.SplitHostPort(host.NetAddress)
	addr := net.JoinHostPort(h, host.Settings.SiaMuxPort)
	numSectors := benchmarkBatchSize / rhpv2.SectorSize
	var data [rhpv2.SectorSize]byte
	key := hdb.nodes.Wallet(host.Network).Key()
	start := time.Now()
	if err := rhp.WithTransportV3(ctx, addr, host.PublicKey, func(t *rhpv3.Transport) error {
		for i := 0; i < numSectors; i++ {
			payment := rhpv3.PayByEphemeralAccount(rhpv3.Account(key.PublicKey()), cost, host.PriceTable.HostBlockHeight+6, key)
			tw := newTTFBWriter(bytes.NewBuffer(data[:]))
			_, _, err := rhp.RPCReadSector(ctx, t, tw, host.PriceTable, &payment, 0, rhpv2.SectorSize, roots[i])
			if err != nil {
				return utils.AddContext(err, "unable to download sector")
			}
			if i == 0 {
				ttfb = tw.TTFB()
			}
		}
		if err != nil {
			return utils.AddContext(err, "download benchmark failed")
		}
		return nil
	}); err != nil {
		return 0, 0, err
	}

	return float64(benchmarkBatchSize) / time.Since(start).Seconds(), ttfb, nil
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

	addr := host.SiamuxAddresses[0]
	numSectors := benchmarkBatchSize / rhpv2.SectorSize
	var data [rhpv4.SectorSize]byte
	key := hdb.nodes.Wallet(host.Network).Key()
	account := rhpv4.Account(key.PublicKey())
	start := time.Now()
	if err := rhp.WithTransportV4(ctx, addr, host.PublicKey, func(t rhpv4utils.TransportClient) error {
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

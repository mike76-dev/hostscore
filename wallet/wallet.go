package wallet

import (
	"fmt"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/internal/sync"
	"github.com/mike76-dev/hostscore/internal/utils"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/coreutils/syncer"
	"go.sia.tech/coreutils/wallet"
	"go.uber.org/zap"
)

const (
	// walletMaintenanceInterval is how often the wallet maintenance is done.
	walletMaintenanceInterval = 10 * time.Minute

	// wantedOutputs is how many unspent SiacoinOutputs we want to have.
	wantedOutputs = 100
)

var (
	defragThreshold     = 300
	maxInputsForDefrag  = 300
	maxDefragUTXOs      = 10
	reservationDuration = 15 * time.Minute
	outputValue         = types.Siacoins(20)
)

// WalletManager represents a wallet that is stored in a MySQL database.
type WalletManager struct {
	wallet *wallet.SingleAddressWallet
	store  *DBStore
	chain  *chain.Manager
	syncer *syncer.Syncer
	log    *zap.Logger
	tg     sync.ThreadGroup
}

// NewWallet returns an initialized WalletManager.
func NewWallet(store *DBStore, cm *chain.Manager, syncer *syncer.Syncer, log *zap.Logger) (*WalletManager, error) {
	tip, _ := store.Tip()
	w, err := wallet.NewSingleAddressWallet(
		store.key, cm, store,
		wallet.WithLogger(log),
		wallet.WithDefragThreshold(defragThreshold),
		wallet.WithMaxDefragUTXOs(maxDefragUTXOs),
		wallet.WithMaxInputsForDefrag(maxInputsForDefrag),
		wallet.WithReservationDuration(reservationDuration),
	)
	if err != nil {
		return nil, err
	}

	wm := &WalletManager{
		wallet: w,
		store:  store,
		chain:  cm,
		syncer: syncer,
		log:    log,
	}

	reorgCh := make(chan struct{}, 1)
	reorgCh <- struct{}{}
	stop := cm.OnReorg(func(index types.ChainIndex) {
		select {
		case reorgCh <- struct{}{}:
		default:
		}
	})

	go func() {
		defer stop()

		if err := wm.tg.Add(); err != nil {
			return
		}
		defer wm.tg.Done()

		for cm.Tip().Height <= tip.Height {
			select {
			case <-wm.tg.StopChan():
				return
			default:
				time.Sleep(5 * time.Second)
			}
		}

		for {
			select {
			case <-wm.tg.StopChan():
				return
			case <-reorgCh:
				index, _ := wm.store.Tip()
				if err := wm.syncStore(index); err != nil {
					wm.log.Error("failed to sync database", zap.String("network", wm.store.network), zap.Error(err))
				}
			}
		}
	}()

	go wm.performWalletMaintenance()

	return wm, nil
}

func (wm *WalletManager) syncStore(index types.ChainIndex) error {
	for index != wm.chain.Tip() {
		select {
		case <-wm.tg.StopChan():
			return nil
		default:
		}

		reverted, applied, err := wm.chain.UpdatesSince(index, 100)
		if err != nil && strings.Contains(err.Error(), "missing block at index") {
			wm.log.Warn("missing block at index, resetting chain state", zap.String("network", wm.store.network), zap.Uint64("height", index.Height))
			if err := wm.store.resetChainState(); err != nil {
				return utils.AddContext(err, "failed to reset consensus state")
			}
			return nil
		} else if err != nil {
			return fmt.Errorf("failed to get updates since %v: %w", index, err)
		} else if len(reverted) == 0 && len(applied) == 0 {
			return nil
		}

		if err := wm.wallet.UpdateChainState(wm.store, reverted, applied); err != nil {
			return fmt.Errorf("failed to update wallet state: %w", err)
		}

		if len(applied) > 0 {
			index = applied[len(applied)-1].State.Index
		} else {
			index = reverted[len(reverted)-1].State.Index
		}

		if err := wm.store.updateChainState(index, true); err != nil {
			wm.log.Error("couldn't update index", zap.String("network", wm.store.network), zap.Error(err))
			return err
		}
	}
	return nil
}

// Close shuts down the WalletManager.
func (wm *WalletManager) Close() error {
	if err := wm.tg.Stop(); err != nil {
		wm.log.Error("unable to stop threads", zap.String("network", wm.store.network), zap.Error(err))
	}
	wm.store.close()
	return nil
}

// Address returns the address of the wallet.
func (wm *WalletManager) Address() types.Address {
	return wm.store.addr
}

// Key returns the private key that controls the wallet.
func (wm *WalletManager) Key() types.PrivateKey {
	return wm.store.key
}

// UnspentSiacoinElements returns the wallet's unspent siacoin outputs.
func (wm *WalletManager) UnspentSiacoinElements() ([]types.SiacoinElement, error) {
	return wm.wallet.UnspentSiacoinElements()
}

// Balance returns the balance of the wallet.
func (wm *WalletManager) Balance() (wallet.Balance, error) {
	return wm.wallet.Balance()
}

// UnconfirmedEvents returns all unconfirmed transactions relevant to the wallet.
func (wm *WalletManager) UnconfirmedEvents() ([]wallet.Event, error) {
	return wm.wallet.UnconfirmedEvents()
}

// FundTransaction adds siacoin inputs worth at least amount to the provided
// transaction. If necessary, a change output will also be added.
func (wm *WalletManager) FundTransaction(txn *types.Transaction, amount types.Currency, useUnconfirmed bool) ([]types.Hash256, error) {
	return wm.wallet.FundTransaction(txn, amount, useUnconfirmed)
}

// SignTransaction adds a signature to each of the specified inputs.
func (wm *WalletManager) SignTransaction(txn *types.Transaction, toSign []types.Hash256, cf types.CoveredFields) {
	wm.wallet.SignTransaction(txn, toSign, cf)
}

// FundV2Transaction adds siacoin inputs worth at least amount to the provided
// transaction. If necessary, a change output will also be added.
func (wm *WalletManager) FundV2Transaction(txn *types.V2Transaction, amount types.Currency, useUnconfirmed bool) (types.ChainIndex, []int, error) {
	return wm.wallet.FundV2Transaction(txn, amount, useUnconfirmed)
}

// SignV2Inputs adds a signature to each of the specified siacoin inputs.
func (wm *WalletManager) SignV2Inputs(txn *types.V2Transaction, toSign []int) {
	wm.wallet.SignV2Inputs(txn, toSign)
}

// ReleaseInputs marks the inputs as unused.
func (wm *WalletManager) ReleaseInputs(v1txns []types.Transaction, v2txns []types.V2Transaction) {
	wm.wallet.ReleaseInputs(v1txns, v2txns)
}

// synced returns true if the wallet is synced to the blockchain.
func (wm *WalletManager) synced() bool {
	isSynced := func(s *syncer.Syncer) bool {
		var count int
		for _, p := range s.Peers() {
			if p.Synced() {
				count++
			}
		}
		return count >= 5
	}
	return isSynced(wm.syncer) && time.Since(wm.chain.TipState().PrevTimestamps[0]) < 24*time.Hour
}

// performWalletMaintenance performs the wallet maintenance periodically.
func (wm *WalletManager) performWalletMaintenance() {
	redistribute := func() {
		if relevantTransactions(wm.chain.PoolTransactions(), wm.store.addr) ||
			relevantV2Transactions(wm.chain.V2PoolTransactions(), wm.store.addr) {
			return
		}

		utxos, err := wm.wallet.UnspentSiacoinElements()
		if err != nil {
			wm.log.Error("couldn't get unspent outputs", zap.String("network", wm.store.network), zap.Error(err))
			return
		}

		numOutputs := wantedOutputs
		maxOutputs := len(utxos) * 10
		if numOutputs > maxOutputs {
			numOutputs = maxOutputs
		}

		balance := wallet.SumOutputs(utxos)
		fee := wm.chain.RecommendedFee()

		if state := wm.chain.TipState(); state.Index.Height < state.Network.HardforkV2.AllowHeight {
			txns, toSign, err := wm.wallet.Redistribute(numOutputs, outputValue, fee)
			if err != nil {
				wm.log.Error("failed to redistribute wallet", zap.String("network", wm.store.network), zap.Int("outputs", numOutputs), zap.Stringer("amount", outputValue), zap.Stringer("balance", balance), zap.Error(err))
				return
			}

			if len(txns) == 0 {
				return
			}

			for i := 0; i < len(txns); i++ {
				wm.SignTransaction(&txns[i], toSign[i], types.CoveredFields{WholeTransaction: true})
			}

			_, err = wm.chain.AddPoolTransactions(txns)
			if err != nil {
				wm.log.Error("failed to broadcast v1 transactions", zap.String("network", wm.store.network), zap.Error(err))
				wm.ReleaseInputs(txns, nil)
				return
			}

			wm.syncer.BroadcastTransactionSet(txns)
		} else {
			txns, toSign, err := wm.wallet.RedistributeV2(numOutputs, outputValue, fee)
			if err != nil {
				wm.log.Error("failed to redistribute wallet", zap.String("network", wm.store.network), zap.Int("outputs", numOutputs), zap.Stringer("amount", outputValue), zap.Stringer("balance", balance), zap.Error(err))
				return
			}

			if len(txns) == 0 {
				return
			}

			for i := 0; i < len(txns); i++ {
				wm.SignV2Inputs(&txns[i], toSign[i])
			}

			_, err = wm.chain.AddV2PoolTransactions(state.Index, txns)
			if err != nil {
				wm.log.Error("failed to broadcast v2 transactions", zap.String("network", wm.store.network), zap.Error(err))
				wm.ReleaseInputs(nil, txns)
				return
			}

			wm.syncer.BroadcastV2TransactionSet(state.Index, txns)
		}
	}

	if err := wm.tg.Add(); err != nil {
		wm.log.Error("couldn't add a thread", zap.String("network", wm.store.network), zap.Error(err))
		return
	}
	defer wm.tg.Done()

	for {
		if wm.synced() {
			break
		}
		select {
		case <-wm.tg.StopChan():
			return
		case <-time.After(30 * time.Second):
		}
	}

	redistribute()

	for {
		select {
		case <-wm.tg.StopChan():
			return
		case <-time.After(walletMaintenanceInterval):
			redistribute()
		}
	}
}

// relevantTransactions returns true if there is at least one relevant
// transaction in the v1 transaction set.
func relevantTransactions(txnSet []types.Transaction, addr types.Address) bool {
	for _, txn := range txnSet {
		if wallet.IsRelevantTransaction(txn, addr) {
			return true
		}
	}
	return false
}

// relevantV2Transactions returns true if there is at least one relevant
// transaction in the v2 transaction set.
func relevantV2Transactions(txnSet []types.V2Transaction, addr types.Address) bool {
	for _, txn := range txnSet {
		for _, sci := range txn.SiacoinInputs {
			if sci.Parent.SiacoinOutput.Address == addr {
				return true
			}
		}

		for _, sco := range txn.SiacoinOutputs {
			if sco.Address == addr {
				return true
			}
		}
	}
	return false
}

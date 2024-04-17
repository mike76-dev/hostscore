package walletutil

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"sync"
	"time"

	siasync "github.com/mike76-dev/hostscore/internal/sync"
	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/persist"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/coreutils/syncer"
	"go.uber.org/zap"
)

const (
	// bytesPerInput is the encoded size of a SiacoinInput and corresponding
	// TransactionSignature, assuming standard UnlockConditions.
	bytesPerInput = 241

	// redistributeBatchSize is the number of outputs to redistribute per txn to
	// avoid creating a txn that is too large.
	redistributeBatchSize = 10

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
)

// ErrInsufficientBalance is returned when there aren't enough unused outputs to
// cover the requested amount.
var ErrInsufficientBalance = errors.New("insufficient balance")

type Wallet struct {
	db             *sql.DB
	s              *DBStore
	sZen           *DBStore
	cm             *chain.Manager
	cmZen          *chain.Manager
	syncer         *syncer.Syncer
	syncerZen      *syncer.Syncer
	unsubscribe    func()
	unsubscribeZen func()
	log            *zap.Logger
	closeFn        func()

	mu     sync.Mutex
	tg     siasync.ThreadGroup
	locked map[types.Hash256]time.Time
}

// Address implements api.Wallet.
func (w *Wallet) Address(network string) types.Address {
	if network == "zen" {
		return w.sZen.Address()
	}
	if network == "mainnet" {
		return w.s.Address()
	}
	panic("wrong network provided")
}

// Key implements api.Wallet.
func (w *Wallet) Key(network string) types.PrivateKey {
	if network == "zen" {
		return w.sZen.key
	}
	if network == "mainnet" {
		return w.s.key
	}
	panic("wrong network provided")

}

// Annotate implements api.Wallet.
func (w *Wallet) Annotate(network string, txns []types.Transaction) ([]wallet.PoolTransaction, error) {
	if network == "zen" {
		return w.sZen.Annotate(txns), nil
	}
	if network == "mainnet" {
		return w.s.Annotate(txns), nil
	}
	panic("wrong network provided")
}

// UnspentOutputs implements api.Wallet.
func (w *Wallet) UnspentOutputs(network string) ([]types.SiacoinElement, []types.SiafundElement, error) {
	if network == "zen" {
		return w.sZen.UnspentOutputs()
	}
	if network == "mainnet" {
		return w.s.UnspentOutputs()
	}
	panic("wrong network provided")
}

// Close shuts down the wallet.
func (w *Wallet) Close() {
	if err := w.tg.Stop(); err != nil {
		w.log.Error("unable to stop threads", zap.Error(err))
	}
	w.unsubscribe()
	w.unsubscribeZen()
	w.s.close()
	w.sZen.close()
	w.closeFn()
}

func syncStore(store *DBStore, cm *chain.Manager, index types.ChainIndex) error {
	for index != cm.Tip() {
		crus, caus, err := cm.UpdatesSince(index, 1000)
		if err != nil {
			return fmt.Errorf("failed to subscribe to chain manager: %w", err)
		} else if err := store.updateChainState(crus, caus, caus[len(caus)-1].State.Index == cm.Tip()); err != nil {
			return fmt.Errorf("failed to update chain state: %w", err)
		}
		index = caus[len(caus)-1].State.Index
	}
	return nil
}

// NewWallet returns a wallet that is stored in a MySQL database.
func NewWallet(db *sql.DB, seed, seedZen, dir string, cm *chain.Manager, cmZen *chain.Manager, syncer *syncer.Syncer, syncerZen *syncer.Syncer) (*Wallet, error) {
	l, closeFn, err := persist.NewFileLogger(filepath.Join(dir, "wallet.log"))
	if err != nil {
		log.Fatal(err)
	}

	store, tip, err := NewDBStore(db, seed, "mainnet", l)
	if err != nil {
		return nil, err
	}

	storeZen, tipZen, err := NewDBStore(db, seedZen, "zen", l)
	if err != nil {
		return nil, err
	}

	w := &Wallet{
		db:        db,
		cm:        cm,
		cmZen:     cmZen,
		syncer:    syncer,
		syncerZen: syncerZen,
		s:         store,
		sZen:      storeZen,
		log:       l,
		closeFn:   closeFn,
		locked:    make(map[types.Hash256]time.Time),
	}

	go func() {
		if err := syncStore(w.s, w.cm, tip); err != nil {
			l.Fatal("failed to subscribe to chain manager", zap.String("network", "mainnet"), zap.Error(err))
		}

		reorgChan := make(chan types.ChainIndex, 1)
		w.unsubscribe = w.cm.OnReorg(func(index types.ChainIndex) {
			select {
			case reorgChan <- index:
			default:
			}
		})

		for range reorgChan {
			lastTip := w.s.tip
			if err := syncStore(w.s, w.cm, lastTip); err != nil {
				l.Error("failed to sync store", zap.String("network", "mainnet"), zap.Error(err))
			}
		}
	}()

	go func() {
		if err := syncStore(w.sZen, w.cmZen, tipZen); err != nil {
			l.Fatal("failed to subscribe to chain manager", zap.String("network", "zen"), zap.Error(err))
		}

		reorgChan := make(chan types.ChainIndex, 1)
		w.unsubscribeZen = w.cmZen.OnReorg(func(index types.ChainIndex) {
			select {
			case reorgChan <- index:
			default:
			}
		})

		for range reorgChan {
			lastTip := w.sZen.tip
			if err := syncStore(w.sZen, w.cmZen, lastTip); err != nil {
				l.Error("failed to sync store", zap.String("network", "zen"), zap.Error(err))
			}
		}
	}()

	rows, err := db.Query("SELECT id, until FROM wt_locked")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		id := make([]byte, 32)
		var until int64
		if err := rows.Scan(&id, &until); err != nil {
			return nil, err
		}
		var oid types.Hash256
		copy(oid[:], id)
		w.locked[oid] = time.Unix(until, 0)
	}

	go w.performWalletMaintenance("mainnet")
	go w.performWalletMaintenance("zen")
	go w.pruneLocked()

	return w, nil
}

func (w *Wallet) lock(id types.Hash256, until time.Time) {
	w.locked[id] = until
	_, err := w.db.Exec(`
		INSERT INTO wt_locked (id, until)
		VALUES (?, ?) AS new
		ON DUPLICATE KEY UPDATE until = new.until
	`, id[:], until.Unix())
	if err != nil {
		w.log.Error("couldn't lock input", zap.Stringer("ID", id), zap.Error(err))
	}
}

func (w *Wallet) release(id types.Hash256) {
	delete(w.locked, id)
	_, err := w.db.Exec("DELETE FROM wt_locked WHERE id = ?", id[:])
	if err != nil {
		w.log.Error("couldn't release input", zap.Stringer("ID", id), zap.Error(err))
	}
}

// Fund adds Siacoin inputs with the required amount to the transaction.
func (w *Wallet) Fund(network string, txn *types.Transaction, amount types.Currency, useUnconfirmed bool) (parents []types.Transaction, toSign []types.Hash256, err error) {
	if network != "mainnet" && network != "zen" {
		panic("wrong network provided")
	}
	if amount.IsZero() {
		return nil, nil, nil
	}

	elements, _, err := w.UnspentOutputs(network)
	if err != nil {
		return nil, nil, utils.AddContext(err, "couldn't get utxos to fund transaction")
	}

	tpoolSpent := make(map[types.Hash256]bool)
	tpoolUtxos := make(map[types.Hash256]types.SiacoinElement)
	var poolTransactions []types.Transaction
	var cs consensus.State
	var addr types.Address
	if network == "zen" {
		poolTransactions = w.cmZen.PoolTransactions()
		cs = w.cmZen.TipState()
		addr = w.sZen.addr
	} else {
		poolTransactions = w.cm.PoolTransactions()
		cs = w.cm.TipState()
		addr = w.s.addr
	}
	for _, txn := range poolTransactions {
		for _, sci := range txn.SiacoinInputs {
			tpoolSpent[types.Hash256(sci.ParentID)] = true
			delete(tpoolUtxos, types.Hash256(sci.ParentID))
		}
		for i, sco := range txn.SiacoinOutputs {
			tpoolUtxos[types.Hash256(txn.SiacoinOutputID(i))] = types.SiacoinElement{
				StateElement: types.StateElement{
					ID: types.Hash256(types.SiacoinOutputID(txn.SiacoinOutputID(i))),
				},
				SiacoinOutput: sco,
			}
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Remove immature, locked and spent outputs.
	utxos := make([]types.SiacoinElement, 0, len(elements))
	var usedSum, immatureSum types.Currency
	for _, sce := range elements {
		if time.Now().Before(w.locked[sce.ID]) || tpoolSpent[sce.ID] {
			usedSum = usedSum.Add(sce.SiacoinOutput.Value)
			continue
		} else if cs.Index.Height < sce.MaturityHeight {
			immatureSum = immatureSum.Add(sce.SiacoinOutput.Value)
			continue
		}
		utxos = append(utxos, sce)
	}

	// Sort by value, descending.
	sort.Slice(utxos, func(i, j int) bool {
		return utxos[i].SiacoinOutput.Value.Cmp(utxos[j].SiacoinOutput.Value) > 0
	})

	var unconfirmedUTXOs []types.SiacoinElement
	var unconfirmedSum types.Currency
	if useUnconfirmed {
		for _, sce := range tpoolUtxos {
			if sce.SiacoinOutput.Address != addr || time.Now().Before(w.locked[sce.ID]) {
				continue
			}
			unconfirmedUTXOs = append(unconfirmedUTXOs, sce)
			unconfirmedSum = unconfirmedSum.Add(sce.SiacoinOutput.Value)
		}
	}

	// Sort by value, descending.
	sort.Slice(unconfirmedUTXOs, func(i, j int) bool {
		return unconfirmedUTXOs[i].SiacoinOutput.Value.Cmp(unconfirmedUTXOs[j].SiacoinOutput.Value) > 0
	})

	// Fund the transaction using the largest utxos first.
	var selected []types.SiacoinElement
	var inputSum types.Currency
	for i, sce := range utxos {
		if inputSum.Cmp(amount) >= 0 {
			utxos = utxos[i:]
			break
		}
		selected = append(selected, sce)
		inputSum = inputSum.Add(sce.SiacoinOutput.Value)
	}

	if inputSum.Cmp(amount) < 0 && useUnconfirmed {
		// Try adding unconfirmed utxos.
		for _, sce := range unconfirmedUTXOs {
			selected = append(selected, sce)
			inputSum = inputSum.Add(sce.SiacoinOutput.Value)
			if inputSum.Cmp(amount) >= 0 {
				break
			}
		}

		if inputSum.Cmp(amount) < 0 {
			// Still not enough funds.
			return nil, nil, fmt.Errorf("%w: inputs %v < needed %v (used: %v immature: %v unconfirmed: %v)", ErrInsufficientBalance, inputSum.String(), amount.String(), usedSum.String(), immatureSum.String(), unconfirmedSum.String())
		}
	} else if inputSum.Cmp(amount) < 0 {
		return nil, nil, fmt.Errorf("%w: inputs %v < needed %v (used: %v immature: %v", ErrInsufficientBalance, inputSum.String(), amount.String(), usedSum.String(), immatureSum.String())
	}

	// Check if remaining utxos should be defragged.
	txnInputs := len(txn.SiacoinInputs) + len(selected)
	if len(utxos) > defragThreshold && txnInputs < maxInputsForDefrag {
		// Add the smallest utxos to the transaction.
		defraggable := utxos
		if len(defraggable) > maxDefragUTXOs {
			defraggable = defraggable[len(defraggable)-maxDefragUTXOs:]
		}
		for i := len(defraggable) - 1; i >= 0; i-- {
			if txnInputs >= maxInputsForDefrag {
				break
			}

			sce := defraggable[i]
			selected = append(selected, sce)
			inputSum = inputSum.Add(sce.SiacoinOutput.Value)
			txnInputs++
		}
	}

	// Add a change output if necessary.
	if inputSum.Cmp(amount) > 0 {
		txn.SiacoinOutputs = append(txn.SiacoinOutputs, types.SiacoinOutput{
			Value:   inputSum.Sub(amount),
			Address: addr,
		})
	}

	toSign = make([]types.Hash256, len(selected))
	for i, sce := range selected {
		txn.SiacoinInputs = append(txn.SiacoinInputs, types.SiacoinInput{
			ParentID:         types.SiacoinOutputID(sce.ID),
			UnlockConditions: types.StandardUnlockConditions(w.Key(network).PublicKey()),
		})
		toSign[i] = sce.ID
		w.lock(sce.ID, time.Now().Add(reservationDuration))
	}

	if network == "zen" {
		return w.cmZen.UnconfirmedParents(*txn), toSign, nil
	}
	return w.cm.UnconfirmedParents(*txn), toSign, nil
}

// Release marks the inputs as unused.
func (w *Wallet) Release(txns ...types.Transaction) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.releaseInputs(txns...)
}

func (w *Wallet) releaseInputs(txns ...types.Transaction) {
	for _, txn := range txns {
		for _, in := range txn.SiacoinInputs {
			w.release(types.Hash256(in.ParentID))
		}
		for _, in := range txn.SiafundInputs {
			w.release(types.Hash256(in.ParentID))
		}
	}
}

// Sign adds signatures corresponding to toSign elements to the transaction.
func (w *Wallet) Sign(network string, txn *types.Transaction, toSign []types.Hash256, cf types.CoveredFields) {
	if network != "mainnet" && network != "zen" {
		panic("wrong network provided")
	}
	var cs consensus.State
	if network == "zen" {
		cs = w.cmZen.TipState()
	} else {
		cs = w.cm.TipState()
	}
	for _, id := range toSign {
		var h types.Hash256
		if cf.WholeTransaction {
			h = cs.WholeSigHash(*txn, id, 0, 0, cf.Signatures)
		} else {
			h = cs.PartialSigHash(*txn, cf)
		}
		sig := w.Key(network).SignHash(h)
		txn.Signatures = append(txn.Signatures, types.TransactionSignature{
			ParentID:       id,
			CoveredFields:  cf,
			PublicKeyIndex: 0,
			Signature:      sig[:],
		})
	}
}

// Redistribute creates a specified number of new outputs and distributes
// the funds between them.
func (w *Wallet) Redistribute(network string, amount types.Currency, outputs int) error {
	if network != "mainnet" && network != "zen" {
		panic("wrong network provided")
	}
	if outputs == 0 {
		return errors.New("number of outputs must be greater than zero")
	}

	var cs consensus.State
	var fee types.Currency
	var pool []types.Transaction

	if network == "zen" {
		cs = w.cmZen.TipState()
		fee = w.cmZen.RecommendedFee()
		pool = w.cmZen.PoolTransactions()
	} else {
		cs = w.cm.TipState()
		fee = w.cm.RecommendedFee()
		pool = w.cm.PoolTransactions()
	}
	height := cs.Index.Height

	// Fetch unspent transaction outputs.
	elements, _, err := w.UnspentOutputs(network)
	if err != nil {
		return err
	}

	// Build map of inputs currently in the tx pool.
	inPool := make(map[types.Hash256]bool)
	for _, ptxn := range pool {
		for _, in := range ptxn.SiacoinInputs {
			inPool[types.Hash256(in.ParentID)] = true
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Adjust the number of desired outputs for any output we encounter that is
	// unused, matured and has the same value.
	utxos := make([]types.SiacoinElement, 0, len(elements))
	for _, sce := range elements {
		inUse := time.Now().Before(w.locked[sce.ID]) || inPool[sce.ID]
		matured := height >= sce.MaturityHeight
		sameValue := sce.SiacoinOutput.Value.Equals(amount)

		// Adjust number of desired outputs.
		if !inUse && matured && sameValue {
			outputs--
		}

		// Collect usable outputs for defragging.
		if !inUse && matured && !sameValue {
			utxos = append(utxos, sce)
		}
	}

	// Return early if we don't have to defrag at all.
	if outputs <= 0 {
		return nil
	}

	// Desc sort.
	sort.Slice(utxos, func(i, j int) bool {
		return utxos[i].SiacoinOutput.Value.Cmp(utxos[j].SiacoinOutput.Value) > 0
	})

	// Prepare all outputs.
	var txns []types.Transaction

	for outputs > 0 {
		var txn types.Transaction
		var toSign []types.Hash256
		for i := 0; i < outputs && i < redistributeBatchSize; i++ {
			txn.SiacoinOutputs = append(txn.SiacoinOutputs, types.SiacoinOutput{
				Value:   amount,
				Address: w.Address(network),
			})
		}
		outputs -= len(txn.SiacoinOutputs)

		// Estimate the fees.
		outputFees := fee.Mul64(cs.TransactionWeight(txn))
		feePerInput := fee.Mul64(bytesPerInput)

		// Collect outputs that cover the total amount.
		var inputs []types.SiacoinElement
		want := amount.Mul64(uint64(len(txn.SiacoinOutputs)))
		for _, sce := range utxos {
			inUse := time.Now().Before(w.locked[sce.ID]) || inPool[sce.ID]
			if inUse {
				continue
			}
			inputs = append(inputs, sce)
			fee := feePerInput.Mul64(uint64(len(inputs))).Add(outputFees)
			if SumOutputs(inputs).Cmp(want.Add(fee)) > 0 {
				break
			}
		}

		// Not enough outputs found.
		fee := feePerInput.Mul64(uint64(len(inputs))).Add(outputFees)
		if sumOut := SumOutputs(inputs); sumOut.Cmp(want.Add(fee)) < 0 {
			// If there is at least one transaction in the set, submit it.
			if len(txns) > 0 {
				break
			}

			// In case of an error we need to free all inputs.
			w.releaseInputs(txns...)
			return fmt.Errorf("network: %s: %w, inputs %v < needed %v + txnFee %v",
				network, ErrInsufficientBalance, sumOut.String(), want.String(), fee.String())
		}

		// Set the miner fee.
		txn.MinerFees = []types.Currency{fee}

		// Add the change output.
		change := SumOutputs(inputs).Sub(want.Add(fee))
		if !change.IsZero() {
			txn.SiacoinOutputs = append(txn.SiacoinOutputs, types.SiacoinOutput{
				Value:   change,
				Address: w.Address(network),
			})
		}

		// Add the inputs.
		for _, sce := range inputs {
			txn.SiacoinInputs = append(txn.SiacoinInputs, types.SiacoinInput{
				ParentID:         types.SiacoinOutputID(sce.ID),
				UnlockConditions: types.StandardUnlockConditions(w.Key(network).PublicKey()),
			})
			toSign = append(toSign, sce.ID)
			w.lock(sce.ID, time.Now().Add(reservationDuration))
		}

		w.Sign(network, &txn, toSign, types.CoveredFields{WholeTransaction: true})
		txns = append(txns, txn)
	}

	if network == "zen" {
		_, err = w.cmZen.AddPoolTransactions(txns)
	} else {
		_, err = w.cm.AddPoolTransactions(txns)
	}
	if err != nil {
		w.releaseInputs(txns...)
		return utils.AddContext(err, "invalid transaction set")
	}
	if network == "zen" {
		w.syncerZen.BroadcastTransactionSet(txns)
	} else {
		w.syncer.BroadcastTransactionSet(txns)
	}

	return nil
}

// SumOutputs returns the total value of the supplied outputs.
func SumOutputs(outputs []types.SiacoinElement) (sum types.Currency) {
	for _, o := range outputs {
		sum = sum.Add(o.SiacoinOutput.Value)
	}
	return
}

// synced returns true if the wallet is synced to the blockchain.
func (w *Wallet) synced(network string) bool {
	isSynced := func(s *syncer.Syncer) bool {
		var count int
		for _, p := range s.Peers() {
			if p.Synced() {
				count++
			}
		}
		return count >= 5
	}
	if network == "zen" {
		return isSynced(w.syncerZen) && time.Since(w.cmZen.TipState().PrevTimestamps[0]) < 24*time.Hour
	}
	if network == "mainnet" {
		return isSynced(w.syncer) && time.Since(w.cm.TipState().PrevTimestamps[0]) < 24*time.Hour
	}
	panic("wrong network provided")
}

// performWalletMaintenance performs the wallet maintenance periodically.
func (w *Wallet) performWalletMaintenance(network string) {
	redistribute := func() {
		if (network == "zen" && relevantTransactions(w.cmZen.PoolTransactions(), w.sZen.addr)) ||
			(network == "mainnet" && relevantTransactions(w.cm.PoolTransactions(), w.s.addr)) {
			return
		}
		utxos, _, err := w.UnspentOutputs(network)
		if err != nil {
			w.log.Error("couldn't get unspent outputs", zap.Error(err))
			return
		}
		balance := SumOutputs(utxos)
		amount := balance.Div64(wantedOutputs).Div64(2)
		err = w.Redistribute(network, amount, wantedOutputs)
		if err != nil {
			w.log.Error("failed to redistribute wallet", zap.String("network", network), zap.Int("outputs", wantedOutputs), zap.Stringer("amount", amount), zap.Stringer("balance", balance), zap.Error(err))
			return
		}
	}

	if err := w.tg.Add(); err != nil {
		w.log.Error("couldn't add a thread", zap.Error(err))
		return
	}
	defer w.tg.Done()

	for {
		if w.synced(network) {
			break
		}
		select {
		case <-w.tg.StopChan():
			return
		case <-time.After(30 * time.Second):
		}
	}

	redistribute()

	for {
		select {
		case <-w.tg.StopChan():
			return
		case <-time.After(walletMaintenanceInterval):
			redistribute()
		}
	}
}

// relevantTransactions returns true if there is at least one relevant
// transaction in the transaction set.
func relevantTransactions(txnSet []types.Transaction, addr types.Address) bool {
	for _, txn := range txnSet {
		ptxn := wallet.Annotate(txn, addr)
		if ptxn.Type != "unrelated" {
			return true
		}
	}
	return false
}

// pruneLocked periodically cleans the locked map from the expired elements.
func (w *Wallet) pruneLocked() {
	if err := w.tg.Add(); err != nil {
		w.log.Error("couldn't add a thread", zap.Error(err))
		return
	}
	defer w.tg.Done()

	for {
		select {
		case <-w.tg.StopChan():
			return
		case <-time.After(time.Hour):
			_, err := w.db.Exec(`
				DELETE FROM wt_locked
				WHERE until < ?
			`, time.Now().Unix())
			if err != nil {
				w.log.Error("couldn't delete expired outputs", zap.Error(err))
			}
			w.mu.Lock()
			for id, until := range w.locked {
				if time.Now().After(until) {
					delete(w.locked, id)
				}
			}
			w.mu.Unlock()
		}
	}
}

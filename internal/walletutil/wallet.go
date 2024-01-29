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
	"github.com/mike76-dev/hostscore/syncer"
	"github.com/mike76-dev/hostscore/wallet"
	"gitlab.com/NebulousLabs/encoding"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
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
	wantedOutputs = 10
)

// ErrInsufficientBalance is returned when there aren't enough unused outputs to
// cover the requested amount.
var ErrInsufficientBalance = errors.New("insufficient balance")

type Wallet struct {
	s      *DBStore
	cm     *chain.Manager
	syncer *syncer.Syncer
	log    *persist.Logger

	mu   sync.Mutex
	tg   siasync.ThreadGroup
	used map[types.Hash256]bool
}

// Address implements api.Wallet.
func (w *Wallet) Address() types.Address {
	return w.s.Address()
}

// Key implements api.Wallet.
func (w *Wallet) Key() types.PrivateKey {
	return w.s.key
}

// Annotate implements api.Wallet.
func (w *Wallet) Annotate(txns []types.Transaction) ([]wallet.PoolTransaction, error) {
	return w.s.Annotate(txns), nil
}

// UnspentOutputs implements api.Wallet.
func (w *Wallet) UnspentOutputs() ([]types.SiacoinElement, []types.SiafundElement, error) {
	return w.s.UnspentOutputs()
}

// Close shuts down the wallet.
func (w *Wallet) Close() {
	if err := w.tg.Stop(); err != nil {
		w.log.Println("[ERROR] unable to stop threads:", err)
	}
	w.s.close()
}

// NewWallet returns a wallet that is stored in a MySQL database.
func NewWallet(db *sql.DB, seed, network, dir string, cm *chain.Manager, syncer *syncer.Syncer) (*Wallet, error) {
	l, err := persist.NewFileLogger(filepath.Join(dir, "wallet.log"))
	if err != nil {
		log.Fatal(err)
	}
	store, tip, err := NewDBStore(db, seed, network, l)
	if err != nil {
		return nil, err
	}
	if err := cm.AddSubscriber(store, tip); err != nil {
		return nil, err
	}

	w := &Wallet{
		cm:     cm,
		syncer: syncer,
		log:    l,
		s:      store,
		used:   make(map[types.Hash256]bool),
	}

	go w.performWalletMaintenance()

	return w, nil
}

// Fund adds Siacoin inputs with the required amount to the transaction.
func (w *Wallet) Fund(txn *types.Transaction, amount types.Currency) (parents []types.Transaction, toSign []types.Hash256, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if amount.IsZero() {
		return nil, nil, nil
	}

	utxos, _, err := w.UnspentOutputs()
	if err != nil {
		return nil, nil, utils.AddContext(err, "couldn't get utxos to fund transaction")
	}

	sort.Slice(utxos, func(i, j int) bool {
		return utxos[i].SiacoinOutput.Value.Cmp(utxos[j].SiacoinOutput.Value) > 0
	})

	inPool := make(map[types.Hash256]bool)
	for _, ptxn := range w.cm.PoolTransactions() {
		for _, in := range ptxn.SiacoinInputs {
			inPool[types.Hash256(in.ParentID)] = true
		}
	}

	var outputSum types.Currency
	var fundingElements []types.SiacoinElement
	for _, sce := range utxos {
		if w.used[types.Hash256(sce.ID)] || inPool[types.Hash256(sce.ID)] {
			continue
		}
		fundingElements = append(fundingElements, sce)
		outputSum = outputSum.Add(sce.SiacoinOutput.Value)
		if outputSum.Cmp(amount) >= 0 {
			break
		}
	}

	if outputSum.Cmp(amount) < 0 {
		return nil, nil, errors.New("insufficient balance")
	} else if outputSum.Cmp(amount) > 0 {
		txn.SiacoinOutputs = append(txn.SiacoinOutputs, types.SiacoinOutput{
			Value:   outputSum.Sub(amount),
			Address: w.Address(),
		})
	}

	toSign = make([]types.Hash256, len(fundingElements))
	for i, sce := range fundingElements {
		txn.SiacoinInputs = append(txn.SiacoinInputs, types.SiacoinInput{
			ParentID:         types.SiacoinOutputID(sce.ID),
			UnlockConditions: types.StandardUnlockConditions(w.Key().PublicKey()),
		})
		toSign[i] = types.Hash256(sce.ID)
		w.used[types.Hash256(sce.ID)] = true
	}

	return w.cm.UnconfirmedParents(*txn), toSign, nil
}

// Release marks the outputs as unused.
func (w *Wallet) Release(txnSet []types.Transaction) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, txn := range txnSet {
		for i := range txn.SiacoinOutputs {
			delete(w.used, types.Hash256(txn.SiacoinOutputID(i)))
		}
		for i := range txn.SiafundOutputs {
			delete(w.used, types.Hash256(txn.SiafundOutputID(i)))
		}
	}
}

// Reserve reserves the outputs for a defined amount of time.
func (w *Wallet) Reserve(scoids []types.SiacoinOutputID, sfoids []types.SiafundOutputID, duration time.Duration) error {
	w.mu.Lock()
	for _, id := range scoids {
		if w.used[types.Hash256(id)] {
			w.mu.Unlock()
			return fmt.Errorf("output %v is already reserved", id)
		}
		w.used[types.Hash256(id)] = true
	}
	for _, id := range sfoids {
		if w.used[types.Hash256(id)] {
			w.mu.Unlock()
			return fmt.Errorf("output %v is already reserved", id)
		}
		w.used[types.Hash256(id)] = true
	}
	w.mu.Unlock()

	if duration == 0 {
		duration = 10 * time.Minute
	}
	time.AfterFunc(duration, func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		for _, id := range scoids {
			delete(w.used, types.Hash256(id))
		}
		for _, id := range sfoids {
			delete(w.used, types.Hash256(id))
		}
	})

	return nil
}

// Sign adds signatures corresponding to toSign elements to the transaction.
func (w *Wallet) Sign(txn *types.Transaction, toSign []types.Hash256, cf types.CoveredFields) error {
	cs := w.cm.TipState()
	for _, id := range toSign {
		ts := types.TransactionSignature{
			ParentID:       id,
			CoveredFields:  cf,
			PublicKeyIndex: 0,
		}
		var h types.Hash256
		if cf.WholeTransaction {
			h = cs.WholeSigHash(*txn, ts.ParentID, ts.PublicKeyIndex, ts.Timelock, cf.Signatures)
		} else {
			h = cs.PartialSigHash(*txn, cf)
		}
		sig := w.Key().SignHash(h)
		ts.Signature = sig[:]
		txn.Signatures = append(txn.Signatures, ts)
	}
	return nil
}

// Redistribute creates a specified number of new outputs and distributes
// the funds between them.
func (w *Wallet) Redistribute(amount types.Currency, outputs int) error {
	if outputs == 0 {
		return errors.New("number of outputs must be greater than zero")
	}

	cs := w.cm.TipState()
	fee := w.cm.RecommendedFee()
	pool := w.cm.PoolTransactions()

	// Build map of inputs currently in the tx pool.
	inPool := make(map[types.Hash256]bool)
	for _, ptxn := range pool {
		for _, in := range ptxn.SiacoinInputs {
			inPool[types.Hash256(in.ParentID)] = true
		}
	}

	// Fetch unspent transaction outputs.
	utxos, _, err := w.UnspentOutputs()
	if err != nil {
		return err
	}

	// Check whether a redistribution is necessary, adjust number of desired
	// outputs accordingly.
	w.mu.Lock()
	for _, sce := range utxos {
		inUse := w.used[sce.ID] || inPool[sce.ID]
		matured := cs.Index.Height >= sce.MaturityHeight
		sameValue := sce.SiacoinOutput.Value.Equals(amount)
		if !inUse && matured && sameValue {
			outputs--
		}
	}
	w.mu.Unlock()
	if outputs <= 0 {
		return nil
	}

	// Desc sort.
	sort.Slice(utxos, func(i, j int) bool {
		return utxos[i].SiacoinOutput.Value.Cmp(utxos[j].SiacoinOutput.Value) > 0
	})

	// Prepare all outputs.
	var txns []types.Transaction
	var toSign []types.Hash256

	for outputs > 0 {
		var txn types.Transaction
		for i := 0; i < outputs && i < redistributeBatchSize; i++ {
			txn.SiacoinOutputs = append(txn.SiacoinOutputs, types.SiacoinOutput{
				Value:   amount,
				Address: w.Address(),
			})
		}
		outputs -= len(txn.SiacoinOutputs)

		// Estimate the fees.
		outputFees := fee.Mul64(uint64(len(encoding.Marshal(txn.SiacoinOutputs))))
		feePerInput := fee.Mul64(bytesPerInput)

		// Collect outputs that cover the total amount.
		var inputs []types.SiacoinElement
		want := amount.Mul64(uint64(len(txn.SiacoinOutputs)))
		var amtInUse, amtSameValue, amtNotMatured types.Currency
		w.mu.Lock()
		for _, sce := range utxos {
			inUse := w.used[sce.ID] || inPool[sce.ID]
			matured := cs.Index.Height >= sce.MaturityHeight
			sameValue := sce.SiacoinOutput.Value.Equals(amount)
			if inUse {
				amtInUse = amtInUse.Add(sce.SiacoinOutput.Value)
				continue
			} else if sameValue {
				amtSameValue = amtSameValue.Add(sce.SiacoinOutput.Value)
				continue
			} else if !matured {
				amtNotMatured = amtNotMatured.Add(sce.SiacoinOutput.Value)
				continue
			}

			inputs = append(inputs, sce)
			inPool[sce.ID] = true
			fee := feePerInput.Mul64(uint64(len(inputs))).Add(outputFees)
			if SumOutputs(inputs).Cmp(want.Add(fee)) > 0 {
				break
			}
		}
		w.mu.Unlock()

		// Not enough outputs found.
		fee := feePerInput.Mul64(uint64(len(inputs))).Add(outputFees)
		if sumOut := SumOutputs(inputs); sumOut.Cmp(want.Add(fee)) < 0 {
			// In case of an error we need to free all inputs.
			w.Release(txns)
			return fmt.Errorf("%w: inputs %v < needed %v + txnFee %v (usable: %v, inUse: %v, sameValue: %v, notMatured: %v)",
				ErrInsufficientBalance, sumOut.String(), want.String(), fee.String(), sumOut.String(), amtInUse.String(), amtSameValue.String(), amtNotMatured.String())
		}

		// Set the miner fee.
		txn.MinerFees = []types.Currency{fee}

		// Add the change output.
		change := SumOutputs(inputs).Sub(want.Add(fee))
		if !change.IsZero() {
			txn.SiacoinOutputs = append(txn.SiacoinOutputs, types.SiacoinOutput{
				Value:   change,
				Address: w.Address(),
			})
		}

		// Add the inputs.
		for _, sce := range inputs {
			txn.SiacoinInputs = append(txn.SiacoinInputs, types.SiacoinInput{
				ParentID:         types.SiacoinOutputID(sce.ID),
				UnlockConditions: types.StandardUnlockConditions(w.Key().PublicKey()),
			})
			toSign = append(toSign, sce.ID)
		}

		txns = append(txns, txn)
	}

	for i := 0; i < len(txns); i++ {
		err = w.Sign(&txns[i], toSign, types.CoveredFields{WholeTransaction: true})
		if err != nil {
			w.Release(txns)
			return utils.AddContext(err, "couldn't sign the transaction")
		}
	}

	_, err = w.cm.AddPoolTransactions(txns)
	if err != nil {
		w.Release(txns)
		return utils.AddContext(err, "invalid transaction set")
	}
	w.syncer.BroadcastTransactionSet(txns)

	return nil
}

// SumOutputs returns the total value of the supplied outputs.
func SumOutputs(outputs []types.SiacoinElement) (sum types.Currency) {
	for _, o := range outputs {
		sum = sum.Add(o.SiacoinOutput.Value)
	}
	return
}

// performWalletMaintenance performs the wallet maintenance periodically.
func (w *Wallet) performWalletMaintenance() {
	redistribute := func() {
		w.log.Println("[DEBUG] starting wallet maintenance")
		if len(w.cm.PoolTransactions()) > 0 {
			w.log.Println("[DEBUG] pending transactions found, skipping")
			return
		}
		utxos, _, err := w.UnspentOutputs()
		if err != nil {
			w.log.Println("[ERROR] couldn't get unspent outputs:", err)
			return
		}
		balance := SumOutputs(utxos)
		amount := balance.Div64(wantedOutputs).Div64(2)
		err = w.Redistribute(amount, wantedOutputs)
		if err != nil {
			w.log.Printf("[ERROR] failed to redistribute wallet into %d outputs of amount %v, balance %v: %v", wantedOutputs, amount, balance, err)
			return
		}
		w.log.Println("[DEBUG] wallet maintenance succeeded")
	}

	if err := w.tg.Add(); err != nil {
		w.log.Println("[ERROR] couldn't add a thread:", err)
		return
	}
	defer w.tg.Done()

	for {
		if w.syncer.Synced() {
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

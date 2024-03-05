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
	"gitlab.com/NebulousLabs/encoding"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/coreutils/syncer"
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

// ErrInsufficientBalance is returned when there aren't enough unused outputs to
// cover the requested amount.
var ErrInsufficientBalance = errors.New("insufficient balance")

type Wallet struct {
	s         *DBStore
	sZen      *DBStore
	cm        *chain.Manager
	cmZen     *chain.Manager
	syncer    *syncer.Syncer
	syncerZen *syncer.Syncer
	log       *persist.Logger

	mu   sync.Mutex
	tg   siasync.ThreadGroup
	used map[types.Hash256]bool
}

// Address implements api.Wallet.
func (w *Wallet) Address(network string) types.Address {
	if network == "zen" {
		return w.sZen.Address()
	}
	return w.s.Address()
}

// Key implements api.Wallet.
func (w *Wallet) Key(network string) types.PrivateKey {
	if network == "zen" {
		return w.sZen.key
	}
	return w.s.key
}

// Annotate implements api.Wallet.
func (w *Wallet) Annotate(network string, txns []types.Transaction) ([]wallet.PoolTransaction, error) {
	if network == "zen" {
		return w.sZen.Annotate(txns), nil
	}
	return w.s.Annotate(txns), nil
}

// UnspentOutputs implements api.Wallet.
func (w *Wallet) UnspentOutputs(network string) ([]types.SiacoinElement, []types.SiafundElement, error) {
	if network == "zen" {
		return w.sZen.UnspentOutputs()
	}
	return w.s.UnspentOutputs()
}

// Close shuts down the wallet.
func (w *Wallet) Close() {
	if err := w.tg.Stop(); err != nil {
		w.log.Println("[ERROR] unable to stop threads:", err)
	}
	w.s.close()
	w.sZen.close()
	w.log.Close()
}

// NewWallet returns a wallet that is stored in a MySQL database.
func NewWallet(db *sql.DB, seed, seedZen, dir string, cm *chain.Manager, cmZen *chain.Manager, syncer *syncer.Syncer, syncerZen *syncer.Syncer) (*Wallet, error) {
	l, err := persist.NewFileLogger(filepath.Join(dir, "wallet.log"))
	if err != nil {
		log.Fatal(err)
	}

	store, tip, err := NewDBStore(db, seed, "mainnet", l)
	if err != nil {
		return nil, err
	}
	if err := cm.AddSubscriber(store, tip); err != nil {
		return nil, err
	}

	storeZen, tipZen, err := NewDBStore(db, seedZen, "zen", l)
	if err != nil {
		return nil, err
	}
	if err := cmZen.AddSubscriber(storeZen, tipZen); err != nil {
		return nil, err
	}

	w := &Wallet{
		cm:        cm,
		cmZen:     cmZen,
		syncer:    syncer,
		syncerZen: syncerZen,
		log:       l,
		s:         store,
		sZen:      storeZen,
		used:      make(map[types.Hash256]bool),
	}

	go w.performWalletMaintenance("mainnet")
	go w.performWalletMaintenance("zen")

	return w, nil
}

// Fund adds Siacoin inputs with the required amount to the transaction.
func (w *Wallet) Fund(network string, txn *types.Transaction, amount types.Currency) (parents []types.Transaction, toSign []types.Hash256, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if amount.IsZero() {
		return nil, nil, nil
	}

	utxos, _, err := w.UnspentOutputs(network)
	if err != nil {
		return nil, nil, utils.AddContext(err, "couldn't get utxos to fund transaction")
	}

	sort.Slice(utxos, func(i, j int) bool {
		return utxos[i].SiacoinOutput.Value.Cmp(utxos[j].SiacoinOutput.Value) > 0
	})

	inPool := make(map[types.Hash256]bool)
	var txns []types.Transaction
	if network == "zen" {
		txns = w.cmZen.PoolTransactions()
	} else {
		txns = w.cm.PoolTransactions()
	}
	for _, ptxn := range txns {
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
			Address: w.Address(network),
		})
	}

	toSign = make([]types.Hash256, len(fundingElements))
	for i, sce := range fundingElements {
		txn.SiacoinInputs = append(txn.SiacoinInputs, types.SiacoinInput{
			ParentID:         types.SiacoinOutputID(sce.ID),
			UnlockConditions: types.StandardUnlockConditions(w.Key(network).PublicKey()),
		})
		toSign[i] = types.Hash256(sce.ID)
		w.used[types.Hash256(sce.ID)] = true
	}

	if network == "zen" {
		return w.cmZen.UnconfirmedParents(*txn), toSign, nil
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
func (w *Wallet) Sign(network string, txn *types.Transaction, toSign []types.Hash256, cf types.CoveredFields) error {
	var cs consensus.State
	if network == "zen" {
		cs = w.cmZen.TipState()
	} else {
		cs = w.cm.TipState()
	}
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
		sig := w.Key(network).SignHash(h)
		ts.Signature = sig[:]
		txn.Signatures = append(txn.Signatures, ts)
	}
	return nil
}

// Redistribute creates a specified number of new outputs and distributes
// the funds between them.
func (w *Wallet) Redistribute(network string, amount types.Currency, outputs int) error {
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

	// Build map of inputs currently in the tx pool.
	inPool := make(map[types.Hash256]bool)
	for _, ptxn := range pool {
		for _, in := range ptxn.SiacoinInputs {
			inPool[types.Hash256(in.ParentID)] = true
		}
	}

	// Fetch unspent transaction outputs.
	utxos, _, err := w.UnspentOutputs(network)
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
			return fmt.Errorf("%w: inputs %v < needed %v + txnFee %v (usable: %v, inUse: %v, sameValue: %v, notMatured: %v, network: %s)",
				ErrInsufficientBalance, sumOut.String(), want.String(), fee.String(), sumOut.String(), amtInUse.String(), amtSameValue.String(), amtNotMatured.String(), network)
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
		}

		err = w.Sign(network, &txn, toSign, types.CoveredFields{WholeTransaction: true})
		txns = append(txns, txn)
		if err != nil {
			w.Release(txns)
			return utils.AddContext(err, "couldn't sign the transaction")
		}
	}

	if network == "zen" {
		_, err = w.cmZen.AddPoolTransactions(txns)
	} else {
		_, err = w.cm.AddPoolTransactions(txns)
	}
	if err != nil {
		w.Release(txns)
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
	return isSynced(w.syncer) && time.Since(w.cm.TipState().PrevTimestamps[0]) < 24*time.Hour
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
			w.log.Println("[ERROR] couldn't get unspent outputs:", err)
			return
		}
		balance := SumOutputs(utxos)
		amount := balance.Div64(wantedOutputs).Div64(2)
		err = w.Redistribute(network, amount, wantedOutputs)
		if err != nil {
			w.log.Printf("[ERROR] failed to redistribute %s wallet into %d outputs of amount %v, balance %v: %v", network, wantedOutputs, amount, balance, err)
			return
		}
	}

	if err := w.tg.Add(); err != nil {
		w.log.Println("[ERROR] couldn't add a thread:", err)
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

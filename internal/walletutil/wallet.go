package walletutil

import (
	"database/sql"
	"log"
	"path/filepath"

	"github.com/mike76-dev/hostscore/persist"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/chain"
	"go.sia.tech/core/types"
)

type ChainManager interface {
	AddSubscriber(s chain.Subscriber, tip types.ChainIndex) error
	BestIndex(height uint64) (types.ChainIndex, bool)
}

type Wallet struct {
	s  *DBStore
	cm ChainManager
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
	w.s.close()
}

// NewWallet returns a wallet that is stored in a MyQL database.
func NewWallet(db *sql.DB, seed, network, dir string, cm ChainManager) (*Wallet, error) {
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
		cm: cm,
		s:  store,
	}

	return w, nil
}

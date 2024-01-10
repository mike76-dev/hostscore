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
	RemoveSubscriber(s chain.Subscriber)
	BestIndex(height uint64) (types.ChainIndex, bool)
}

type EphemeralWallet struct {
	s  *EphemeralStore
	cm ChainManager
}

// NewEphemeralWallet creates a new EphemeralWallet.
func NewEphemeralWallet(cm ChainManager, seed string) *EphemeralWallet {
	store := NewEphemeralStore(seed)
	return &EphemeralWallet{store, cm}
}

// Addresses implements api.Wallet.
func (ew *EphemeralWallet) Address() types.Address {
	return ew.s.Address()
}

// Key implements api.Wallet.
func (ew *EphemeralWallet) Key() types.PrivateKey {
	return ew.s.key
}

// Events implements api.Wallet.
func (ew *EphemeralWallet) Events(offset, limit int) ([]wallet.Event, error) {
	return ew.s.Events(offset, limit)
}

// Annotate implements api.Wallet.
func (ew *EphemeralWallet) Annotate(txns []types.Transaction) ([]wallet.PoolTransaction, error) {
	return ew.s.Annotate(txns), nil
}

// UnspentOutputs implements api.Wallet.
func (ew *EphemeralWallet) UnspentOutputs() ([]types.SiacoinElement, []types.SiafundElement, error) {
	return ew.s.UnspentOutputs()
}

type JSONWallet struct {
	s  *JSONStore
	cm ChainManager
}

// Address implements api.Wallet.
func (w *JSONWallet) Address() types.Address {
	return w.s.Address()
}

// Key implements api.Wallet.
func (w *JSONWallet) Key() types.PrivateKey {
	return w.s.key
}

// Events implements api.Wallet.
func (w *JSONWallet) Events(offset, limit int) ([]wallet.Event, error) {
	return w.s.Events(offset, limit)
}

// Annotate implements api.Wallet.
func (w *JSONWallet) Annotate(txns []types.Transaction) ([]wallet.PoolTransaction, error) {
	return w.s.Annotate(txns), nil
}

// UnspentOutputs implements api.Wallet.
func (w *JSONWallet) UnspentOutputs() ([]types.SiacoinElement, []types.SiafundElement, error) {
	return w.s.UnspentOutputs()
}

// NewJSONWallet returns a wallet that is stored in the specified directory.
func NewJSONWallet(seed, dir string, cm ChainManager) (*JSONWallet, error) {
	store, tip, err := NewJSONStore(seed, filepath.Join(dir, "wallet.json"))
	if err != nil {
		return nil, err
	}
	if err := cm.AddSubscriber(store, tip); err != nil {
		return nil, err
	}

	w := &JSONWallet{
		cm: cm,
		s:  store,
	}

	return w, nil
}

type DBWallet struct {
	s   *DBStore
	cm  ChainManager
	log *persist.Logger
}

// Address implements api.Wallet.
func (w *DBWallet) Address() types.Address {
	return w.s.Address()
}

// Key implements api.Wallet.
func (w *DBWallet) Key() types.PrivateKey {
	return w.s.key
}

// Events implements api.Wallet.
func (w *DBWallet) Events(offset, limit int) ([]wallet.Event, error) {
	return w.s.Events(offset, limit)
}

// Annotate implements api.Wallet.
func (w *DBWallet) Annotate(txns []types.Transaction) ([]wallet.PoolTransaction, error) {
	return w.s.Annotate(txns), nil
}

// UnspentOutputs implements api.Wallet.
func (w *DBWallet) UnspentOutputs() ([]types.SiacoinElement, []types.SiafundElement, error) {
	return w.s.UnspentOutputs()
}

// Close shuts down the wallet.
func (w *DBWallet) Close() {
	w.s.close()
	w.log.Close()
}

// NewDBWallet returns a wallet that is stored in a MyQL database.
func NewDBWallet(db *sql.DB, seed, network, dir string, cm ChainManager) (*DBWallet, error) {
	store, tip, err := NewDBStore(db, seed, network)
	if err != nil {
		return nil, err
	}
	l, err := persist.NewFileLogger(filepath.Join(dir, "wallet.log"))
	if err != nil {
		log.Fatal(err)
	}
	if err := cm.AddSubscriber(store, tip); err != nil {
		return nil, err
	}

	w := &DBWallet{
		cm:  cm,
		s:   store,
		log: l,
	}

	return w, nil
}

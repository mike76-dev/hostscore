package wallet

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/wallet"
	"go.uber.org/zap"
)

// A DBStore stores wallet state in a MySQL database.
type DBStore struct {
	tip           types.ChainIndex
	addr          types.Address
	key           types.PrivateKey
	sces          map[types.SiacoinOutputID]types.SiacoinElement
	mu            sync.Mutex
	db            *sql.DB
	tx            *sql.Tx
	log           *zap.Logger
	network       string
	lastCommitted time.Time
}

func (s *DBStore) save() error {
	if s.tx == nil {
		return errors.New("there is no transaction")
	}

	_, err := s.tx.Exec(`
		REPLACE INTO wt_tip (network, height, bid)
		VALUES (?, ?, ?)
	`, s.network, s.tip.Height, s.tip.ID[:])
	if err != nil {
		s.tx.Rollback()
		s.tx, _ = s.db.Begin()
		return utils.AddContext(err, "couldn't update tip")
	}

	err = s.tx.Commit()
	if err != nil {
		return utils.AddContext(err, "couldn't commit transaction")
	}

	s.tx, err = s.db.Begin()
	s.lastCommitted = time.Now()
	return err
}

func (s *DBStore) load() error {
	var height uint64
	id := make([]byte, 32)
	err := s.db.QueryRow(`
		SELECT height, bid
		FROM wt_tip
		WHERE network = ?
	`, s.network).Scan(&height, &id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return utils.AddContext(err, "couldn't load tip")
	}
	s.tip.Height = height
	copy(s.tip.ID[:], id)

	rows, err := s.db.Query(`
		SELECT scoid, bytes
		FROM wt_sces
		WHERE network = ?
	`, s.network)
	if err != nil {
		return utils.AddContext(err, "couldn't query SC elements")
	}

	var b []byte
	for rows.Next() {
		if err := rows.Scan(&id, &b); err != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't scan SC element")
		}
		var scoid types.SiacoinOutputID
		copy(scoid[:], id)
		d := types.NewBufDecoder(b)
		var sce types.SiacoinElement
		sce.DecodeFrom(d)
		if d.Err() != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't decode SC element")
		}
		s.sces[scoid] = sce
	}
	rows.Close()

	s.tx, err = s.db.Begin()
	return err
}

// Tip implements wallet.SingleAddressStore.
func (s *DBStore) Tip() (types.ChainIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.tip, nil
}

// UnspentSiacoinElements implements wallet.SingleAddressStore.
func (s *DBStore) UnspentSiacoinElements() (utxos []types.SiacoinElement, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sce := range s.sces {
		utxos = append(utxos, sce)
	}

	return
}

// WalletEventCount implements wallet.SingleAddressStore.
func (s *DBStore) WalletEventCount() (uint64, error) {
	return 0, nil
}

// WalletEvents implements wallet.SingleAddressStore.
func (s *DBStore) WalletEvents(int, int) ([]wallet.Event, error) {
	return nil, nil
}

func (s *DBStore) resetChainState() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tx == nil {
		var err error
		s.tx, err = s.db.Begin()
		if err != nil {
			return err
		}
	}

	_, err := s.tx.Exec("DELETE FROM wt_sces WHERE network = ?", s.network)
	if err != nil {
		return err
	}

	s.tip = types.ChainIndex{}
	return s.save()
}

func (s *DBStore) getSiacoinElements() (sces []types.SiacoinElement) {
	for _, sce := range s.sces {
		sces = append(sces, sce)
	}

	return
}

func (s *DBStore) updateSiacoinElements(sces []types.SiacoinElement) error {
	for _, sce := range sces {
		sce.StateElement.MerkleProof = append([]types.Hash256(nil), sce.StateElement.MerkleProof...)
		s.sces[types.SiacoinOutputID(sce.ID)] = sce
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sce.EncodeTo(e)
		e.Flush()
		_, err := s.tx.Exec(`
			INSERT INTO wt_sces (scoid, network, bytes)
			VALUES (?, ?, ?) AS new
			ON DUPLICATE KEY UPDATE
				bytes = new.bytes
		`, sce.ID[:], s.network, buf.Bytes())
		if err != nil {
			s.log.Error("couldn't add SC output", zap.String("network", s.network), zap.Error(err))
			return err
		}
	}
	return nil
}

func (s *DBStore) removeSiacoinElements(sces []types.SiacoinElement) error {
	for _, sce := range sces {
		delete(s.sces, types.SiacoinOutputID(sce.ID))
		_, err := s.tx.Exec(`
			DELETE FROM wt_sces
			WHERE scoid = ?
			AND network = ?
		`, sce.ID[:], s.network)
		if err != nil {
			s.log.Error("couldn't delete SC output", zap.String("network", s.network), zap.Error(err))
			return err
		}
	}
	return nil
}

// UpdateWalletSiacoinElementProofs implements wallet.UpdateTx.
func (s *DBStore) UpdateWalletSiacoinElementProofs(updater wallet.ProofUpdater) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sces := s.getSiacoinElements()
	for i := range sces {
		updater.UpdateElementProof(&sces[i].StateElement)
	}

	return s.updateSiacoinElements(sces)
}

func (s *DBStore) logEvent(event wallet.Event) {
	inflow, outflow := event.SiacoinInflow(), event.SiacoinOutflow()
	desc := s.network + ": found new "
	switch event.Type {
	case wallet.EventTypeV1Transaction:
		desc += "v1 transaction"
	case wallet.EventTypeV2Transaction:
		desc += "v2 transaction"
	default:
		desc += "unknown event"
	}
	desc += ", id: " + event.ID.String()
	if !inflow.IsZero() {
		desc += ", inflow: " + inflow.String()
	}
	if !outflow.IsZero() {
		desc += ", outflow: " + outflow.String()
	}
	s.log.Info(desc)
}

// WalletApplyIndex implements wallet.UpdateTx.
func (s *DBStore) WalletApplyIndex(_ types.ChainIndex, created, spent []types.SiacoinElement, events []wallet.Event, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.removeSiacoinElements(spent); err != nil {
		return fmt.Errorf("failed to delete Siacoin elements: %w", err)
	}

	if err := s.updateSiacoinElements(created); err != nil {
		return fmt.Errorf("failed to create Siacoin elements: %w", err)
	}

	for _, event := range events {
		s.logEvent(event)
	}

	return nil
}

// WalletRevertIndex implements wallet.UpdateTx.
func (s *DBStore) WalletRevertIndex(_ types.ChainIndex, removed, unspent []types.SiacoinElement, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.removeSiacoinElements(removed); err != nil {
		return fmt.Errorf("failed to delete Siacoin elements: %w", err)
	}

	if err := s.updateSiacoinElements(unspent); err != nil {
		return fmt.Errorf("failed to create Siacoin elements: %w", err)
	}

	return nil
}

// updateChainState applies the chain manager updates.
func (s *DBStore) updateChainState(index types.ChainIndex, mayCommit bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tip = index
	if mayCommit || time.Since(s.lastCommitted) >= 3*time.Second {
		return s.save()
	}

	return nil
}

func (s *DBStore) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tx != nil {
		s.tx.Commit()
	}
}

// NewDBStore returns a new DBStore.
func NewDBStore(db *sql.DB, seedPhrase, network string, logger *zap.Logger) (*DBStore, types.ChainIndex, error) {
	var seed [32]byte
	if err := wallet.SeedFromPhrase(&seed, seedPhrase); err != nil {
		return nil, types.ChainIndex{}, err
	}

	sk := wallet.KeyFromSeed(&seed, 0)
	s := &DBStore{
		addr:    types.StandardUnlockHash(sk.PublicKey()),
		key:     sk,
		sces:    make(map[types.SiacoinOutputID]types.SiacoinElement),
		db:      db,
		network: network,
		log:     logger,
	}

	if err := s.load(); err != nil {
		s.log.Error("couldn't load wallet", zap.String("network", s.network), zap.Error(err))
		return nil, types.ChainIndex{}, err
	}

	return s, s.tip, nil
}

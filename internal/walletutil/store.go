package walletutil

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.uber.org/zap"
)

// A DBStore stores wallet state in a MySQL database.
type DBStore struct {
	tip           types.ChainIndex
	addr          types.Address
	key           types.PrivateKey
	sces          map[types.SiacoinOutputID]types.SiacoinElement
	sfes          map[types.SiafundOutputID]types.SiafundElement
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

	row := 1
	if s.network == "zen" {
		row = 2
	}
	_, err := s.tx.Exec(`
		REPLACE INTO wt_tip (id, network, height, bid)
		VALUES (?, ?, ?, ?)
	`, row, s.network, s.tip.Height, s.tip.ID[:])
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
	row := 1
	if s.network == "zen" {
		row = 2
	}
	var height uint64
	id := make([]byte, 32)
	err := s.db.QueryRow(`
		SELECT height, bid
		FROM wt_tip
		WHERE id = ?
	`, row).Scan(&height, &id)
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

	rows, err = s.db.Query(`
		SELECT sfoid, bytes
		FROM wt_sfes
		WHERE network = ?
	`, s.network)
	if err != nil {
		return utils.AddContext(err, "couldn't query SF elements")
	}

	for rows.Next() {
		if err := rows.Scan(&id, &b); err != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't scan SF element")
		}
		var sfoid types.SiafundOutputID
		copy(sfoid[:], id)
		d := types.NewBufDecoder(b)
		var sfe types.SiafundElement
		sfe.DecodeFrom(d)
		if d.Err() != nil {
			rows.Close()
			return utils.AddContext(err, "couldn't decode SF element")
		}
		s.sfes[sfoid] = sfe
	}
	rows.Close()

	s.tx, err = s.db.Begin()
	return err
}

func (s *DBStore) addSiacoinElements(sces []types.SiacoinElement) error {
	for _, sce := range sces {
		sce.MerkleProof = append([]types.Hash256(nil), sce.MerkleProof...)
		s.sces[types.SiacoinOutputID(sce.ID)] = sce
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sce.EncodeTo(e)
		e.Flush()
		_, err := s.tx.Exec(`
			INSERT INTO wt_sces (scoid, network, bytes)
			VALUES (?, ?, ?)
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

func (s *DBStore) addSiafundElements(sfes []types.SiafundElement) error {
	for _, sfe := range sfes {
		sfe.MerkleProof = append([]types.Hash256(nil), sfe.MerkleProof...)
		s.sfes[types.SiafundOutputID(sfe.ID)] = sfe
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sfe.EncodeTo(e)
		e.Flush()
		_, err := s.tx.Exec(`
			INSERT INTO wt_sfes (sfoid, network, bytes)
			VALUES (?, ?, ?)
		`, sfe.ID[:], s.network, buf.Bytes())
		if err != nil {
			s.log.Error("couldn't add SF output", zap.String("network", s.network), zap.Error(err))
			return err
		}
	}
	return nil
}

func (s *DBStore) removeSiafundElements(sfes []types.SiafundElement) error {
	for _, sfe := range sfes {
		delete(s.sfes, types.SiafundOutputID(sfe.ID))
		_, err := s.tx.Exec(`
			DELETE FROM wt_sfes
			WHERE sfoid = ?
			AND network = ?
		`, sfe.ID[:], s.network)
		if err != nil {
			s.log.Error("couldn't delete SF output", zap.String("network", s.network), zap.Error(err))
			return err
		}
	}
	return nil
}

// applyChainUpdate atomically applies a chain update to the store.
func (s *DBStore) applyChainUpdate(cau chain.ApplyUpdate) error {
	// Check if the update is for the right network.
	if s.network != cau.State.Network.Name {
		return nil
	}

	// Determine which Siacoin and Siafund elements are ephemeral.
	created := make(map[types.Hash256]bool)
	ephemeral := make(map[types.Hash256]bool)
	for _, txn := range cau.Block.Transactions {
		for i := range txn.SiacoinOutputs {
			created[types.Hash256(txn.SiacoinOutputID(i))] = true
		}
		for _, input := range txn.SiacoinInputs {
			ephemeral[types.Hash256(input.ParentID)] = created[types.Hash256(input.ParentID)]
		}
		for i := range txn.SiafundOutputs {
			created[types.Hash256(txn.SiafundOutputID(i))] = true
		}
		for _, input := range txn.SiafundInputs {
			ephemeral[types.Hash256(input.ParentID)] = created[types.Hash256(input.ParentID)]
		}
	}

	// Add new Siacoin elements to the store.
	var newSiacoinElements, spentSiacoinElements []types.SiacoinElement
	cau.ForEachSiacoinElement(func(se types.SiacoinElement, spent bool) {
		if ephemeral[se.ID] {
			return
		}

		if se.SiacoinOutput.Address != s.addr {
			return
		}

		if spent {
			spentSiacoinElements = append(spentSiacoinElements, se)
		} else {
			newSiacoinElements = append(newSiacoinElements, se)
		}
	})

	if err := s.addSiacoinElements(newSiacoinElements); err != nil {
		return fmt.Errorf("failed to add Siacoin elements: %w", err)
	} else if err := s.removeSiacoinElements(spentSiacoinElements); err != nil {
		return fmt.Errorf("failed to remove Siacoin elements: %w", err)
	}

	var newSiafundElements, spentSiafundElements []types.SiafundElement
	cau.ForEachSiafundElement(func(se types.SiafundElement, spent bool) {
		if ephemeral[se.ID] {
			return
		}

		if se.SiafundOutput.Address != s.addr {
			return
		}

		if spent {
			spentSiafundElements = append(spentSiafundElements, se)
		} else {
			newSiafundElements = append(newSiafundElements, se)
		}
	})

	if err := s.addSiafundElements(newSiafundElements); err != nil {
		return fmt.Errorf("failed to add Siafund elements: %w", err)
	} else if err := s.removeSiafundElements(spentSiafundElements); err != nil {
		return fmt.Errorf("failed to remove Siafund elements: %w", err)
	}

	// Apply events.
	events := wallet.AppliedEvents(cau.State, cau.Block, cau, s.addr)
	for _, event := range events {
		s.log.Info("found new event", zap.String("network", s.network), zap.Stringer("event", event))
	}

	// Update Siacoin element proofs.
	for id, sce := range s.sces {
		cau.UpdateElementProof(&sce.StateElement)
		s.sces[id] = sce
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sce.EncodeTo(e)
		e.Flush()
		_, err := s.tx.Exec(`
			UPDATE wt_sces
			SET bytes = ?
			WHERE scoid = ?
			AND network = ?
		`, buf.Bytes(), sce.ID[:], s.network)
		if err != nil {
			s.log.Error("couldn't update SC element proof", zap.String("network", s.network), zap.Error(err))
			return err
		}
	}

	// Update Siafund element proofs.
	for id, sfe := range s.sfes {
		cau.UpdateElementProof(&sfe.StateElement)
		s.sfes[id] = sfe
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sfe.EncodeTo(e)
		e.Flush()
		_, err := s.tx.Exec(`
			UPDATE wt_sfes
			SET bytes = ?
			WHERE sfoid = ?
			AND network = ?
		`, buf.Bytes(), sfe.ID[:], s.network)
		if err != nil {
			s.log.Error("couldn't update SF element proof", zap.String("network", s.network), zap.Error(err))
			return err
		}
	}

	s.tip = cau.State.Index

	return nil
}

// revertChainUpdate atomically reverts a chain update from the store.
func (s *DBStore) revertChainUpdate(cru chain.RevertUpdate) error {
	// Check if the update is for the right network.
	if s.network != cru.State.Network.Name {
		return nil
	}

	// Determine which Siacoin and Siafund elements are ephemeral.
	created := make(map[types.Hash256]bool)
	ephemeral := make(map[types.Hash256]bool)
	for _, txn := range cru.Block.Transactions {
		for i := range txn.SiacoinOutputs {
			created[types.Hash256(txn.SiacoinOutputID(i))] = true
		}
		for _, input := range txn.SiacoinInputs {
			ephemeral[types.Hash256(input.ParentID)] = created[types.Hash256(input.ParentID)]
		}
		for i := range txn.SiafundOutputs {
			created[types.Hash256(txn.SiafundOutputID(i))] = true
		}
		for _, input := range txn.SiafundInputs {
			ephemeral[types.Hash256(input.ParentID)] = created[types.Hash256(input.ParentID)]
		}
	}

	var removedSiacoinElements, addedSiacoinElements []types.SiacoinElement
	cru.ForEachSiacoinElement(func(se types.SiacoinElement, spent bool) {
		if ephemeral[se.ID] {
			return
		}

		if se.SiacoinOutput.Address != s.addr {
			return
		}

		if spent {
			// Re-add any spent Siacoin elements.
			addedSiacoinElements = append(addedSiacoinElements, se)
		} else {
			// Delete any created Siacoin elements.
			removedSiacoinElements = append(removedSiacoinElements, se)
		}
	})

	// Revert Siacoin element changes.
	if err := s.addSiacoinElements(addedSiacoinElements); err != nil {
		return fmt.Errorf("failed to add Siacoin elements: %w", err)
	} else if err := s.removeSiacoinElements(removedSiacoinElements); err != nil {
		return fmt.Errorf("failed to remove Siacoin elements: %w", err)
	}

	var removedSiafundElements, addedSiafundElements []types.SiafundElement
	cru.ForEachSiafundElement(func(se types.SiafundElement, spent bool) {
		if ephemeral[se.ID] {
			return
		}

		if se.SiafundOutput.Address != s.addr {
			return
		}

		if spent {
			// Re-add any spent Siafund elements.
			addedSiafundElements = append(addedSiafundElements, se)
		} else {
			// Delete any created Siafund elements.
			removedSiafundElements = append(removedSiafundElements, se)
		}
	})

	// Revert Siafund element changes.
	if err := s.addSiafundElements(addedSiafundElements); err != nil {
		return fmt.Errorf("failed to add Siafund elements: %w", err)
	} else if err := s.removeSiafundElements(removedSiafundElements); err != nil {
		return fmt.Errorf("failed to remove Siafund elements: %w", err)
	}

	// Update Siacoin element proofs.
	for id, sce := range s.sces {
		cru.UpdateElementProof(&sce.StateElement)
		s.sces[id] = sce
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sce.EncodeTo(e)
		e.Flush()
		_, err := s.tx.Exec(`
			UPDATE wt_sces
			SET bytes = ?
			WHERE scoid = ?
			AND network = ?
		`, buf.Bytes(), sce.ID[:], s.network)
		if err != nil {
			s.log.Error("couldn't update SC element proof", zap.String("network", s.network), zap.Error(err))
			return err
		}
	}

	// Update Siafund element proofs.
	for id, sfe := range s.sfes {
		cru.UpdateElementProof(&sfe.StateElement)
		s.sfes[id] = sfe
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sfe.EncodeTo(e)
		e.Flush()
		_, err := s.tx.Exec(`
			UPDATE wt_sfes
			SET bytes = ?
			WHERE sfoid = ?
			AND network = ?
		`, buf.Bytes(), sfe.ID[:], s.network)
		if err != nil {
			s.log.Error("couldn't update SF element proof", zap.String("network", s.network), zap.Error(err))
			return err
		}
	}

	// Revert events.
	for _, event := range wallet.AppliedEvents(cru.State, cru.Block, cru, s.addr) {
		s.log.Info("reverting event", zap.String("network", s.network), zap.Stringer("event", event))
	}

	return nil
}

// updateChainState applies the chain manager updates.
func (s *DBStore) updateChainState(reverted []chain.RevertUpdate, applied []chain.ApplyUpdate, mayCommit bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cru := range reverted {
		revertedIndex := types.ChainIndex{
			ID:     cru.Block.ID(),
			Height: cru.State.Index.Height + 1,
		}
		if err := s.revertChainUpdate(cru); err != nil {
			return fmt.Errorf("failed to revert chain update %q: %w", revertedIndex, err)
		}
	}

	for _, cau := range applied {
		if err := s.applyChainUpdate(cau); err != nil {
			return fmt.Errorf("failed to apply chain update %q: %w", cau.State.Index, err)
		}
	}

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

// Annotate implements api.Wallet.
func (s *DBStore) Annotate(txns []types.Transaction) (ptxns []wallet.PoolTransaction) {
	for _, txn := range txns {
		ptxn := wallet.Annotate(txn, s.addr)
		if ptxn.Type != "unrelated" {
			ptxns = append(ptxns, ptxn)
		}
	}
	return
}

// UnspentOutputs implements api.Wallet.
func (s *DBStore) UnspentOutputs() (sces []types.SiacoinElement, sfes []types.SiafundElement, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sco := range s.sces {
		sces = append(sces, sco)
	}
	for _, sfo := range s.sfes {
		sfes = append(sfes, sfo)
	}
	return
}

// Address implements api.Wallet.
func (s *DBStore) Address() types.Address {
	return s.addr
}

// NewDBStore returns a new DBStore.
func NewDBStore(db *sql.DB, seed, network string, logger *zap.Logger) (*DBStore, types.ChainIndex, error) {
	sk, err := wallet.KeyFromPhrase(seed)
	if err != nil {
		return nil, types.ChainIndex{}, err
	}
	s := &DBStore{
		addr:    types.StandardUnlockHash(sk.PublicKey()),
		key:     sk,
		sces:    make(map[types.SiacoinOutputID]types.SiacoinElement),
		sfes:    make(map[types.SiafundOutputID]types.SiafundElement),
		db:      db,
		network: network,
		log:     logger,
	}

	err = s.load()
	if err != nil {
		s.log.Error("couldn't load wallet", zap.String("network", s.network), zap.Error(err))
	}

	return s, s.tip, err
}

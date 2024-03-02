package walletutil

import (
	"bytes"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/persist"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
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
	log           *persist.Logger
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

// ProcessChainApplyUpdate implements chain.Subscriber.
func (s *DBStore) ProcessChainApplyUpdate(cau *chain.ApplyUpdate, mayCommit bool) (err error) {
	// Check if the update is for the right network.
	if s.network != cau.State.Network.Name {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	events := wallet.AppliedEvents(cau.State, cau.Block, cau, s.addr)
	for _, event := range events {
		s.log.Printf("[INFO] %s: found %s\n", strings.ToUpper(string(s.network[0]))+s.network[1:], event.String())
	}

	// Add/remove outputs.
	cau.ForEachSiacoinElement(func(sce types.SiacoinElement, spent bool) {
		if sce.SiacoinOutput.Address == s.addr {
			if spent {
				delete(s.sces, types.SiacoinOutputID(sce.ID))
				_, err = s.tx.Exec(`
					DELETE FROM wt_sces
					WHERE scoid = ?
					AND network = ?
				`, sce.ID[:], s.network)
				if err != nil {
					s.log.Println("[ERROR] couldn't delete SC output:", err)
				}
			} else {
				sce.MerkleProof = append([]types.Hash256(nil), sce.MerkleProof...)
				s.sces[types.SiacoinOutputID(sce.ID)] = sce
				var buf bytes.Buffer
				e := types.NewEncoder(&buf)
				sce.EncodeTo(e)
				e.Flush()
				_, err = s.tx.Exec(`
					INSERT INTO wt_sces (scoid, network, bytes)
					VALUES (?, ?, ?)
				`, sce.ID[:], s.network, buf.Bytes())
				if err != nil {
					s.log.Println("[ERROR] couldn't add SC output:", err)
				}
			}
		}
	})
	cau.ForEachSiafundElement(func(sfe types.SiafundElement, spent bool) {
		if sfe.SiafundOutput.Address == s.addr {
			if spent {
				delete(s.sfes, types.SiafundOutputID(sfe.ID))
				_, err = s.tx.Exec(`
					DELETE FROM wt_sfes
					WHERE sfoid = ?
					AND network = ?
				`, sfe.ID[:], s.network)
				if err != nil {
					s.log.Println("[ERROR] couldn't delete SF output:", err)
				}
			} else {
				sfe.MerkleProof = append([]types.Hash256(nil), sfe.MerkleProof...)
				s.sfes[types.SiafundOutputID(sfe.ID)] = sfe
				var buf bytes.Buffer
				e := types.NewEncoder(&buf)
				sfe.EncodeTo(e)
				e.Flush()
				_, err = s.tx.Exec(`
					INSERT INTO wt_sfes (sfoid, network, bytes)
					VALUES (?, ?, ?)
				`, sfe.ID[:], s.network, buf.Bytes())
				if err != nil {
					s.log.Println("[ERROR] couldn't add SF output:", err)
				}
			}
		}
	})

	// Update proofs.
	for id, sce := range s.sces {
		cau.UpdateElementProof(&sce.StateElement)
		s.sces[id] = sce
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sce.EncodeTo(e)
		e.Flush()
		_, err = s.tx.Exec(`
			UPDATE wt_sces
			SET bytes = ?
			WHERE scoid = ?
			AND network = ?
		`, buf.Bytes(), sce.ID[:], s.network)
		if err != nil {
			s.log.Println("[ERROR] couldn't update SC element proof:", err)
		}
	}
	for id, sfe := range s.sfes {
		cau.UpdateElementProof(&sfe.StateElement)
		s.sfes[id] = sfe
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sfe.EncodeTo(e)
		e.Flush()
		_, err = s.tx.Exec(`
			UPDATE wt_sfes
			SET bytes = ?
			WHERE sfoid = ?
			AND network = ?
		`, buf.Bytes(), sfe.ID[:], s.network)
		if err != nil {
			s.log.Println("[ERROR] couldn't update SF element proof:", err)
		}
	}

	s.tip = cau.State.Index
	if mayCommit || time.Since(s.lastCommitted) >= 3*time.Second {
		return s.save()
	}

	return nil
}

// ProcessChainRevertUpdate implements chain.Subscriber.
func (s *DBStore) ProcessChainRevertUpdate(cru *chain.RevertUpdate) (err error) {
	// Check if the update is for the right network.
	if s.network != cru.State.Network.Name {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range wallet.AppliedEvents(cru.State, cru.Block, cru, s.addr) {
		s.log.Printf("[INFO] %s: reverting %s\n", strings.ToUpper(string(s.network[0]))+s.network[1:], event.String())
	}

	cru.ForEachSiacoinElement(func(sce types.SiacoinElement, spent bool) {
		if sce.SiacoinOutput.Address == s.addr {
			if !spent {
				delete(s.sces, types.SiacoinOutputID(sce.ID))
				_, err = s.tx.Exec(`
					DELETE FROM wt_sces
					WHERE scoid = ?
					AND network = ?
				`, sce.ID[:], s.network)
				if err != nil {
					s.log.Println("[ERROR] couldn't delete SC output:", err)
				}
			} else {
				sce.MerkleProof = append([]types.Hash256(nil), sce.MerkleProof...)
				s.sces[types.SiacoinOutputID(sce.ID)] = sce
				var buf bytes.Buffer
				e := types.NewEncoder(&buf)
				sce.EncodeTo(e)
				e.Flush()
				_, err = s.tx.Exec(`
					INSERT INTO wt_sces (scoid, network, bytes)
					VALUES (?, ?, ?)
				`, sce.ID[:], s.network, buf.Bytes())
				if err != nil {
					s.log.Println("[ERROR] couldn't add SC output:", err)
				}
			}
		}
	})
	cru.ForEachSiafundElement(func(sfe types.SiafundElement, spent bool) {
		if sfe.SiafundOutput.Address == s.addr {
			if !spent {
				delete(s.sfes, types.SiafundOutputID(sfe.ID))
				_, err = s.tx.Exec(`
					DELETE FROM wt_sfes
					WHERE sfoid = ?
					AND network = ?
				`, sfe.ID[:], s.network)
				if err != nil {
					s.log.Println("[ERROR] couldn't delete SF output:", err)
				}
			} else {
				sfe.MerkleProof = append([]types.Hash256(nil), sfe.MerkleProof...)
				s.sfes[types.SiafundOutputID(sfe.ID)] = sfe
				var buf bytes.Buffer
				e := types.NewEncoder(&buf)
				sfe.EncodeTo(e)
				e.Flush()
				_, err = s.tx.Exec(`
					INSERT INTO wt_sfes (sfoid, network, bytes)
					VALUES (?, ?, ?)
				`, sfe.ID[:], s.network, buf.Bytes())
				if err != nil {
					s.log.Println("[ERROR] couldn't add SF output:", err)
				}
			}
		}
	})

	// Update proofs.
	for id, sce := range s.sces {
		cru.UpdateElementProof(&sce.StateElement)
		s.sces[id] = sce
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sce.EncodeTo(e)
		e.Flush()
		_, err = s.tx.Exec(`
			UPDATE wt_sces
			SET bytes = ?
			WHERE scoid = ?
			AND network = ?
		`, buf.Bytes(), sce.ID[:], s.network)
		if err != nil {
			s.log.Println("[ERROR] couldn't update SC element proof:", err)
		}
	}
	for id, sfe := range s.sfes {
		cru.UpdateElementProof(&sfe.StateElement)
		s.sfes[id] = sfe
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sfe.EncodeTo(e)
		e.Flush()
		_, err = s.tx.Exec(`
			UPDATE wt_sfes
			SET bytes = ?
			WHERE sfoid = ?
			AND network = ?
		`, buf.Bytes(), sfe.ID[:], s.network)
		if err != nil {
			s.log.Println("[ERROR] couldn't update SF element proof:", err)
		}
	}

	s.tip = cru.State.Index

	err = s.save()
	if err != nil {
		s.log.Println("[ERROR] couldn't save wallet:", err)
	}

	return nil
}

func (s *DBStore) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tx != nil {
		s.tx.Commit()
	}
	s.log.Close()
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
func NewDBStore(db *sql.DB, seed, network string, logger *persist.Logger) (*DBStore, types.ChainIndex, error) {
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
		s.log.Println("[ERROR] couldn't load wallet:", err)
	}

	return s, s.tip, err
}

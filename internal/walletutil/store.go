package walletutil

import (
	"bytes"
	"database/sql"
	"errors"
	"sync"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/persist"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/chain"
	"go.sia.tech/core/types"
)

// A DBStore stores wallet state in a MySQL database.
type DBStore struct {
	tip     types.ChainIndex
	addr    types.Address
	key     types.PrivateKey
	sces    map[types.SiacoinOutputID]types.SiacoinElement
	sfes    map[types.SiafundOutputID]types.SiafundElement
	mu      sync.Mutex
	db      *sql.DB
	tx      *sql.Tx
	log     *persist.Logger
	network string
}

func (s *DBStore) save() error {
	if s.tx == nil {
		return errors.New("there is no transaction")
	}

	_, err := s.tx.Exec(`
		REPLACE INTO wt_tip_`+s.network+` (id, height, bid)
		VALUES (1, ?, ?)
	`, s.tip.Height, s.tip.ID[:])
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
	return err
}

func (s *DBStore) load() error {
	var height uint64
	id := make([]byte, 32)
	err := s.db.QueryRow(`
		SELECT height, bid
		FROM wt_tip_`+s.network+`
		WHERE id = 1
	`).Scan(&height, &id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return utils.AddContext(err, "couldn't load tip")
	}
	s.tip.Height = height
	copy(s.tip.ID[:], id)

	rows, err := s.db.Query("SELECT scoid, bytes FROM wt_sces_" + s.network)
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

	rows, err = s.db.Query("SELECT sfoid, bytes FROM wt_sfes_" + s.network)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	events := wallet.AppliedEvents(cau.State, cau.Block, cau, s.addr)
	for _, event := range events {
		s.log.Printf("[INFO] found %s\n", event.String())
	}

	// add/remove outputs
	cau.ForEachSiacoinElement(func(sce types.SiacoinElement, spent bool) {
		if sce.SiacoinOutput.Address == s.addr {
			if spent {
				delete(s.sces, types.SiacoinOutputID(sce.ID))
				_, err = s.tx.Exec("DELETE FROM wt_sces_"+s.network+" WHERE scoid = ?", sce.ID[:])
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
					INSERT INTO wt_sces_`+s.network+` (scoid, bytes)
					VALUES (?, ?)
				`, sce.ID[:], buf.Bytes())
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
				_, err = s.tx.Exec("DELETE FROM wt_sfes_"+s.network+" WHERE sfoid = ?", sfe.ID[:])
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
					INSERT INTO wt_sfes_`+s.network+` (sfoid, bytes)
					VALUES (?, ?)
				`, sfe.ID[:], buf.Bytes())
				if err != nil {
					s.log.Println("[ERROR] couldn't add SF output:", err)
				}
			}
		}
	})

	// update proofs
	for id, sce := range s.sces {
		cau.UpdateElementProof(&sce.StateElement)
		s.sces[id] = sce
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sce.EncodeTo(e)
		e.Flush()
		_, err = s.tx.Exec(`
			UPDATE wt_sces_`+s.network+`
			SET bytes = ?
			WHERE scoid = ?
		`, buf.Bytes(), sce.ID[:])
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
			UPDATE wt_sfes_`+s.network+`
			SET bytes = ?
			WHERE sfoid = ?
		`, buf.Bytes(), sfe.ID[:])
		if err != nil {
			s.log.Println("[ERROR] couldn't update SF element proof:", err)
		}
	}

	s.tip = cau.State.Index
	if mayCommit {
		return s.save()
	}

	return nil
}

// ProcessChainRevertUpdate implements chain.Subscriber.
func (s *DBStore) ProcessChainRevertUpdate(cru *chain.RevertUpdate) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range wallet.AppliedEvents(cru.State, cru.Block, cru, s.addr) {
		s.log.Printf("[INFO] reverting %s\n", event.String())
	}

	cru.ForEachSiacoinElement(func(sce types.SiacoinElement, spent bool) {
		if sce.SiacoinOutput.Address == s.addr {
			if !spent {
				delete(s.sces, types.SiacoinOutputID(sce.ID))
				_, err = s.tx.Exec("DELETE FROM wt_sces_"+s.network+" WHERE scoid = ?", sce.ID[:])
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
					INSERT INTO wt_sces_`+s.network+` (scoid, bytes)
					VALUES (?, ?)
				`, sce.ID[:], buf.Bytes())
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
				_, err = s.tx.Exec("DELETE FROM wt_sfes_"+s.network+" WHERE sfoid = ?", sfe.ID[:])
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
					INSERT INTO wt_sfes_`+s.network+` (sfoid, bytes)
					VALUES (?, ?)
				`, sfe.ID[:], buf.Bytes())
				if err != nil {
					s.log.Println("[ERROR] couldn't add SF output:", err)
				}
			}
		}
	})

	// update proofs
	for id, sce := range s.sces {
		cru.UpdateElementProof(&sce.StateElement)
		s.sces[id] = sce
		var buf bytes.Buffer
		e := types.NewEncoder(&buf)
		sce.EncodeTo(e)
		e.Flush()
		_, err = s.tx.Exec(`
			UPDATE wt_sces_`+s.network+`
			SET bytes = ?
			WHERE scoid = ?
		`, buf.Bytes(), sce.ID[:])
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
			UPDATE wt_sfes_`+s.network+`
			SET bytes = ?
			WHERE sfoid = ?
		`, buf.Bytes(), sfe.ID[:])
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

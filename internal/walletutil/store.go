package walletutil

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/chain"
	"go.sia.tech/core/types"
)

// An EphemeralStore stores wallet state in memory.
type EphemeralStore struct {
	tip    types.ChainIndex
	addr   types.Address
	key    types.PrivateKey
	sces   map[types.SiacoinOutputID]types.SiacoinElement
	sfes   map[types.SiafundOutputID]types.SiafundElement
	events []wallet.Event
	mu     sync.Mutex
}

// Events implements api.Wallet.
func (s *EphemeralStore) Events(offset, limit int) (events []wallet.Event, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit == -1 {
		limit = len(s.events)
	}
	if offset > len(s.events) {
		offset = len(s.events)
	}
	if offset+limit > len(s.events) {
		limit = len(s.events) - offset
	}
	// reverse
	es := make([]wallet.Event, limit)
	for i := range es {
		es[i] = s.events[len(s.events)-offset-i-1]
	}
	return es, nil
}

// Annotate implements api.Wallet.
func (s *EphemeralStore) Annotate(txns []types.Transaction) (ptxns []wallet.PoolTransaction) {
	for _, txn := range txns {
		ptxn := wallet.Annotate(txn, s.addr)
		if ptxn.Type != "unrelated" {
			ptxns = append(ptxns, ptxn)
		}
	}
	return
}

// UnspentOutputs implements api.Wallet.
func (s *EphemeralStore) UnspentOutputs() (sces []types.SiacoinElement, sfes []types.SiafundElement, err error) {
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
func (s *EphemeralStore) Address() types.Address {
	return s.addr
}

// ProcessChainApplyUpdate implements chain.Subscriber.
func (s *EphemeralStore) ProcessChainApplyUpdate(cau *chain.ApplyUpdate, _ bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := wallet.AppliedEvents(cau.State, cau.Block, cau, s.addr)
	s.events = append(s.events, events...)

	// add/remove outputs
	cau.ForEachSiacoinElement(func(sce types.SiacoinElement, spent bool) {
		if sce.SiacoinOutput.Address == s.addr {
			if spent {
				delete(s.sces, types.SiacoinOutputID(sce.ID))
			} else {
				sce.MerkleProof = append([]types.Hash256(nil), sce.MerkleProof...)
				s.sces[types.SiacoinOutputID(sce.ID)] = sce
			}
		}
	})
	cau.ForEachSiafundElement(func(sfe types.SiafundElement, spent bool) {
		if sfe.SiafundOutput.Address == s.addr {
			if spent {
				delete(s.sfes, types.SiafundOutputID(sfe.ID))
			} else {
				sfe.MerkleProof = append([]types.Hash256(nil), sfe.MerkleProof...)
				s.sfes[types.SiafundOutputID(sfe.ID)] = sfe
			}
		}
	})

	// update proofs
	for id, sce := range s.sces {
		cau.UpdateElementProof(&sce.StateElement)
		s.sces[id] = sce
	}
	for id, sfe := range s.sfes {
		cau.UpdateElementProof(&sfe.StateElement)
		s.sfes[id] = sfe
	}

	s.tip = cau.State.Index
	return nil
}

// ProcessChainRevertUpdate implements chain.Subscriber.
func (s *EphemeralStore) ProcessChainRevertUpdate(cru *chain.RevertUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// terribly inefficient, but not a big deal because reverts are infrequent
	numEvents := len(wallet.AppliedEvents(cru.State, cru.Block, cru, s.addr))
	s.events = s.events[:len(s.events)-numEvents]

	cru.ForEachSiacoinElement(func(sce types.SiacoinElement, spent bool) {
		if sce.SiacoinOutput.Address == s.addr {
			if !spent {
				delete(s.sces, types.SiacoinOutputID(sce.ID))
			} else {
				sce.MerkleProof = append([]types.Hash256(nil), sce.MerkleProof...)
				s.sces[types.SiacoinOutputID(sce.ID)] = sce
			}
		}
	})
	cru.ForEachSiafundElement(func(sfe types.SiafundElement, spent bool) {
		if sfe.SiafundOutput.Address == s.addr {
			if !spent {
				delete(s.sfes, types.SiafundOutputID(sfe.ID))
			} else {
				sfe.MerkleProof = append([]types.Hash256(nil), sfe.MerkleProof...)
				s.sfes[types.SiafundOutputID(sfe.ID)] = sfe
			}
		}
	})

	// update proofs
	for id, sce := range s.sces {
		cru.UpdateElementProof(&sce.StateElement)
		s.sces[id] = sce
	}
	for id, sfe := range s.sfes {
		cru.UpdateElementProof(&sfe.StateElement)
		s.sfes[id] = sfe
	}

	s.tip = cru.State.Index
	return nil
}

// NewEphemeralStore returns a new EphemeralStore.
func NewEphemeralStore(seed string) *EphemeralStore {
	sk, err := wallet.KeyFromPhrase(seed)
	if err != nil {
		return nil
	}
	return &EphemeralStore{
		addr: types.StandardUnlockHash(sk.PublicKey()),
		key:  sk,
		sces: make(map[types.SiacoinOutputID]types.SiacoinElement),
		sfes: make(map[types.SiafundOutputID]types.SiafundElement),
	}
}

// A JSONStore stores wallet state in memory, backed by a JSON file.
type JSONStore struct {
	*EphemeralStore
	path string
}

type persistData struct {
	Tip             types.ChainIndex
	SiacoinElements map[types.SiacoinOutputID]types.SiacoinElement
	SiafundElements map[types.SiafundOutputID]types.SiafundElement
	Events          []wallet.Event
}

func (s *JSONStore) save() error {
	js, err := json.MarshalIndent(persistData{
		Tip:             s.tip,
		SiacoinElements: s.sces,
		SiafundElements: s.sfes,
		Events:          s.events,
	}, "", "  ")
	if err != nil {
		return err
	}

	f, err := os.OpenFile(s.path+"_tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(js); err != nil {
		return err
	} else if f.Sync(); err != nil {
		return err
	} else if f.Close(); err != nil {
		return err
	} else if err := os.Rename(s.path+"_tmp", s.path); err != nil {
		return err
	}
	return nil
}

func (s *JSONStore) load() error {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	defer f.Close()
	var p persistData
	if err := json.NewDecoder(f).Decode(&p); err != nil {
		return err
	}
	s.tip = p.Tip
	s.sces = p.SiacoinElements
	s.sfes = p.SiafundElements
	s.events = p.Events
	return nil
}

// ProcessChainApplyUpdate implements chain.Subscriber.
func (s *JSONStore) ProcessChainApplyUpdate(cau *chain.ApplyUpdate, mayCommit bool) error {
	err := s.EphemeralStore.ProcessChainApplyUpdate(cau, mayCommit)
	if err == nil && mayCommit {
		err = s.save()
	}
	return err
}

// ProcessChainRevertUpdate implements chain.Subscriber.
func (s *JSONStore) ProcessChainRevertUpdate(cru *chain.RevertUpdate) error {
	return s.EphemeralStore.ProcessChainRevertUpdate(cru)
}

// NewJSONStore returns a new JSONStore.
func NewJSONStore(seed, path string) (*JSONStore, types.ChainIndex, error) {
	s := &JSONStore{
		EphemeralStore: NewEphemeralStore(seed),
		path:           path,
	}
	err := s.load()
	return s, s.tip, err
}

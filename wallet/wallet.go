package wallet

import (
	"fmt"
	"time"

	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
)

// StandardTransactionSignature is the most common form of TransactionSignature.
// It covers the entire transaction, references a sole public key, and has no
// timelock.
func StandardTransactionSignature(id types.Hash256) types.TransactionSignature {
	return types.TransactionSignature{
		ParentID:       id,
		CoveredFields:  types.CoveredFields{WholeTransaction: true},
		PublicKeyIndex: 0,
	}
}

// SignTransaction signs txn with the given key. The TransactionSignature object
// must already be present in txn at the given index.
func SignTransaction(cs consensus.State, txn *types.Transaction, sigIndex int, key types.PrivateKey) {
	tsig := &txn.Signatures[sigIndex]
	var sigHash types.Hash256
	if tsig.CoveredFields.WholeTransaction {
		sigHash = cs.WholeSigHash(*txn, tsig.ParentID, tsig.PublicKeyIndex, tsig.Timelock, tsig.CoveredFields.Signatures)
	} else {
		sigHash = cs.PartialSigHash(*txn, tsig.CoveredFields)
	}
	sig := key.SignHash(sigHash)
	tsig.Signature = sig[:]
}

// A PoolTransaction summarizes the wallet-relevant data in a txpool
// transaction.
type PoolTransaction struct {
	ID       types.TransactionID `json:"id"`
	Raw      types.Transaction   `json:"raw"`
	Type     string              `json:"type"`
	Sent     types.Currency      `json:"sent"`
	Received types.Currency      `json:"received"`
	Locked   types.Currency      `json:"locked"`
}

// Annotate annotates a txpool transaction.
func Annotate(txn types.Transaction, addr types.Address) PoolTransaction {
	ptxn := PoolTransaction{ID: txn.ID(), Raw: txn, Type: "unknown"}

	var totalValue types.Currency
	for _, sco := range txn.SiacoinOutputs {
		totalValue = totalValue.Add(sco.Value)
	}
	for _, fc := range txn.FileContracts {
		totalValue = totalValue.Add(fc.Payout)
	}
	for _, fee := range txn.MinerFees {
		totalValue = totalValue.Add(fee)
	}

	var ownedIn, ownedOut int
	for _, sci := range txn.SiacoinInputs {
		if sci.UnlockConditions.UnlockHash() == addr {
			ownedIn++
		}
	}
	for _, sco := range txn.SiacoinOutputs {
		if sco.Address == addr {
			ownedOut++
		}
	}
	var ins, outs string
	switch {
	case ownedIn == 0:
		ins = "none"
	case ownedIn < len(txn.SiacoinInputs):
		ins = "some"
	case ownedIn == len(txn.SiacoinInputs):
		ins = "all"
	}
	switch {
	case ownedOut == 0:
		outs = "none"
	case ownedOut < len(txn.SiacoinOutputs):
		outs = "some"
	case ownedOut == len(txn.SiacoinOutputs):
		outs = "all"
	}

	switch {
	case ins == "none" && outs == "none":
		ptxn.Type = "unrelated"
	case ins == "all":
		ptxn.Sent = totalValue
		switch {
		case outs == "all":
			ptxn.Type = "redistribution"
		case len(txn.FileContractRevisions) > 0:
			ptxn.Type = "contract revision"
		case len(txn.StorageProofs) > 0:
			ptxn.Type = "storage proof"
		case len(txn.ArbitraryData) > 0:
			ptxn.Type = "announcement"
		default:
			ptxn.Type = "send"
		}
	case ins == "none" && outs != "none":
		ptxn.Type = "receive"
		for _, sco := range txn.SiacoinOutputs {
			if sco.Address == addr {
				ptxn.Received = ptxn.Received.Add(sco.Value)
			}
		}
	case ins == "some" && len(txn.FileContracts) > 0:
		ptxn.Type = "contract"
		for _, fc := range txn.FileContracts {
			var validLocked, missedLocked types.Currency
			for _, sco := range fc.ValidProofOutputs {
				if sco.Address == addr {
					validLocked = validLocked.Add(fc.Payout)
				}
			}
			for _, sco := range fc.MissedProofOutputs {
				if sco.Address == addr {
					missedLocked = missedLocked.Add(fc.Payout)
				}
			}
			if validLocked.Cmp(missedLocked) > 0 {
				ptxn.Locked = ptxn.Locked.Add(validLocked)
			} else {
				ptxn.Locked = ptxn.Locked.Add(missedLocked)
			}
		}
	}

	return ptxn
}

// An Event is something interesting that happened on the Sia blockchain.
type Event struct {
	Index     types.ChainIndex
	Timestamp time.Time
	Val       interface{ eventType() string }
}

func (*EventTransaction) eventType() string        { return "transaction" }
func (*EventMinerPayout) eventType() string        { return "miner payout" }
func (*EventMissedFileContract) eventType() string { return "missed file contract" }

// String implements fmt.Stringer.
func (e *Event) String() string {
	return fmt.Sprintf("%s at %s: %s", e.Val.eventType(), e.Timestamp, e.Val)
}

// A HostAnnouncement represents a host announcement within an EventTransaction.
type HostAnnouncement struct {
	PublicKey  types.PublicKey `json:"publicKey"`
	NetAddress string          `json:"netAddress"`
}

// A SiafundInput represents a siafund input within an EventTransaction.
type SiafundInput struct {
	SiafundElement types.SiafundElement `json:"siafundElement"`
	ClaimElement   types.SiacoinElement `json:"claimElement"`
}

// A FileContract represents a file contract within an EventTransaction.
type FileContract struct {
	FileContract types.FileContractElement `json:"fileContract"`
	// only non-nil if transaction revised contract
	Revision *types.FileContract `json:"revision,omitempty"`
	// only non-nil if transaction resolved contract
	ValidOutputs []types.SiacoinElement `json:"validOutputs,omitempty"`
}

// A V2FileContract represents a v2 file contract within an EventTransaction.
type V2FileContract struct {
	FileContract types.V2FileContractElement `json:"fileContract"`
	// only non-nil if transaction revised contract
	Revision *types.V2FileContract `json:"revision,omitempty"`
	// only non-nil if transaction resolved contract
	Resolution types.V2FileContractResolutionType `json:"resolution,omitempty"`
	Outputs    []types.SiacoinElement             `json:"outputs,omitempty"`
}

type EventTransaction struct {
	ID                types.TransactionID    `json:"id"`
	SiacoinInputs     []types.SiacoinElement `json:"siacoinInputs"`
	SiacoinOutputs    []types.SiacoinElement `json:"siacoinOutputs"`
	SiafundInputs     []SiafundInput         `json:"siafundInputs"`
	SiafundOutputs    []types.SiafundElement `json:"siafundOutputs"`
	FileContracts     []FileContract         `json:"fileContracts"`
	V2FileContracts   []V2FileContract       `json:"v2FileContracts"`
	HostAnnouncements []HostAnnouncement     `json:"hostAnnouncements"`
	Fee               types.Currency         `json:"fee"`
}

type EventMinerPayout struct {
	SiacoinOutput types.SiacoinElement `json:"siacoinOutput"`
}

type EventMissedFileContract struct {
	FileContract  types.FileContractElement `json:"fileContract"`
	MissedOutputs []types.SiacoinElement    `json:"missedOutputs"`
}

// String implements fmt.Stringer.
func (et *EventTransaction) String() string {
	result := et.ID.String()
	if len(et.SiacoinOutputs) > 0 {
		result += ": Siacoin outputs: "
	}
	for i, sco := range et.SiacoinOutputs {
		result += sco.SiacoinOutput.Address.String()
		result += fmt.Sprintf(" (%s)", sco.SiacoinOutput.Value)
		if i < len(et.SiacoinOutputs)-1 {
			result += ", "
		}
	}
	if len(et.SiafundOutputs) > 0 {
		result += "; Siafund outputs: "
	}
	for i, sfo := range et.SiafundOutputs {
		result += sfo.SiafundOutput.Address.String()
		result += fmt.Sprintf(" (%d SF)", sfo.SiafundOutput.Value)
		if i < len(et.SiafundOutputs)-1 {
			result += ", "
		}
	}
	return result
}

// String implements fmt.Stringer.
func (emp *EventMinerPayout) String() string {
	return fmt.Sprintf("%s (%s)",
		emp.SiacoinOutput.SiacoinOutput.Address.String(),
		emp.SiacoinOutput.SiacoinOutput.Value,
	)
}

// String implements fmt.Stringer.
func (emfc *EventMissedFileContract) String() string {
	return emfc.FileContract.ID.String()
}

type ChainUpdate interface {
	ForEachSiacoinElement(func(sce types.SiacoinElement, spent bool))
	ForEachSiafundElement(func(sfe types.SiafundElement, spent bool))
	ForEachFileContractElement(func(fce types.FileContractElement, rev *types.FileContractElement, resolved, valid bool))
	ForEachV2FileContractElement(func(fce types.V2FileContractElement, rev *types.V2FileContractElement, res types.V2FileContractResolutionType))
}

// AppliedEvents extracts a list of relevant events from a chain update.
func AppliedEvents(cs consensus.State, b types.Block, cu ChainUpdate, addr types.Address) []Event {
	var events []Event
	addEvent := func(v interface{ eventType() string }) {
		events = append(events, Event{
			Timestamp: b.Timestamp,
			Index:     cs.Index,
			Val:       v,
		})
	}

	// do a first pass to see if there's anything relevant in the block
	relevantContract := func(fc types.FileContract) bool {
		for _, sco := range fc.ValidProofOutputs {
			if sco.Address == addr {
				return true
			}
		}
		for _, sco := range fc.MissedProofOutputs {
			if sco.Address == addr {
				return true
			}
		}
		return false
	}
	relevantV2Contract := func(fc types.V2FileContract) bool {
		if fc.RenterOutput.Address == addr {
			return true
		}
		if fc.HostOutput.Address == addr {
			return true
		}
		return false
	}
	relevantV2ContractResolution := func(res types.V2FileContractResolutionType) bool {
		switch r := res.(type) {
		case *types.V2FileContractFinalization:
			return relevantV2Contract(types.V2FileContract(*r))
		case *types.V2FileContractRenewal:
			return relevantV2Contract(r.InitialRevision) || relevantV2Contract(r.FinalRevision)
		}
		return false
	}
	anythingRelevant := func() (ok bool) {
		cu.ForEachSiacoinElement(func(sce types.SiacoinElement, spent bool) {
			if ok || sce.SiacoinOutput.Address == addr {
				ok = true
			}
		})
		cu.ForEachSiafundElement(func(sfe types.SiafundElement, spent bool) {
			if ok || sfe.SiafundOutput.Address == addr {
				ok = true
			}
		})
		cu.ForEachFileContractElement(func(fce types.FileContractElement, rev *types.FileContractElement, resolved, valid bool) {
			if ok || relevantContract(fce.FileContract) || (rev != nil && relevantContract(rev.FileContract)) {
				ok = true
			}
		})
		cu.ForEachV2FileContractElement(func(fce types.V2FileContractElement, rev *types.V2FileContractElement, res types.V2FileContractResolutionType) {
			if ok ||
				relevantV2Contract(fce.V2FileContract) ||
				(rev != nil && relevantV2Contract(rev.V2FileContract)) ||
				(res != nil && relevantV2ContractResolution(res)) {
				ok = true
			}
		})
		return
	}()
	if !anythingRelevant {
		return nil
	}

	// collect all elements
	sces := make(map[types.SiacoinOutputID]types.SiacoinElement)
	sfes := make(map[types.SiafundOutputID]types.SiafundElement)
	fces := make(map[types.FileContractID]types.FileContractElement)
	v2fces := make(map[types.FileContractID]types.V2FileContractElement)
	cu.ForEachSiacoinElement(func(sce types.SiacoinElement, spent bool) {
		sce.MerkleProof = nil
		sces[types.SiacoinOutputID(sce.ID)] = sce
	})
	cu.ForEachSiafundElement(func(sfe types.SiafundElement, spent bool) {
		sfe.MerkleProof = nil
		sfes[types.SiafundOutputID(sfe.ID)] = sfe
	})
	cu.ForEachFileContractElement(func(fce types.FileContractElement, rev *types.FileContractElement, resolved, valid bool) {
		fce.MerkleProof = nil
		fces[types.FileContractID(fce.ID)] = fce
	})
	cu.ForEachV2FileContractElement(func(fce types.V2FileContractElement, rev *types.V2FileContractElement, res types.V2FileContractResolutionType) {
		fce.MerkleProof = nil
		v2fces[types.FileContractID(fce.ID)] = fce
	})

	relevantTxn := func(txn types.Transaction) bool {
		for _, sci := range txn.SiacoinInputs {
			if sce := sces[sci.ParentID]; sce.SiacoinOutput.Address == addr {
				return true
			}
		}
		for _, sco := range txn.SiacoinOutputs {
			if sco.Address == addr {
				return true
			}
		}
		for _, sfi := range txn.SiafundInputs {
			if sfe := sfes[sfi.ParentID]; sfe.SiafundOutput.Address == addr {
				return true
			}
		}
		for _, sfo := range txn.SiafundOutputs {
			if sfo.Address == addr {
				return true
			}
		}
		for _, fc := range txn.FileContracts {
			if relevantContract(fc) {
				return true
			}
		}
		for _, fcr := range txn.FileContractRevisions {
			if relevantContract(fcr.FileContract) {
				return true
			}
		}
		for _, sp := range txn.StorageProofs {
			if relevantContract(fces[sp.ParentID].FileContract) {
				return true
			}
		}
		return false
	}

	relevantV2Txn := func(txn types.V2Transaction) bool {
		for _, sci := range txn.SiacoinInputs {
			if sci.Parent.SiacoinOutput.Address == addr {
				return true
			}
		}
		for _, sco := range txn.SiacoinOutputs {
			if sco.Address == addr {
				return true
			}
		}
		for _, sfi := range txn.SiafundInputs {
			if sfi.Parent.SiafundOutput.Address == addr {
				return true
			}
		}
		for _, sfo := range txn.SiafundOutputs {
			if sfo.Address == addr {
				return true
			}
		}
		for _, fc := range txn.FileContracts {
			if relevantV2Contract(fc) {
				return true
			}
		}
		for _, fcr := range txn.FileContractRevisions {
			if relevantV2Contract(fcr.Parent.V2FileContract) || relevantV2Contract(fcr.Revision) {
				return true
			}
		}
		for _, fcr := range txn.FileContractResolutions {
			if relevantV2Contract(fcr.Parent.V2FileContract) {
				return true
			}
			switch r := fcr.Resolution.(type) {
			case *types.V2FileContractFinalization:
				if relevantV2Contract(types.V2FileContract(*r)) {
					return true
				}
			case *types.V2FileContractRenewal:
				if relevantV2Contract(r.InitialRevision) || relevantV2Contract(r.FinalRevision) {
					return true
				}
			}
		}
		return false
	}

	// handle v1 transactions
	for _, txn := range b.Transactions {
		if !relevantTxn(txn) {
			continue
		}

		e := &EventTransaction{
			ID:             txn.ID(),
			SiacoinInputs:  make([]types.SiacoinElement, len(txn.SiacoinInputs)),
			SiacoinOutputs: make([]types.SiacoinElement, len(txn.SiacoinOutputs)),
			SiafundInputs:  make([]SiafundInput, len(txn.SiafundInputs)),
			SiafundOutputs: make([]types.SiafundElement, len(txn.SiafundOutputs)),
		}

		for i := range txn.SiacoinInputs {
			e.SiacoinInputs[i] = sces[txn.SiacoinInputs[i].ParentID]
		}
		for i := range txn.SiacoinOutputs {
			e.SiacoinOutputs[i] = sces[txn.SiacoinOutputID(i)]
		}
		for i := range txn.SiafundInputs {
			e.SiafundInputs[i] = SiafundInput{
				SiafundElement: sfes[txn.SiafundInputs[i].ParentID],
				ClaimElement:   sces[txn.SiafundClaimOutputID(i)],
			}
		}
		for i := range txn.SiafundOutputs {
			e.SiafundOutputs[i] = sfes[txn.SiafundOutputID(i)]
		}
		addContract := func(id types.FileContractID) *FileContract {
			for i := range e.FileContracts {
				if types.FileContractID(e.FileContracts[i].FileContract.ID) == id {
					return &e.FileContracts[i]
				}
			}
			e.FileContracts = append(e.FileContracts, FileContract{FileContract: fces[id]})
			return &e.FileContracts[len(e.FileContracts)-1]
		}
		for i := range txn.FileContracts {
			addContract(txn.FileContractID(i))
		}
		for i := range txn.FileContractRevisions {
			fc := addContract(txn.FileContractRevisions[i].ParentID)
			rev := txn.FileContractRevisions[i].FileContract
			fc.Revision = &rev
		}
		for i := range txn.StorageProofs {
			fc := addContract(txn.StorageProofs[i].ParentID)
			fc.ValidOutputs = make([]types.SiacoinElement, len(fc.FileContract.FileContract.ValidProofOutputs))
			for i := range fc.ValidOutputs {
				fc.ValidOutputs[i] = sces[types.FileContractID(fc.FileContract.ID).ValidOutputID(i)]
			}
		}
		for _, arb := range txn.ArbitraryData {
			var prefix types.Specifier
			var uk types.UnlockKey
			d := types.NewBufDecoder(arb)
			prefix.DecodeFrom(d)
			netAddress := d.ReadString()
			uk.DecodeFrom(d)
			if d.Err() == nil && prefix == types.NewSpecifier("HostAnnouncement") &&
				uk.Algorithm == types.SpecifierEd25519 && len(uk.Key) == len(types.PublicKey{}) {
				e.HostAnnouncements = append(e.HostAnnouncements, HostAnnouncement{
					PublicKey:  *(*types.PublicKey)(uk.Key),
					NetAddress: netAddress,
				})
			}
		}
		for i := range txn.MinerFees {
			e.Fee = e.Fee.Add(txn.MinerFees[i])
		}

		addEvent(e)
	}

	// handle v2 transactions
	for _, txn := range b.V2Transactions() {
		if !relevantV2Txn(txn) {
			continue
		}

		txid := txn.ID()
		e := &EventTransaction{
			ID:             txid,
			SiacoinInputs:  make([]types.SiacoinElement, len(txn.SiacoinInputs)),
			SiacoinOutputs: make([]types.SiacoinElement, len(txn.SiacoinOutputs)),
			SiafundInputs:  make([]SiafundInput, len(txn.SiafundInputs)),
			SiafundOutputs: make([]types.SiafundElement, len(txn.SiafundOutputs)),
		}
		for i := range txn.SiacoinInputs {
			// NOTE: here (and elsewhere), we fetch the element from our maps,
			// rather than using the parent directly, because our copy has its
			// Merkle proof nil'd out
			e.SiacoinInputs[i] = sces[types.SiacoinOutputID(txn.SiacoinInputs[i].Parent.ID)]
		}
		for i := range txn.SiacoinOutputs {
			e.SiacoinOutputs[i] = sces[txn.SiacoinOutputID(txid, i)]
		}
		for i := range txn.SiafundInputs {
			sfoid := types.SiafundOutputID(txn.SiafundInputs[i].Parent.ID)
			e.SiafundInputs[i] = SiafundInput{
				SiafundElement: sfes[sfoid],
				ClaimElement:   sces[sfoid.ClaimOutputID()],
			}
		}
		for i := range txn.SiafundOutputs {
			e.SiafundOutputs[i] = sfes[txn.SiafundOutputID(txid, i)]
		}
		addContract := func(id types.FileContractID) *V2FileContract {
			for i := range e.V2FileContracts {
				if types.FileContractID(e.V2FileContracts[i].FileContract.ID) == id {
					return &e.V2FileContracts[i]
				}
			}
			e.V2FileContracts = append(e.V2FileContracts, V2FileContract{FileContract: v2fces[id]})
			return &e.V2FileContracts[len(e.V2FileContracts)-1]
		}
		for i := range txn.FileContracts {
			addContract(txn.V2FileContractID(txid, i))
		}
		for _, fcr := range txn.FileContractRevisions {
			fc := addContract(types.FileContractID(fcr.Parent.ID))
			fc.Revision = &fcr.Revision
		}
		for _, fcr := range txn.FileContractResolutions {
			fc := addContract(types.FileContractID(fcr.Parent.ID))
			fc.Resolution = fcr.Resolution
			fc.Outputs = []types.SiacoinElement{
				sces[types.FileContractID(fcr.Parent.ID).V2RenterOutputID()],
				sces[types.FileContractID(fcr.Parent.ID).V2HostOutputID()],
			}
		}
		for _, a := range txn.Attestations {
			if a.Key == "HostAnnouncement" {
				e.HostAnnouncements = append(e.HostAnnouncements, HostAnnouncement{
					PublicKey:  a.PublicKey,
					NetAddress: string(a.Value),
				})
			}
		}

		e.Fee = txn.MinerFee
		addEvent(e)
	}

	// handle missed contracts
	cu.ForEachFileContractElement(func(fce types.FileContractElement, rev *types.FileContractElement, resolved, valid bool) {
		if resolved && !valid {
			if !relevantContract(fce.FileContract) {
				return
			}
			missedOutputs := make([]types.SiacoinElement, len(fce.FileContract.MissedProofOutputs))
			for i := range missedOutputs {
				missedOutputs[i] = sces[types.FileContractID(fce.ID).MissedOutputID(i)]
			}
			addEvent(&EventMissedFileContract{
				FileContract:  fce,
				MissedOutputs: missedOutputs,
			})
		}
	})

	// handle block rewards
	for i := range b.MinerPayouts {
		if b.MinerPayouts[i].Address == addr {
			addEvent(&EventMinerPayout{
				SiacoinOutput: sces[cs.Index.ID.MinerOutputID(i)],
			})
		}
	}

	return events
}

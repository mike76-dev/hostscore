package rhp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.sia.tech/mux/v1"
)

const (
	// responseLeeway is the amount of leeway given to the maxLen when we read
	// the response in the ReadSector RPC.
	responseLeeway = 1 << 12 // 4 KiB
)

var (
	// errMaxRevisionReached occurs when trying to revise a contract that has
	// already reached the highest possible revision number. Usually happens
	// when trying to use a renewed contract.
	errMaxRevisionReached = errors.New("contract has reached the maximum number of revisions")
)

// PriceTablePaymentFunc is a function that can be passed in to RPCPriceTable.
// It is called after the price table is received from the host and supposed to
// create a payment for that table and return it. It can also be used to perform
// gouging checks before paying for the table.
type PriceTablePaymentFunc func(pt rhpv3.HostPriceTable) (rhpv3.PaymentMethod, error)

// RPCPriceTable negotiates a price table with the host.
func RPCPriceTable(ctx context.Context, t *rhpv3.Transport, paymentFunc PriceTablePaymentFunc) (pt rhpv3.HostPriceTable, err error) {
	s := t.DialStream()
	defer s.Close()
	s.SetDeadline(time.Now().Add(5 * time.Second))

	const maxPriceTableSize = 16 * 1024
	var ptr rhpv3.RPCUpdatePriceTableResponse
	if err := s.WriteRequest(rhpv3.RPCUpdatePriceTableID, nil); err != nil {
		return rhpv3.HostPriceTable{}, err
	} else if err := s.ReadResponse(&ptr, maxPriceTableSize); err != nil {
		return rhpv3.HostPriceTable{}, err
	} else if err := json.Unmarshal(ptr.PriceTableJSON, &pt); err != nil {
		return rhpv3.HostPriceTable{}, err
	} else if payment, err := paymentFunc(pt); err != nil {
		return rhpv3.HostPriceTable{}, err
	} else if payment == nil {
		return pt, nil // Intended not to pay.
	} else if err := processPayment(s, payment); err != nil {
		return rhpv3.HostPriceTable{}, err
	} else if err := s.ReadResponse(&rhpv3.RPCPriceTableResponse{}, 0); err != nil {
		return rhpv3.HostPriceTable{}, err
	}
	return pt, nil
}

// processPayment carries out the payment for an RPC.
func processPayment(s *rhpv3.Stream, payment rhpv3.PaymentMethod) error {
	var paymentType types.Specifier
	switch payment.(type) {
	case *rhpv3.PayByContractRequest:
		paymentType = rhpv3.PaymentTypeContract
	case *rhpv3.PayByEphemeralAccountRequest:
		paymentType = rhpv3.PaymentTypeEphemeralAccount
	default:
		panic("unhandled payment method")
	}
	if err := s.WriteResponse(&paymentType); err != nil {
		return err
	} else if err := s.WriteResponse(payment); err != nil {
		return err
	}
	if _, ok := payment.(*rhpv3.PayByContractRequest); ok {
		var pr rhpv3.PaymentResponse
		if err := s.ReadResponse(&pr, 4096); err != nil {
			return err
		}
	}
	return nil
}

// RPCReadSector calls the ExecuteProgram RPC with a ReadSector instruction.
func RPCReadSector(ctx context.Context, t *rhpv3.Transport, w io.Writer, pt rhpv3.HostPriceTable, payment rhpv3.PaymentMethod, offset, length uint32, merkleRoot types.Hash256) (cost, refund types.Currency, err error) {
	s := t.DialStream()
	defer s.Close()

	var buf bytes.Buffer
	e := types.NewEncoder(&buf)
	e.WriteUint64(uint64(length))
	e.WriteUint64(uint64(offset))
	merkleRoot.EncodeTo(e)
	e.Flush()

	req := rhpv3.RPCExecuteProgramRequest{
		FileContractID: types.FileContractID{},
		Program: []rhpv3.Instruction{&rhpv3.InstrReadSector{
			LengthOffset:     0,
			OffsetOffset:     8,
			MerkleRootOffset: 16,
			ProofRequired:    true,
		}},
		ProgramData: buf.Bytes(),
	}

	var cancellationToken types.Specifier
	var resp rhpv3.RPCExecuteProgramResponse
	if err = s.WriteRequest(rhpv3.RPCExecuteProgramID, &pt.UID); err != nil {
		return
	} else if err = processPayment(s, payment); err != nil {
		return
	} else if err = s.WriteResponse(&req); err != nil {
		return
	} else if err = s.ReadResponse(&cancellationToken, 16); err != nil {
		return
	} else if err = s.ReadResponse(&resp, rhpv2.SectorSize+responseLeeway); err != nil {
		return
	}

	// Check response error.
	if err = resp.Error; err != nil {
		refund = resp.FailureRefund
		return
	}
	cost = resp.TotalCost

	// Verify proof.
	proofStart := int(offset) / utils.SegmentSize
	proofEnd := int(offset+length) / utils.SegmentSize
	if !utils.VerifyRangeProof(resp.Output, resp.Proof, proofStart, proofEnd, merkleRoot) {
		err = errors.New("proof verification failed")
		return
	}

	_, err = w.Write(resp.Output)
	return
}

// RPCAppendSector calls the ExecuteProgram RPC with an AppendSector instruction.
func RPCAppendSector(ctx context.Context, t *rhpv3.Transport, renterKey types.PrivateKey, pt rhpv3.HostPriceTable, rev types.FileContractRevision, payment rhpv3.PaymentMethod, sector *[rhpv2.SectorSize]byte) (sectorRoot types.Hash256, cost types.Currency, err error) {
	// Sanity check revision first.
	if rev.RevisionNumber == math.MaxUint64 {
		return types.Hash256{}, types.ZeroCurrency, errMaxRevisionReached
	}

	s := t.DialStream()
	defer s.Close()

	req := rhpv3.RPCExecuteProgramRequest{
		FileContractID: rev.ParentID,
		Program: []rhpv3.Instruction{&rhpv3.InstrAppendSector{
			SectorDataOffset: 0,
			ProofRequired:    true,
		}},
		ProgramData: (*sector)[:],
	}

	var cancellationToken types.Specifier
	var executeResp rhpv3.RPCExecuteProgramResponse
	if err = s.WriteRequest(rhpv3.RPCExecuteProgramID, &pt.UID); err != nil {
		return
	} else if err = processPayment(s, payment); err != nil {
		return
	} else if err = s.WriteResponse(&req); err != nil {
		return
	} else if err = s.ReadResponse(&cancellationToken, 16); err != nil {
		return
	} else if err = s.ReadResponse(&executeResp, 65536); err != nil {
		return
	}

	// Compute expected collateral and refund.
	expectedCost, expectedCollateral, expectedRefund, err := uploadSectorCost(pt, rev.WindowEnd)
	if err != nil {
		return types.Hash256{}, types.ZeroCurrency, err
	}

	// Apply leeways.
	// TODO: remove once most hosts use hostd. Then we can check for exact values.
	expectedCollateral = expectedCollateral.Mul64(9).Div64(10)
	expectedCost = expectedCost.Mul64(11).Div64(10)
	expectedRefund = expectedRefund.Mul64(9).Div64(10)

	// Check if the cost, collateral and refund match our expectation.
	if executeResp.TotalCost.Cmp(expectedCost) > 0 {
		return types.Hash256{}, types.ZeroCurrency, fmt.Errorf("cost exceeds expectation: %v > %v", executeResp.TotalCost, expectedCost)
	}
	if executeResp.FailureRefund.Cmp(expectedRefund) < 0 {
		return types.Hash256{}, types.ZeroCurrency, fmt.Errorf("insufficient refund: %v < %v", executeResp.FailureRefund, expectedRefund)
	}
	if executeResp.AdditionalCollateral.Cmp(expectedCollateral) < 0 {
		return types.Hash256{}, types.ZeroCurrency, fmt.Errorf("insufficient collateral: %v < %v", executeResp.AdditionalCollateral, expectedCollateral)
	}

	// Set the cost and refund.
	cost = executeResp.TotalCost
	defer func() {
		if err != nil {
			cost = types.ZeroCurrency
			if executeResp.FailureRefund.Cmp(cost) < 0 {
				cost = cost.Sub(executeResp.FailureRefund)
			}
		}
	}()

	// Check response error.
	if err = executeResp.Error; err != nil {
		return
	}
	cost = executeResp.TotalCost

	// Include the refund in the collateral.
	collateral := executeResp.AdditionalCollateral.Add(executeResp.FailureRefund)

	// Check proof.
	sectorRoot = rhpv2.SectorRoot(sector)
	if rev.Filesize == 0 {
		// For the first upload to a contract we don't get a proof. So we just
		// assert that the new contract root matches the root of the sector.
		if rev.Filesize == 0 && executeResp.NewMerkleRoot != sectorRoot {
			return types.Hash256{}, types.ZeroCurrency, fmt.Errorf("merkle root doesn't match the sector root upon first upload to contract: %v != %v", executeResp.NewMerkleRoot, sectorRoot)
		}
	} else {
		// Otherwise we make sure the proof was transmitted and verify it.
		actions := []rhpv2.RPCWriteAction{{Type: rhpv2.RPCWriteActionAppend}} // TODO: change once rhpv3 support is available.
		if !rhpv2.VerifyDiffProof(actions, rev.Filesize/rhpv2.SectorSize, executeResp.Proof, []types.Hash256{}, rev.FileMerkleRoot, executeResp.NewMerkleRoot, []types.Hash256{sectorRoot}) {
			return types.Hash256{}, types.ZeroCurrency, errors.New("proof verification failed")
		}
	}

	// Finalize the program with a new revision.
	newRevision := rev
	newValid, newMissed, err := updateRevisionOutputs(&newRevision, types.ZeroCurrency, collateral)
	if err != nil {
		return types.Hash256{}, types.ZeroCurrency, err
	}
	newRevision.Filesize += rhpv2.SectorSize
	newRevision.RevisionNumber++
	newRevision.FileMerkleRoot = executeResp.NewMerkleRoot

	h := types.NewHasher()
	newRevision.EncodeTo(h.E)
	finalizeReq := rhpv3.RPCFinalizeProgramRequest{
		Signature:         renterKey.SignHash(h.Sum()),
		ValidProofValues:  newValid,
		MissedProofValues: newMissed,
		RevisionNumber:    newRevision.RevisionNumber,
	}

	var finalizeResp rhpv3.RPCFinalizeProgramResponse
	if err = s.WriteResponse(&finalizeReq); err != nil {
		return
	} else if err = s.ReadResponse(&finalizeResp, 64); err != nil {
		return
	}

	// Read one more time to receive a potential error in case finalising the
	// contract fails after receiving the RPCFinalizeProgramResponse. This also
	// guarantees that the program is finalised before we return.
	// TODO: remove once most hosts use hostd.
	errFinalise := s.ReadResponse(&finalizeResp, 64)
	if errFinalise != nil &&
		!errors.Is(errFinalise, io.EOF) &&
		!errors.Is(errFinalise, mux.ErrClosedConn) &&
		!errors.Is(errFinalise, mux.ErrClosedStream) &&
		!errors.Is(errFinalise, mux.ErrPeerClosedStream) &&
		!errors.Is(errFinalise, mux.ErrPeerClosedConn) {
		err = errFinalise
		return
	}
	return
}

// padBandwitdh pads the bandwidth to the next multiple of 1460 bytes.  1460
// bytes is the maximum size of a TCP packet when using IPv4.
// TODO: once hostd becomes the only host implementation we can simplify this.
func padBandwidth(pt rhpv3.HostPriceTable, rc rhpv3.ResourceCost) rhpv3.ResourceCost {
	padCost := func(cost, paddingSize types.Currency) types.Currency {
		if paddingSize.IsZero() {
			return cost // Might happen if bandwidth is free.
		}
		return cost.Add(paddingSize).Sub(types.NewCurrency64(1)).Div(paddingSize).Mul(paddingSize)
	}
	minPacketSize := uint64(1460)
	minIngress := pt.UploadBandwidthCost.Mul64(minPacketSize)
	minEgress := pt.DownloadBandwidthCost.Mul64(3*minPacketSize + responseLeeway)
	rc.Ingress = padCost(rc.Ingress, minIngress)
	rc.Egress = padCost(rc.Egress, minEgress)
	return rc
}

// uploadSectorCost returns an overestimate for the cost of uploading a sector
// to a host.
func uploadSectorCost(pt rhpv3.HostPriceTable, windowEnd uint64) (cost, collateral, storage types.Currency, _ error) {
	rc := pt.BaseCost()
	rc = rc.Add(pt.AppendSectorCost(windowEnd - pt.HostBlockHeight))
	rc = padBandwidth(pt, rc)
	cost, collateral = rc.Total()

	// Overestimate the cost by 10%.
	cost, overflow := cost.Mul64WithOverflow(11)
	if overflow {
		return types.ZeroCurrency, types.ZeroCurrency, types.ZeroCurrency, errors.New("overflow occurred while adding leeway to read sector cost")
	}
	return cost.Div64(10), collateral, rc.Storage, nil
}

// updateRevisionOutputs updates the revision outputs with new values.
func updateRevisionOutputs(rev *types.FileContractRevision, cost, collateral types.Currency) (valid, missed []types.Currency, err error) {
	// Allocate new slices; don't want to risk accidentally sharing memory.
	rev.ValidProofOutputs = append([]types.SiacoinOutput(nil), rev.ValidProofOutputs...)
	rev.MissedProofOutputs = append([]types.SiacoinOutput(nil), rev.MissedProofOutputs...)

	// Move valid payout from renter to host.
	var underflow, overflow bool
	rev.ValidProofOutputs[0].Value, underflow = rev.ValidProofOutputs[0].Value.SubWithUnderflow(cost)
	rev.ValidProofOutputs[1].Value, overflow = rev.ValidProofOutputs[1].Value.AddWithOverflow(cost)
	if underflow || overflow {
		err = errors.New("insufficient funds to pay host")
		return
	}

	// Move missed payout from renter to void.
	rev.MissedProofOutputs[0].Value, underflow = rev.MissedProofOutputs[0].Value.SubWithUnderflow(cost)
	rev.MissedProofOutputs[2].Value, overflow = rev.MissedProofOutputs[2].Value.AddWithOverflow(cost)
	if underflow || overflow {
		err = errors.New("insufficient funds to move missed payout to void")
		return
	}

	// Move collateral from host to void.
	rev.MissedProofOutputs[1].Value, underflow = rev.MissedProofOutputs[1].Value.SubWithUnderflow(collateral)
	rev.MissedProofOutputs[2].Value, overflow = rev.MissedProofOutputs[2].Value.AddWithOverflow(collateral)
	if underflow || overflow {
		err = errors.New("insufficient collateral")
		return
	}

	return []types.Currency{rev.ValidProofOutputs[0].Value, rev.ValidProofOutputs[1].Value},
		[]types.Currency{rev.MissedProofOutputs[0].Value, rev.MissedProofOutputs[1].Value, rev.MissedProofOutputs[2].Value}, nil
}

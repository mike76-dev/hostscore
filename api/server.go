package api

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/syncer"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/gateway"
	"go.sia.tech/core/types"
	"go.sia.tech/jape"
	"lukechampine.com/frand"
)

type (
	// A ChainManager manages blockchain and txpool state.
	ChainManager interface {
		TipState() consensus.State
		AddBlocks([]types.Block) error
		RecommendedFee() types.Currency
		PoolTransactions() []types.Transaction
		V2PoolTransactions() []types.V2Transaction
		AddPoolTransactions(txns []types.Transaction) (bool, error)
		AddV2PoolTransactions(index types.ChainIndex, txns []types.V2Transaction) (bool, error)
		UnconfirmedParents(txn types.Transaction) []types.Transaction
	}

	// A Syncer can connect to other peers and synchronize the blockchain.
	Syncer interface {
		Addr() string
		Peers() []*gateway.Peer
		PeerInfo(peer string) (syncer.PeerInfo, bool)
		Connect(addr string) (*gateway.Peer, error)
		BroadcastHeader(bh gateway.BlockHeader)
		BroadcastTransactionSet(txns []types.Transaction)
		BroadcastV2TransactionSet(index types.ChainIndex, txns []types.V2Transaction)
		BroadcastV2BlockOutline(bo gateway.V2BlockOutline)
	}

	// A Wallet manages the wallet.
	Wallet interface {
		Address() types.Address
		Events(offset, limit int) ([]wallet.Event, error)
		UnspentOutputs() ([]types.SiacoinElement, []types.SiafundElement, error)
		Annotate(pool []types.Transaction) ([]wallet.PoolTransaction, error)
	}
)

type server struct {
	cm ChainManager
	s  Syncer
	w  Wallet

	// for walletsReserveHandler
	mu   sync.Mutex
	used map[types.Hash256]bool
}

func (s *server) consensusNetworkHandler(jc jape.Context) {
	jc.Encode(*s.cm.TipState().Network)
}

func (s *server) consensusTipHandler(jc jape.Context) {
	jc.Encode(s.cm.TipState().Index)
}

func (s *server) consensusTipStateHandler(jc jape.Context) {
	jc.Encode(s.cm.TipState())
}

func (s *server) syncerPeersHandler(jc jape.Context) {
	var peers []GatewayPeer
	for _, p := range s.s.Peers() {
		info, ok := s.s.PeerInfo(p.Addr)
		if !ok {
			continue
		}
		peers = append(peers, GatewayPeer{
			Addr:    p.Addr,
			Inbound: p.Inbound,
			Version: p.Version,

			FirstSeen:      info.FirstSeen,
			ConnectedSince: info.LastConnect,
			SyncedBlocks:   info.SyncedBlocks,
			SyncDuration:   info.SyncDuration,
		})
	}
	jc.Encode(peers)
}

func (s *server) syncerConnectHandler(jc jape.Context) {
	var addr string
	if jc.Decode(&addr) != nil {
		return
	}
	_, err := s.s.Connect(addr)
	jc.Check("couldn't connect to peer", err)
}

func (s *server) syncerBroadcastBlockHandler(jc jape.Context) {
	var b types.Block
	if jc.Decode(&b) != nil {
		return
	} else if jc.Check("block is invalid", s.cm.AddBlocks([]types.Block{b})) != nil {
		return
	}
	if b.V2 == nil {
		s.s.BroadcastHeader(gateway.BlockHeader{
			ParentID:   b.ParentID,
			Nonce:      b.Nonce,
			Timestamp:  b.Timestamp,
			MerkleRoot: b.MerkleRoot(),
		})
	} else {
		s.s.BroadcastV2BlockOutline(gateway.OutlineBlock(b, s.cm.PoolTransactions(), s.cm.V2PoolTransactions()))
	}
}

func (s *server) txpoolTransactionsHandler(jc jape.Context) {
	jc.Encode(TxpoolTransactionsResponse{
		Transactions:   s.cm.PoolTransactions(),
		V2Transactions: s.cm.V2PoolTransactions(),
	})
}

func (s *server) txpoolFeeHandler(jc jape.Context) {
	jc.Encode(s.cm.RecommendedFee())
}

func (s *server) txpoolBroadcastHandler(jc jape.Context) {
	var tbr TxpoolBroadcastRequest
	if jc.Decode(&tbr) != nil {
		return
	}
	if len(tbr.Transactions) != 0 {
		_, err := s.cm.AddPoolTransactions(tbr.Transactions)
		if jc.Check("invalid transaction set", err) != nil {
			return
		}
		s.s.BroadcastTransactionSet(tbr.Transactions)
	}
	if len(tbr.V2Transactions) != 0 {
		index := s.cm.TipState().Index
		_, err := s.cm.AddV2PoolTransactions(index, tbr.V2Transactions)
		if jc.Check("invalid v2 transaction set", err) != nil {
			return
		}
		s.s.BroadcastV2TransactionSet(index, tbr.V2Transactions)
	}
}

func (s *server) walletAddressHandlerGET(jc jape.Context) {
	addr := s.w.Address()
	jc.Encode(addr)
}

func (s *server) walletBalanceHandler(jc jape.Context) {
	scos, sfos, err := s.w.UnspentOutputs()
	if jc.Check("couldn't load outputs", err) != nil {
		return
	}
	height := s.cm.TipState().Index.Height
	var sc, immature types.Currency
	var sf uint64
	for _, sco := range scos {
		if height >= sco.MaturityHeight {
			sc = sc.Add(sco.SiacoinOutput.Value)
		} else {
			immature = immature.Add(sco.SiacoinOutput.Value)
		}
	}
	for _, sfo := range sfos {
		sf += sfo.SiafundOutput.Value
	}
	jc.Encode(WalletBalanceResponse{
		Siacoins:         sc,
		ImmatureSiacoins: immature,
		Siafunds:         sf,
	})
}

func (s *server) walletEventsHandler(jc jape.Context) {
	offset, limit := 0, -1
	if jc.DecodeForm("offset", &offset) != nil || jc.DecodeForm("limit", &limit) != nil {
		return
	}
	events, err := s.w.Events(offset, limit)
	if jc.Check("couldn't load events", err) != nil {
		return
	}
	jc.Encode(events)
}

func (s *server) walletTxpoolHandler(jc jape.Context) {
	pool, err := s.w.Annotate(s.cm.PoolTransactions())
	if jc.Check("couldn't annotate pool", err) != nil {
		return
	}
	jc.Encode(pool)
}

func (s *server) walletOutputsHandler(jc jape.Context) {
	scos, sfos, err := s.w.UnspentOutputs()
	if jc.Check("couldn't load outputs", err) != nil {
		return
	}
	jc.Encode(WalletOutputsResponse{
		SiacoinOutputs: scos,
		SiafundOutputs: sfos,
	})
}

func (s *server) walletReserveHandler(jc jape.Context) {
	var wrr WalletReserveRequest
	if jc.Decode(&wrr) != nil {
		return
	}

	s.mu.Lock()
	for _, id := range wrr.SiacoinOutputs {
		if s.used[types.Hash256(id)] {
			s.mu.Unlock()
			jc.Error(fmt.Errorf("output %v is already reserved", id), http.StatusBadRequest)
			return
		}
		s.used[types.Hash256(id)] = true
	}
	for _, id := range wrr.SiafundOutputs {
		if s.used[types.Hash256(id)] {
			s.mu.Unlock()
			jc.Error(fmt.Errorf("output %v is already reserved", id), http.StatusBadRequest)
			return
		}
		s.used[types.Hash256(id)] = true
	}
	s.mu.Unlock()

	if wrr.Duration == 0 {
		wrr.Duration = 10 * time.Minute
	}
	time.AfterFunc(wrr.Duration, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, id := range wrr.SiacoinOutputs {
			delete(s.used, types.Hash256(id))
		}
		for _, id := range wrr.SiafundOutputs {
			delete(s.used, types.Hash256(id))
		}
	})
}

func (s *server) walletReleaseHandler(jc jape.Context) {
	var wrr WalletReleaseRequest
	if jc.Decode(&wrr) != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range wrr.SiacoinOutputs {
		delete(s.used, types.Hash256(id))
	}
	for _, id := range wrr.SiafundOutputs {
		delete(s.used, types.Hash256(id))
	}
}

func (s *server) walletFundHandler(jc jape.Context) {
	fundTxn := func(txn *types.Transaction, amount types.Currency, utxos []types.SiacoinElement, changeAddr types.Address, pool []types.Transaction) ([]types.Hash256, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if amount.IsZero() {
			return nil, nil
		}
		inPool := make(map[types.Hash256]bool)
		for _, ptxn := range pool {
			for _, in := range ptxn.SiacoinInputs {
				inPool[types.Hash256(in.ParentID)] = true
			}
		}
		frand.Shuffle(len(utxos), reflect.Swapper(utxos))
		var outputSum types.Currency
		var fundingElements []types.SiacoinElement
		for _, sce := range utxos {
			if s.used[types.Hash256(sce.ID)] || inPool[types.Hash256(sce.ID)] {
				continue
			}
			fundingElements = append(fundingElements, sce)
			outputSum = outputSum.Add(sce.SiacoinOutput.Value)
			if outputSum.Cmp(amount) >= 0 {
				break
			}
		}
		if outputSum.Cmp(amount) < 0 {
			return nil, errors.New("insufficient balance")
		} else if outputSum.Cmp(amount) > 0 {
			if changeAddr == types.VoidAddress {
				return nil, errors.New("change address must be specified")
			}
			txn.SiacoinOutputs = append(txn.SiacoinOutputs, types.SiacoinOutput{
				Value:   outputSum.Sub(amount),
				Address: changeAddr,
			})
		}

		toSign := make([]types.Hash256, len(fundingElements))
		for i, sce := range fundingElements {
			txn.SiacoinInputs = append(txn.SiacoinInputs, types.SiacoinInput{
				ParentID: types.SiacoinOutputID(sce.ID),
				// UnlockConditions left empty for client to fill in
			})
			toSign[i] = types.Hash256(sce.ID)
			s.used[types.Hash256(sce.ID)] = true
		}

		return toSign, nil
	}

	var wfr WalletFundRequest
	if jc.Decode(&wfr) != nil {
		return
	}
	utxos, _, err := s.w.UnspentOutputs()
	if jc.Check("couldn't get utxos to fund transaction", err) != nil {
		return
	}

	txn := wfr.Transaction
	toSign, err := fundTxn(&txn, wfr.Amount, utxos, wfr.ChangeAddress, s.cm.PoolTransactions())
	if jc.Check("couldn't fund transaction", err) != nil {
		return
	}
	jc.Encode(WalletFundResponse{
		Transaction: txn,
		ToSign:      toSign,
		DependsOn:   s.cm.UnconfirmedParents(txn),
	})
}

func (s *server) walletFundSFHandler(jc jape.Context) {
	fundTxn := func(txn *types.Transaction, amount uint64, utxos []types.SiafundElement, changeAddr, claimAddr types.Address, pool []types.Transaction) ([]types.Hash256, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if amount == 0 {
			return nil, nil
		}
		inPool := make(map[types.Hash256]bool)
		for _, ptxn := range pool {
			for _, in := range ptxn.SiafundInputs {
				inPool[types.Hash256(in.ParentID)] = true
			}
		}
		frand.Shuffle(len(utxos), reflect.Swapper(utxos))
		var outputSum uint64
		var fundingElements []types.SiafundElement
		for _, sfe := range utxos {
			if s.used[types.Hash256(sfe.ID)] || inPool[types.Hash256(sfe.ID)] {
				continue
			}
			fundingElements = append(fundingElements, sfe)
			outputSum += sfe.SiafundOutput.Value
			if outputSum >= amount {
				break
			}
		}
		if outputSum < amount {
			return nil, errors.New("insufficient balance")
		} else if outputSum > amount {
			if changeAddr == types.VoidAddress {
				return nil, errors.New("change address must be specified")
			}
			txn.SiafundOutputs = append(txn.SiafundOutputs, types.SiafundOutput{
				Value:   outputSum - amount,
				Address: changeAddr,
			})
		}

		toSign := make([]types.Hash256, len(fundingElements))
		for i, sfe := range fundingElements {
			txn.SiafundInputs = append(txn.SiafundInputs, types.SiafundInput{
				ParentID:     types.SiafundOutputID(sfe.ID),
				ClaimAddress: claimAddr,
				// UnlockConditions left empty for client to fill in
			})
			toSign[i] = types.Hash256(sfe.ID)
			s.used[types.Hash256(sfe.ID)] = true
		}

		return toSign, nil
	}

	var wfr WalletFundSFRequest
	if jc.Decode(&wfr) != nil {
		return
	}
	_, utxos, err := s.w.UnspentOutputs()
	if jc.Check("couldn't get utxos to fund transaction", err) != nil {
		return
	}

	txn := wfr.Transaction
	toSign, err := fundTxn(&txn, wfr.Amount, utxos, wfr.ChangeAddress, wfr.ClaimAddress, s.cm.PoolTransactions())
	if jc.Check("couldn't fund transaction", err) != nil {
		return
	}
	jc.Encode(WalletFundResponse{
		Transaction: txn,
		ToSign:      toSign,
		DependsOn:   s.cm.UnconfirmedParents(txn),
	})
}

// NewServer returns an HTTP handler that serves the hsd API.
func NewServer(cm ChainManager, s Syncer, w Wallet) http.Handler {
	srv := server{
		cm:   cm,
		s:    s,
		w:    w,
		used: make(map[types.Hash256]bool),
	}
	return jape.Mux(map[string]jape.Handler{
		"GET /consensus/network":  srv.consensusNetworkHandler,
		"GET /consensus/tip":      srv.consensusTipHandler,
		"GET /consensus/tipstate": srv.consensusTipStateHandler,

		"GET  /syncer/peers":           srv.syncerPeersHandler,
		"POST /syncer/connect":         srv.syncerConnectHandler,
		"POST /syncer/broadcast/block": srv.syncerBroadcastBlockHandler,

		"GET  /txpool/transactions": srv.txpoolTransactionsHandler,
		"GET  /txpool/fee":          srv.txpoolFeeHandler,
		"POST /txpool/broadcast":    srv.txpoolBroadcastHandler,

		"GET    /wallet/address": srv.walletAddressHandlerGET,
		"GET    /wallet/balance": srv.walletBalanceHandler,
		"GET    /wallet/events":  srv.walletEventsHandler,
		"GET    /wallet/txpool":  srv.walletTxpoolHandler,
		"GET    /wallet/outputs": srv.walletOutputsHandler,
		"POST   /wallet/reserve": srv.walletReserveHandler,
		"POST   /wallet/release": srv.walletReleaseHandler,
		"POST   /wallet/fund":    srv.walletFundHandler,
		"POST   /wallet/fundsf":  srv.walletFundSFHandler,
	})
}

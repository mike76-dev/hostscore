package api

import (
	"net/http"
	"time"

	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/internal/walletutil"
	"github.com/mike76-dev/hostscore/syncer"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/jape"
)

type server struct {
	cm  *chain.Manager
	s   *syncer.Syncer
	w   *walletutil.Wallet
	hdb *hostdb.HostDB
}

func (s *server) consensusNetworkHandler(jc jape.Context) {
	jc.Encode(*s.cm.TipState().Network)
}

func (s *server) consensusTipHandler(jc jape.Context) {
	resp := ConsensusTipResponse{
		Height:  s.cm.TipState().Index.Height,
		BlockID: s.cm.TipState().Index.ID,
		Synced:  s.s.Synced(),
	}
	jc.Encode(resp)
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

func (s *server) txpoolTransactionsHandler(jc jape.Context) {
	jc.Encode(TxpoolTransactionsResponse{
		Transactions:   s.cm.PoolTransactions(),
		V2Transactions: s.cm.V2PoolTransactions(),
	})
}

func (s *server) txpoolFeeHandler(jc jape.Context) {
	jc.Encode(s.cm.RecommendedFee())
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

func (s *server) hostDBHostsHandler(jc jape.Context) {
	offset, limit := 0, -1
	if jc.DecodeForm("offset", &offset) != nil || jc.DecodeForm("limit", &limit) != nil {
		return
	}
	hosts := s.hdb.Hosts(offset, limit)
	jc.Encode(hosts)
}

func (s *server) hostDBScansHandler(jc jape.Context) {
	var from, to time.Time
	var pk types.PublicKey
	if jc.DecodeForm("from", &from) != nil ||
		jc.DecodeForm("to", &to) != nil ||
		jc.DecodeForm("host", &pk) != nil {
		return
	}
	scans, err := s.hdb.Scans(pk, from, to)
	if jc.Check("couldn't get scan history", err) != nil {
		return
	}
	jc.Encode(scans)
}

func (s *server) hostDBScanHistoryHandler(jc jape.Context) {
	var from, to time.Time
	if jc.DecodeForm("from", &from) != nil || jc.DecodeForm("to", &to) != nil {
		return
	}
	history, err := s.hdb.ScanHistory(from, to)
	if jc.Check("couldn't get scan history", err) != nil {
		return
	}
	jc.Encode(history)
}

func (s *server) hostDBBenchmarksHandler(jc jape.Context) {
	var from, to time.Time
	var pk types.PublicKey
	if jc.DecodeForm("from", &from) != nil ||
		jc.DecodeForm("to", &to) != nil ||
		jc.DecodeForm("host", &pk) != nil {
		return
	}
	benchmarks, err := s.hdb.Benchmarks(pk, from, to)
	if jc.Check("couldn't get benchmark history", err) != nil {
		return
	}
	jc.Encode(benchmarks)
}

func (s *server) hostDBBenchmarkHistoryHandler(jc jape.Context) {
	var from, to time.Time
	if jc.DecodeForm("from", &from) != nil || jc.DecodeForm("to", &to) != nil {
		return
	}
	history, err := s.hdb.BenchmarkHistory(from, to)
	if jc.Check("couldn't get benchmark history", err) != nil {
		return
	}
	jc.Encode(history)
}

// NewServer returns an HTTP handler that serves the hsd API.
func NewServer(cm *chain.Manager, s *syncer.Syncer, w *walletutil.Wallet, hdb *hostdb.HostDB) http.Handler {
	srv := server{
		cm:  cm,
		s:   s,
		w:   w,
		hdb: hdb,
	}
	return jape.Mux(map[string]jape.Handler{
		"GET /consensus/network":  srv.consensusNetworkHandler,
		"GET /consensus/tip":      srv.consensusTipHandler,
		"GET /consensus/tipstate": srv.consensusTipStateHandler,

		"GET  /syncer/peers": srv.syncerPeersHandler,

		"GET  /txpool/transactions": srv.txpoolTransactionsHandler,
		"GET  /txpool/fee":          srv.txpoolFeeHandler,

		"GET    /wallet/address": srv.walletAddressHandlerGET,
		"GET    /wallet/balance": srv.walletBalanceHandler,
		"GET    /wallet/txpool":  srv.walletTxpoolHandler,
		"GET    /wallet/outputs": srv.walletOutputsHandler,

		"GET    /hostdb/hosts":              srv.hostDBHostsHandler,
		"GET    /hostdb/scans":              srv.hostDBScansHandler,
		"GET    /hostdb/scans/history":      srv.hostDBScanHistoryHandler,
		"GET    /hostdb/benchmarks":         srv.hostDBBenchmarksHandler,
		"GET    /hostdb/benchmarks/history": srv.hostDBBenchmarkHistoryHandler,
	})
}

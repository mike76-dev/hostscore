package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/internal/walletutil"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/coreutils/syncer"
	"go.sia.tech/jape"
)

type server struct {
	cm    *chain.Manager
	cmZen *chain.Manager
	s     *syncer.Syncer
	sZen  *syncer.Syncer
	w     *walletutil.Wallet
	hdb   *hostdb.HostDB
}

func isSynced(s *syncer.Syncer) bool {
	var count int
	for _, p := range s.Peers() {
		if p.Synced() {
			count++
		}
	}
	return count >= 5
}

func (s *server) consensusNetworkHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network == "" || network == "mainnet" {
		jc.Encode(*s.cm.TipState().Network)
		return
	}
	if network == "zen" {
		jc.Encode(*s.cmZen.TipState().Network)
		return
	}
	jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
}

func (s *server) consensusTipHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}
	var state consensus.State
	var synced bool
	if network == "" || network == "mainnet" {
		network = "Mainnet"
		state = s.cm.TipState()
		synced = isSynced(s.s)
	} else {
		network = "Zen"
		state = s.cmZen.TipState()
		synced = isSynced(s.sZen)
	}
	synced = synced && time.Since(state.PrevTimestamps[0]) < 24*time.Hour
	resp := ConsensusTipResponse{
		Network: network,
		Height:  state.Index.Height,
		BlockID: state.Index.ID,
		Synced:  synced,
	}
	jc.Encode(resp)
}

func (s *server) consensusTipStateHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network == "" || network == "mainnet" {
		jc.Encode(s.cm.TipState())
		return
	}
	if network == "zen" {
		jc.Encode(s.cmZen.TipState())
		return
	}
	jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
}

func (s *server) syncerPeersHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}
	var peers []GatewayPeer
	var ps []*syncer.Peer
	if network == "" || network == "mainnet" {
		ps = s.s.Peers()
	} else {
		ps = s.sZen.Peers()
	}
	for _, p := range ps {
		peers = append(peers, GatewayPeer{
			Addr:    p.Addr(),
			Inbound: p.Inbound,
			Version: p.Version(),
		})
	}
	jc.Encode(peers)
}

func (s *server) txpoolTransactionsHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}
	var txns []types.Transaction
	var v2txns []types.V2Transaction
	if network == "" || network == "mainnet" {
		txns = s.cm.PoolTransactions()
		v2txns = s.cm.V2PoolTransactions()
	} else {
		txns = s.cmZen.PoolTransactions()
		v2txns = s.cmZen.V2PoolTransactions()
	}
	jc.Encode(TxpoolTransactionsResponse{
		Transactions:   txns,
		V2Transactions: v2txns,
	})
}

func (s *server) txpoolFeeHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}
	if network == "" || network == "mainnet" {
		jc.Encode(s.cm.RecommendedFee())
	} else {
		jc.Encode(s.cmZen.RecommendedFee())
	}
}

func (s *server) walletAddressHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}
	if network == "" {
		network = "mainnet"
	}
	addr := s.w.Address(network)
	jc.Encode(addr)
}

func (s *server) walletBalanceHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}
	if network == "" {
		network = "mainnet"
	}

	scos, sfos, err := s.w.UnspentOutputs(network)
	if jc.Check("couldn't load outputs", err) != nil {
		return
	}

	var height uint64
	if network == "zen" {
		height = s.cmZen.TipState().Index.Height
	} else {
		height = s.cm.TipState().Index.Height
	}

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
		Network:          strings.ToUpper(string(network[0])) + network[1:],
		Siacoins:         sc,
		ImmatureSiacoins: immature,
		Siafunds:         sf,
	})
}

func (s *server) walletTxpoolHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}
	if network == "" {
		network = "mainnet"
	}

	var txns []types.Transaction
	if network == "zen" {
		txns = s.cmZen.PoolTransactions()
	} else {
		txns = s.cm.PoolTransactions()
	}

	pool, err := s.w.Annotate(network, txns)
	if jc.Check("couldn't annotate pool", err) != nil {
		return
	}
	jc.Encode(pool)
}

func (s *server) walletOutputsHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}
	if network == "" {
		network = "mainnet"
	}

	scos, sfos, err := s.w.UnspentOutputs(network)
	if jc.Check("couldn't load outputs", err) != nil {
		return
	}
	jc.Encode(WalletOutputsResponse{
		Network:        strings.ToUpper(string(network[0])) + network[1:],
		SiacoinOutputs: scos,
		SiafundOutputs: sfos,
	})
}

func (s *server) hostDBHostsHandler(jc jape.Context) {
	var network, query, allHosts string
	if jc.DecodeForm("network", &network) != nil ||
		jc.DecodeForm("query", &query) != nil ||
		jc.DecodeForm("all", &allHosts) != nil {
		return
	}

	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}
	if network == "" {
		network = "mainnet"
	}

	offset, limit := 0, -1
	if jc.DecodeForm("offset", &offset) != nil || jc.DecodeForm("limit", &limit) != nil {
		return
	}

	var all bool
	if allHosts == "true" {
		all = true
	}

	hosts, more, total := s.hdb.Hosts(network, all, offset, limit, query)
	jc.Encode(HostdbHostsResponse{
		Hosts: hosts,
		More:  more,
		Total: total,
	})
}

func (s *server) hostDBScansHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}

	var from, to time.Time
	var pk types.PublicKey
	if jc.DecodeForm("from", &from) != nil ||
		jc.DecodeForm("to", &to) != nil ||
		jc.DecodeForm("host", &pk) != nil {
		return
	}
	scans, err := s.hdb.Scans(network, pk, from, to)
	if jc.Check("couldn't get scan history", err) != nil {
		return
	}
	jc.Encode(scans)
}

func (s *server) hostDBScanHistoryHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}

	var from, to time.Time
	if jc.DecodeForm("from", &from) != nil || jc.DecodeForm("to", &to) != nil {
		return
	}
	history, err := s.hdb.ScanHistory(network, from, to)
	if jc.Check("couldn't get scan history", err) != nil {
		return
	}
	jc.Encode(history)
}

func (s *server) hostDBBenchmarksHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}

	var from, to time.Time
	var pk types.PublicKey
	if jc.DecodeForm("from", &from) != nil ||
		jc.DecodeForm("to", &to) != nil ||
		jc.DecodeForm("host", &pk) != nil {
		return
	}
	benchmarks, err := s.hdb.Benchmarks(network, pk, from, to)
	if jc.Check("couldn't get benchmark history", err) != nil {
		return
	}
	jc.Encode(benchmarks)
}

func (s *server) hostDBBenchmarkHistoryHandler(jc jape.Context) {
	var network string
	if jc.DecodeForm("network", &network) != nil {
		return
	}
	network = strings.ToLower(network)
	if network != "" && network != "mainnet" && network != "zen" {
		jc.Error(errors.New("wrong network parameter"), http.StatusBadRequest)
		return
	}

	var from, to time.Time
	if jc.DecodeForm("from", &from) != nil || jc.DecodeForm("to", &to) != nil {
		return
	}
	history, err := s.hdb.BenchmarkHistory(network, from, to)
	if jc.Check("couldn't get benchmark history", err) != nil {
		return
	}
	jc.Encode(history)
}

// NewServer returns an HTTP handler that serves the hsd API.
func NewServer(cm *chain.Manager, cmZen *chain.Manager, s *syncer.Syncer, sZen *syncer.Syncer, w *walletutil.Wallet, hdb *hostdb.HostDB) http.Handler {
	srv := server{
		cm:    cm,
		cmZen: cmZen,
		s:     s,
		sZen:  sZen,
		w:     w,
		hdb:   hdb,
	}
	return jape.Mux(map[string]jape.Handler{
		"GET /consensus/network":  srv.consensusNetworkHandler,
		"GET /consensus/tip":      srv.consensusTipHandler,
		"GET /consensus/tipstate": srv.consensusTipStateHandler,

		"GET  /syncer/peers": srv.syncerPeersHandler,

		"GET  /txpool/transactions": srv.txpoolTransactionsHandler,
		"GET  /txpool/fee":          srv.txpoolFeeHandler,

		"GET    /wallet/address": srv.walletAddressHandler,
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

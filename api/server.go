package api

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/internal/build"
	"go.sia.tech/coreutils/syncer"
	"go.sia.tech/jape"
)

type server struct {
	nodes hostdb.NodeStore
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

func checkNetwork(jc jape.Context, network *string) error {
	if err := jc.DecodeForm("network", network); err != nil {
		fmt.Println(err)
		return err
	}

	*network = strings.ToLower(*network)
	if *network == "" {
		*network = "mainnet"
	}

	if *network != "mainnet" && *network != "zen" {
		err := errors.New("wrong network parameter")
		jc.Error(err, http.StatusBadRequest)
		return err
	}

	return nil
}

func (s *server) nodeStatusHandler(jc jape.Context) {
	balance, err := s.nodes.Wallet("mainnet").Balance()
	if jc.Check("couldn't fetch Mainnet balance", err) != nil {
		return
	}

	balanceZen, err := s.nodes.Wallet("zen").Balance()
	if jc.Check("couldn't fetch Zen balance", err) != nil {
		return
	}

	height := s.nodes.ChainManager("mainnet").TipState().Index.Height
	heightZen := s.nodes.ChainManager("zen").TipState().Index.Height

	jc.Encode(NodeStatusResponse{
		Version:    build.NodeVersion,
		Height:     height,
		HeightZen:  heightZen,
		Balance:    balance,
		BalanceZen: balanceZen,
	})
}

func (s *server) consensusNetworkHandler(jc jape.Context) {
	var network string
	if checkNetwork(jc, &network) != nil {
		return
	}

	jc.Encode(*s.nodes.ChainManager(network).TipState().Network)
}

func (s *server) consensusTipHandler(jc jape.Context) {
	var network string
	if checkNetwork(jc, &network) != nil {
		return
	}

	state := s.nodes.ChainManager(network).TipState()
	synced := isSynced(s.nodes.Syncer(network)) && time.Since(state.PrevTimestamps[0]) < 24*time.Hour

	jc.Encode(ConsensusTipResponse{
		Height:  state.Index.Height,
		BlockID: state.Index.ID,
		Synced:  synced,
	})
}

func (s *server) consensusTipStateHandler(jc jape.Context) {
	var network string
	if checkNetwork(jc, &network) != nil {
		return
	}

	jc.Encode(s.nodes.ChainManager(network).TipState())
}

func (s *server) syncerPeersHandler(jc jape.Context) {
	var network string
	if checkNetwork(jc, &network) != nil {
		return
	}

	var peers []GatewayPeer
	ps := s.nodes.Syncer(network).Peers()

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
	if checkNetwork(jc, &network) != nil {
		return
	}

	jc.Encode(TxpoolTransactionsResponse{
		Transactions:   s.nodes.ChainManager(network).PoolTransactions(),
		V2Transactions: s.nodes.ChainManager(network).V2PoolTransactions(),
	})
}

func (s *server) txpoolFeeHandler(jc jape.Context) {
	var network string
	if checkNetwork(jc, &network) != nil {
		return
	}

	jc.Encode(s.nodes.ChainManager(network).RecommendedFee())
}

func (s *server) walletAddressHandler(jc jape.Context) {
	var network string
	if checkNetwork(jc, &network) != nil {
		return
	}

	jc.Encode(s.nodes.Wallet(network).Address())
}

func (s *server) walletBalanceHandler(jc jape.Context) {
	var network string
	if checkNetwork(jc, &network) != nil {
		return
	}

	balance, err := s.nodes.Wallet(network).Balance()
	if jc.Check("couldn't fetch balance", err) != nil {
		return
	}

	jc.Encode(balance)
}

func (s *server) walletEventsHandler(jc jape.Context) {
	var network string
	if checkNetwork(jc, &network) != nil {
		return
	}

	events, err := s.nodes.Wallet(network).UnconfirmedEvents()
	if jc.Check("couldn't load events", err) != nil {
		return
	}

	jc.Encode(events)
}

func (s *server) walletOutputsHandler(jc jape.Context) {
	var network string
	if checkNetwork(jc, &network) != nil {
		return
	}

	scos, err := s.nodes.Wallet(network).UnspentSiacoinElements()
	if jc.Check("couldn't load outputs", err) != nil {
		return
	}

	jc.Encode(scos)
}

func (s *server) hostDBUpdatesHandler(jc jape.Context) {
	updates, err := s.nodes.HostDB().RecentUpdates()
	if jc.Check("couldn't receive HostDB updates", err) != nil {
		return
	}
	jc.Encode(updates)
}

func (s *server) hostDBUpdatesConfirmHandler(jc jape.Context) {
	var id string
	if jc.DecodeForm("id", &id) != nil {
		return
	}

	updateID, err := hex.DecodeString(id)
	if jc.Check("wrong update ID provided", err) != nil {
		return
	}

	jc.Check("couldn't finalize updates", s.nodes.HostDB().FinalizeUpdates(hostdb.UpdateID(updateID)))
}

// NewServer returns an HTTP handler that serves the hsd API.
func NewServer(nodes hostdb.NodeStore) http.Handler {
	srv := server{nodes}
	return jape.Mux(map[string]jape.Handler{
		"GET /node/status": srv.nodeStatusHandler,

		"GET /consensus/network":  srv.consensusNetworkHandler,
		"GET /consensus/tip":      srv.consensusTipHandler,
		"GET /consensus/tipstate": srv.consensusTipStateHandler,

		"GET  /syncer/peers": srv.syncerPeersHandler,

		"GET  /txpool/transactions": srv.txpoolTransactionsHandler,
		"GET  /txpool/fee":          srv.txpoolFeeHandler,

		"GET    /wallet/address": srv.walletAddressHandler,
		"GET    /wallet/balance": srv.walletBalanceHandler,
		"GET    /wallet/events":  srv.walletEventsHandler,
		"GET    /wallet/outputs": srv.walletOutputsHandler,

		"GET    /hostdb/updates":         srv.hostDBUpdatesHandler,
		"GET    /hostdb/updates/confirm": srv.hostDBUpdatesConfirmHandler,
	})
}

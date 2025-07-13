package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	client "github.com/mike76-dev/hostscore/api"
	"github.com/mike76-dev/hostscore/external"
	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/internal/build"
	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	"go.uber.org/zap"
)

var (
	lowBalanceThreshold  = types.Siacoins(200)
	zeroBalanceThreshold = types.Siacoins(10)
)

type hostResponse struct {
	Host portalHost `json:"host"`
}

type hostsResponse struct {
	Hosts []portalHost `json:"hosts"`
	More  bool         `json:"more"`
	Total int          `json:"total"`
}

type keysResponse struct {
	Keys []types.PublicKey `json:"keys"`
}

type hostCount struct {
	Total  int `json:"total"`
	Online int `json:"online"`
}

type networkHostsResponse struct {
	Hosts hostCount `json:"hosts"`
}

type scansResponse struct {
	Scans []scanHistory `json:"scans"`
}

type benchmarksResponse struct {
	Benchmarks []hostdb.BenchmarkHistoryEntry `json:"benchmarks"`
}

type networkStatus struct {
	Height  uint64 `json:"height"`
	Balance string `json:"balance"`
}

type nodeStatus struct {
	Online   bool                     `json:"online"`
	Version  string                   `json:"version"`
	Networks map[string]networkStatus `json:"networks"`
}

type statusResponse struct {
	Nodes   map[string]nodeStatus `json:"nodes"`
	Version string                `json:"version"`
}

type priceChange struct {
	Timestamp        time.Time      `json:"timestamp"`
	RemainingStorage uint64         `json:"remainingStorage"`
	TotalStorage     uint64         `json:"totalStorage"`
	Collateral       types.Currency `json:"collateral"`
	StoragePrice     types.Currency `json:"storagePrice"`
	UploadPrice      types.Currency `json:"uploadPrice"`
	DownloadPrice    types.Currency `json:"downloadPrice"`
}

type priceChangeResponse struct {
	PriceChanges []priceChange `json:"changes"`
}

type averagesResponse struct {
	Averages map[string]networkAverages `json:"averages"`
}

type countriesResponse struct {
	Countries []string `json:"countries"`
}

type scoreBreakdown struct {
	PricesScore       float64 `json:"prices"`
	StorageScore      float64 `json:"storage"`
	CollateralScore   float64 `json:"collateral"`
	InteractionsScore float64 `json:"interactions"`
	UptimeScore       float64 `json:"uptime"`
	AgeScore          float64 `json:"age"`
	VersionScore      float64 `json:"version"`
	LatencyScore      float64 `json:"latency"`
	BenchmarksScore   float64 `json:"benchmarks"`
	ContractsScore    float64 `json:"contracts"`
	TotalScore        float64 `json:"total"`
}

type portalScan struct {
	Timestamp time.Time     `json:"timestamp"`
	Success   bool          `json:"success"`
	Latency   time.Duration `json:"latency"`
	Error     string        `json:"error"`
}

type scanHistory struct {
	Timestamp time.Time       `json:"timestamp"`
	Success   bool            `json:"success"`
	Latency   time.Duration   `json:"latency"`
	Error     string          `json:"error"`
	PublicKey types.PublicKey `json:"publicKey"`
	Network   string          `json:"network"`
	Node      string          `json:"node"`
}

type nodeInteractions struct {
	Uptime           time.Duration          `json:"uptime"`
	Downtime         time.Duration          `json:"downtime"`
	ScanHistory      []portalScan           `json:"scanHistory"`
	BenchmarkHistory []hostdb.HostBenchmark `json:"benchmarkHistory"`
	LastSeen         time.Time              `json:"lastSeen"`
	ActiveHosts      int                    `json:"activeHosts"`
	Score            scoreBreakdown         `json:"score"`
	hostdb.HostInteractions
}

type portalHost struct {
	ID              int                         `json:"id"`
	Rank            int                         `json:"rank"`
	PublicKey       types.PublicKey             `json:"publicKey"`
	FirstSeen       time.Time                   `json:"firstSeen"`
	KnownSince      uint64                      `json:"knownSince"`
	NetAddress      string                      `json:"netaddress"`
	Blocked         bool                        `json:"blocked"`
	V2              bool                        `json:"v2"`
	Interactions    map[string]nodeInteractions `json:"interactions"`
	IPNets          []string                    `json:"ipNets"`
	LastIPChange    time.Time                   `json:"lastIPChange"`
	Score           scoreBreakdown              `json:"score"`
	V2Settings      rhpv4.HostSettings          `json:"v2Settings,omitempty"`
	SiamuxAddresses []string                    `json:"siamuxAddresses"`
	external.IPInfo
}

type networkAverages struct {
	StoragePrice     types.Currency `json:"storagePrice"`
	Collateral       types.Currency `json:"collateral"`
	UploadPrice      types.Currency `json:"uploadPrice"`
	DownloadPrice    types.Currency `json:"downloadPrice"`
	ContractDuration uint64         `json:"contractDuration"`
	Available        bool           `json:"available"`
}

type sortType int

const (
	noSort sortType = iota
	sortByID
	sortByRank
	sortByTotalStorage
	sortByUsedStorage
	sortByStoragePrice
	sortByUploadPrice
	sortByDownloadPrice
)

var (
	networks = []string{"mainnet", "zen"}
)

type portalAPI struct {
	router   httprouter.Router
	store    *jsonStore
	db       *sql.DB
	token    string
	log      *zap.Logger
	clients  map[string]*client.Client
	mu       sync.RWMutex
	cache    *responseCache
	hosts    map[string]map[types.PublicKey]*portalHost
	stopChan chan struct{}
	averages map[string]map[string]networkAverages
	nodes    map[string]nodeStatus
	rl       *ratelimiter
}

func newAPI(s *jsonStore, db *sql.DB, token string, logger *zap.Logger, cache *responseCache) (*portalAPI, error) {
	api := &portalAPI{
		store:    s,
		db:       db,
		token:    token,
		log:      logger,
		clients:  make(map[string]*client.Client),
		cache:    cache,
		hosts:    make(map[string]map[types.PublicKey]*portalHost),
		stopChan: make(chan struct{}),
		averages: make(map[string]map[string]networkAverages),
		nodes:    make(map[string]nodeStatus),
	}

	for _, network := range networks {
		api.hosts[network] = make(map[types.PublicKey]*portalHost)
	}

	api.rl = newRatelimiter(api.stopChan)

	err := api.load()
	if err != nil {
		return nil, err
	}

	go api.doRequestStatus()
	go api.requestUpdates()
	go api.updateAverages()
	go api.pruneOldScans()

	return api, nil
}

func (api *portalAPI) close() {
	close(api.stopChan)
}

func (api *portalAPI) requestUpdates() {
	timeout := time.Minute
	for {
		select {
		case <-api.stopChan:
			return
		case <-time.After(timeout):
		}

		timeout = time.Minute
		for node, c := range api.clients {
			updates, err := c.Updates()
			if err != nil {
				api.log.Error("failed to request updates", zap.String("node", node), zap.Error(err))
			}
			if err := api.insertUpdates(node, updates); err != nil {
				api.log.Error("failed to insert updates", zap.String("node", node), zap.Error(err))
			}
			if len(updates.Hosts)+len(updates.Scans)+len(updates.Benchmarks) > 500 {
				timeout = 5 * time.Second
			}
		}
	}
}

func (api *portalAPI) requestStatus() {
	nodes := make(map[string]nodeStatus)
	var mu sync.Mutex
	for n, c := range api.clients {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var status client.NodeStatusResponse
		var err error
		done := make(chan struct{})
		go func() {
			status, err = c.NodeStatus()
			close(done)
		}()

		select {
		case <-done:
			if err != nil {
				api.log.Error("couldn't get node status", zap.String("node", n), zap.Error(err))
				mu.Lock()
				nodes[n] = nodeStatus{Online: false}
				mu.Unlock()
			} else {
				mu.Lock()
				nodes[n] = nodeStatus{
					Online:   true,
					Version:  status.Version,
					Networks: make(map[string]networkStatus),
				}
				for _, network := range status.Networks {
					nodes[n].Networks[network.Network] = networkStatus{
						Height:  network.Height,
						Balance: balanceStatus(network.Balance.Confirmed),
					}
				}
				mu.Unlock()
			}
		case <-ctx.Done():
			api.log.Error("NodeStatus call timed out", zap.String("node", n))
			mu.Lock()
			nodes[n] = nodeStatus{Online: false}
			mu.Unlock()
		}
	}
	api.nodes = nodes
}

func (api *portalAPI) doRequestStatus() {
	api.requestStatus()
	for {
		select {
		case <-api.stopChan:
			return
		case <-time.After(5 * time.Minute):
		}
		api.requestStatus()
	}
}

func (api *portalAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// For testing only.
	/*if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "*")
	}
	if r.Method == "OPTIONS" {
		return
	}*/

	api.mu.RLock()
	api.router.ServeHTTP(w, r)
	api.mu.RUnlock()
}

func (api *portalAPI) buildHTTPRoutes() {
	router := httprouter.New()

	router.GET("/hosts", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.hostsHandler(w, req, ps)
	})
	router.GET("/hosts/keys", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.hostsKeysHandler(w, req, ps)
	})
	router.GET("/hosts/host", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.hostsHostHandler(w, req, ps)
	})
	router.GET("/hosts/scans", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.hostsScansHandler(w, req, ps)
	})
	router.GET("/hosts/benchmarks", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.hostsBenchmarksHandler(w, req, ps)
	})
	router.GET("/hosts/changes", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.hostsChangesHandler(w, req, ps)
	})

	router.GET("/network/hosts", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.networkHostsHandler(w, req, ps)
	})
	router.GET("/network/averages", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.networkAveragesHandler(w, req, ps)
	})
	router.GET("/network/countries", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.networkCountriesHandler(w, req, ps)
	})

	router.GET("/service/status", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.serviceStatusHandler(w, req, ps)
	})

	api.mu.Lock()
	api.router = *router
	api.mu.Unlock()
}

func checkNetwork(network string) string {
	network = strings.ToLower(network)
	if network == "" {
		network = "mainnet"
	}

	for _, n := range networks {
		if network == n {
			return network
		}
	}

	return ""
}

func (api *portalAPI) hostsHostHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	network := checkNetwork(req.FormValue("network"))
	if network == "" {
		writeError(w, "wrong network", http.StatusBadRequest)
		return
	}

	h := req.FormValue("host")
	if h == "" {
		writeError(w, "host not provided", http.StatusBadRequest)
		return
	}

	var pk types.PublicKey
	if !strings.HasPrefix(h, "ed25519:") {
		h = "ed25519:" + h
	}
	err := pk.UnmarshalText([]byte(h))
	if err != nil {
		writeError(w, "invalid public key", http.StatusBadRequest)
		return
	}

	host, ok := api.cache.getHost(network, pk)
	if !ok {
		host, err = api.getHost(network, pk)
		if err != nil && errors.Is(err, errHostNotFound) {
			writeError(w, "host not found", http.StatusBadRequest)
			return
		}
		if err != nil {
			api.log.Error("couldn't get host", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, hostResponse{Host: host})
}

func (api *portalAPI) hostsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	network := checkNetwork(req.FormValue("network"))
	if network == "" {
		writeError(w, "wrong network", http.StatusBadRequest)
		return
	}

	query := strings.ToLower(req.FormValue("query"))
	country := strings.ToUpper(req.FormValue("country"))
	allHosts := strings.ToLower(req.FormValue("all"))
	var all bool
	if allHosts == "true" {
		all = true
	}

	offset, limit := int64(0), int64(-1)
	var err error
	off := req.FormValue("offset")
	if off == "" {
		writeError(w, "offset not provided", http.StatusBadRequest)
		return
	}
	offset, err = strconv.ParseInt(off, 10, 64)
	if err != nil {
		writeError(w, "invalid offset", http.StatusBadRequest)
		return
	}

	lim := req.FormValue("limit")
	if lim == "" {
		writeError(w, "limit not provided", http.StatusBadRequest)
		return
	}
	limit, err = strconv.ParseInt(lim, 10, 64)
	if err != nil {
		writeError(w, "invalid limit", http.StatusBadRequest)
		return
	}

	var sortBy sortType
	sb := strings.ToLower(req.FormValue("sort"))
	if sb == "" {
		sb = "rank"
	}
	switch sb {
	case "id":
		sortBy = sortByID
	case "rank":
		sortBy = sortByRank
	case "total":
		sortBy = sortByTotalStorage
	case "used":
		sortBy = sortByUsedStorage
	case "storage":
		sortBy = sortByStoragePrice
	case "upload":
		sortBy = sortByUploadPrice
	case "download":
		sortBy = sortByDownloadPrice
	default:
		writeError(w, "invalid sorting type", http.StatusBadRequest)
		return
	}

	order := strings.ToLower(req.FormValue("order"))
	if order == "" {
		order = "asc"
	}
	asc := true
	if order != "asc" && order != "desc" {
		writeError(w, "invalid sorting order", http.StatusBadRequest)
		return
	}
	if order == "desc" {
		asc = false
	}

	hosts, more, total, ok := api.cache.getHosts(network, all, int(offset), int(limit), query, country, sortBy, asc)
	if !ok {
		hosts, more, total, err = api.getHosts(network, all, int(offset), int(limit), query, country, sortBy, asc)
		if err != nil {
			api.log.Error("couldn't get hosts", zap.Error(err))
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}
		api.cache.putHosts(network, all, int(offset), int(limit), query, country, sortBy, asc, hosts, more, total)
	}

	// Prefetch the next bunch of hosts.
	if more {
		go func() {
			_, _, _, ok := api.cache.getHosts(network, all, int(offset+limit), int(limit), query, country, sortBy, asc)
			if !ok {
				h, m, t, err := api.getHosts(network, all, int(offset+limit), int(limit), query, country, sortBy, asc)
				if err != nil {
					return
				}
				api.cache.putHosts(network, all, int(offset+limit), int(limit), query, country, sortBy, asc, h, m, t)
			}
		}()
	}

	writeJSON(w, hostsResponse{
		Hosts: hosts,
		More:  more,
		Total: total,
	})
}

func (api *portalAPI) hostsKeysHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	err := req.ParseForm()
	if err != nil {
		writeError(w, "unable to parse request", http.StatusBadRequest)
		return
	}

	network := checkNetwork(req.FormValue("network"))
	if network == "" {
		writeError(w, "wrong network", http.StatusBadRequest)
		return
	}

	node := strings.ToLower(req.FormValue("node"))
	if node == "" {
		node = "global"
	}
	_, ok := api.clients[node]
	if node != "global" && !ok {
		writeError(w, "wrong node", http.StatusBadRequest)
		return
	}

	msp := req.FormValue("maxStoragePrice")
	mup := req.FormValue("maxUploadPrice")
	mdp := req.FormValue("maxDownloadPrice")
	mcp := req.FormValue("maxContractPrice")
	var maxStoragePrice, maxUploadPrice, maxDownloadPrice, maxContractPrice types.Currency
	if msp == "" {
		maxStoragePrice = types.MaxCurrency
	} else {
		maxStoragePrice, err = types.ParseCurrency(msp)
		if err != nil {
			writeError(w, "invalid max storage price", http.StatusBadRequest)
			return
		}
	}
	if mup == "" {
		maxUploadPrice = types.MaxCurrency
	} else {
		maxUploadPrice, err = types.ParseCurrency(mup)
		if err != nil {
			writeError(w, "invalid max upload price", http.StatusBadRequest)
			return
		}
	}
	if mdp == "" {
		maxDownloadPrice = types.MaxCurrency
	} else {
		maxDownloadPrice, err = types.ParseCurrency(mdp)
		if err != nil {
			writeError(w, "invalid max download price", http.StatusBadRequest)
			return
		}
	}
	if mcp == "" {
		maxContractPrice = types.MaxCurrency
	} else {
		maxContractPrice, err = types.ParseCurrency(mcp)
		if err != nil {
			writeError(w, "invalid max contract price", http.StatusBadRequest)
			return
		}
	}

	md := req.FormValue("minContractDuration")
	var minDuration int64
	if md != "" {
		minDuration, err = strconv.ParseInt(md, 10, 64)
		if err != nil {
			writeError(w, "invalid min contract duration", http.StatusBadRequest)
			return
		}
	}

	ms := req.FormValue("minAvailableStorage")
	var minStorage int64
	if ms != "" {
		minStorage, err = strconv.ParseInt(ms, 10, 64)
		if err != nil {
			writeError(w, "invalid min available storage", http.StatusBadRequest)
			return
		}
	}

	ml := req.FormValue("maxLatency")
	mus := req.FormValue("minUploadSpeed")
	mds := req.FormValue("minDownloadSpeed")
	var maxLatency, minUploadSpeed, minDownloadSpeed int64
	if ml != "" {
		maxLatency, err = strconv.ParseInt(ml, 10, 64)
		if err != nil {
			writeError(w, "invalid max latency", http.StatusBadRequest)
			return
		}
	}
	if mus != "" {
		minUploadSpeed, err = strconv.ParseInt(mus, 10, 64)
		if err != nil {
			writeError(w, "invalid min upload speed", http.StatusBadRequest)
			return
		}
	}
	if mds != "" {
		minDownloadSpeed, err = strconv.ParseInt(mds, 10, 64)
		if err != nil {
			writeError(w, "invalid min download speed", http.StatusBadRequest)
			return
		}
	}

	countries := req.Form["country"]
	limit := int64(-1)
	lim := req.FormValue("limit")
	if lim != "" {
		limit, err = strconv.ParseInt(lim, 10, 64)
		if err != nil {
			writeError(w, "invalid limit", http.StatusBadRequest)
			return
		}
	}

	keys, err := api.getHostKeys(
		network,
		node,
		maxStoragePrice,
		maxUploadPrice,
		maxDownloadPrice,
		maxContractPrice,
		uint64(minDuration),
		uint64(minStorage),
		time.Duration(maxLatency),
		float64(minUploadSpeed),
		float64(minDownloadSpeed),
		countries,
		int(limit),
	)
	if err != nil {
		api.log.Error("couldn't get host keys", zap.Error(err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, keysResponse{Keys: keys})
}

func (api *portalAPI) hostsScansHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	network := checkNetwork(req.FormValue("network"))
	if network == "" {
		writeError(w, "wrong network", http.StatusBadRequest)
		return
	}

	node := strings.ToLower(req.FormValue("node"))
	if node == "" {
		node = "global"
	}
	_, ok := api.clients[node]
	if node != "global" && !ok {
		writeError(w, "wrong node", http.StatusBadRequest)
		return
	}

	host := req.FormValue("host")
	if host == "" {
		writeError(w, "host not provided", http.StatusBadRequest)
		return
	}

	var pk types.PublicKey
	if !strings.HasPrefix(host, "ed25519:") {
		host = "ed25519:" + host
	}
	err := pk.UnmarshalText([]byte(host))
	if err != nil {
		writeError(w, "invalid public key", http.StatusBadRequest)
		return
	}

	var from, to time.Time
	to = time.Now()
	f := req.FormValue("from")
	if f != "" {
		from, err = time.Parse(time.RFC3339, f)
		if err != nil {
			writeError(w, "invalid timestamp", http.StatusBadRequest)
			return
		}
	}
	t := req.FormValue("to")
	if t != "" {
		to, err = time.Parse(time.RFC3339, t)
		if err != nil {
			writeError(w, "invalid timestamp", http.StatusBadRequest)
			return
		}
	}

	all := true
	allScans := strings.ToLower(req.FormValue("all"))
	if allScans == "false" {
		all = false
	}

	limit := int64(-1)
	lim := req.FormValue("limit")
	if lim != "" {
		limit, err = strconv.ParseInt(lim, 10, 64)
		if err != nil {
			writeError(w, "invalid limit", http.StatusBadRequest)
			return
		}
	}

	scans, err := api.getScans(network, node, pk, all, from, to, limit)
	if err != nil && errors.Is(err, errHostNotFound) {
		writeError(w, "host not found", http.StatusBadRequest)
		return
	}
	if err != nil {
		api.log.Error("couldn't get scan history", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, scansResponse{Scans: scans})
}

func (api *portalAPI) hostsBenchmarksHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	network := checkNetwork(req.FormValue("network"))
	if network == "" {
		writeError(w, "wrong network", http.StatusBadRequest)
		return
	}

	node := strings.ToLower(req.FormValue("node"))
	if node == "" {
		node = "global"
	}
	_, ok := api.clients[node]
	if node != "global" && !ok {
		writeError(w, "wrong node", http.StatusBadRequest)
		return
	}

	host := req.FormValue("host")
	if host == "" {
		writeError(w, "host not provided", http.StatusBadRequest)
		return
	}

	var pk types.PublicKey
	if !strings.HasPrefix(host, "ed25519:") {
		host = "ed25519:" + host
	}
	err := pk.UnmarshalText([]byte(host))
	if err != nil {
		writeError(w, "invalid public key", http.StatusBadRequest)
		return
	}

	var from, to time.Time
	to = time.Now()
	f := req.FormValue("from")
	if f != "" {
		from, err = time.Parse(time.RFC3339, f)
		if err != nil {
			writeError(w, "invalid timestamp", http.StatusBadRequest)
			return
		}
	}
	t := req.FormValue("to")
	if t != "" {
		to, err = time.Parse(time.RFC3339, t)
		if err != nil {
			writeError(w, "invalid timestamp", http.StatusBadRequest)
			return
		}
	}

	all := true
	allBenchmarks := strings.ToLower(req.FormValue("all"))
	if allBenchmarks == "false" {
		all = false
	}

	limit := int64(-1)
	lim := req.FormValue("limit")
	if lim != "" {
		limit, err = strconv.ParseInt(lim, 10, 64)
		if err != nil {
			writeError(w, "invalid limit", http.StatusBadRequest)
			return
		}
	}

	benchmarks, err := api.getBenchmarks(network, node, pk, all, from, to, limit)
	if err != nil && errors.Is(err, errHostNotFound) {
		writeError(w, "host not found", http.StatusBadRequest)
		return
	}
	if err != nil {
		api.log.Error("couldn't get benchmark history", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, benchmarksResponse{Benchmarks: benchmarks})
}

func balanceStatus(balance types.Currency) string {
	if balance.Cmp(zeroBalanceThreshold) < 0 {
		return "empty"
	}
	if balance.Cmp(lowBalanceThreshold) < 0 {
		return "low"
	}
	return "ok"
}

func (api *portalAPI) serviceStatusHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	writeJSON(w, statusResponse{
		Version: build.ClientVersion,
		Nodes:   api.nodes,
	})
}

func (api *portalAPI) networkHostsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	network := checkNetwork(req.FormValue("network"))
	if network == "" {
		writeError(w, "wrong network", http.StatusBadRequest)
		return
	}

	var hosts hostCount
	api.mu.RLock()
	hosts.Total = len(api.hosts[network])
	for _, host := range api.hosts[network] {
		if isOnline(*host) {
			hosts.Online++
		}
	}
	api.mu.RUnlock()

	writeJSON(w, networkHostsResponse{Hosts: hosts})
}

func (api *portalAPI) hostsChangesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	network := checkNetwork(req.FormValue("network"))
	if network == "" {
		writeError(w, "wrong network", http.StatusBadRequest)
		return
	}

	host := req.FormValue("host")
	if host == "" {
		writeError(w, "host not provided", http.StatusBadRequest)
		return
	}

	var pk types.PublicKey
	if !strings.HasPrefix(host, "ed25519:") {
		host = "ed25519:" + host
	}
	err := pk.UnmarshalText([]byte(host))
	if err != nil {
		writeError(w, "invalid public key", http.StatusBadRequest)
		return
	}

	var from, to time.Time
	f := req.FormValue("from")
	if f != "" {
		from, err = time.Parse(time.RFC3339, f)
		if err != nil {
			writeError(w, "invalid timestamp", http.StatusBadRequest)
			return
		}
	}
	t := req.FormValue("to")
	if t != "" {
		to, err = time.Parse(time.RFC3339, t)
		if err != nil {
			writeError(w, "invalid timestamp", http.StatusBadRequest)
			return
		}
	}

	limit := int64(-1)
	lim := req.FormValue("limit")
	if lim != "" {
		limit, err = strconv.ParseInt(lim, 10, 64)
		if err != nil {
			writeError(w, "invalid limit", http.StatusBadRequest)
			return
		}
	}

	pcs, err := api.getPriceChanges(network, pk, from, to, limit)
	if err != nil && errors.Is(err, errHostNotFound) {
		writeError(w, "host not found", http.StatusBadRequest)
		return
	}
	if err != nil {
		api.log.Error("couldn't get price changes", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, priceChangeResponse{PriceChanges: pcs})
}

func (api *portalAPI) networkAveragesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	network := checkNetwork(req.FormValue("network"))
	if network == "" {
		writeError(w, "wrong network", http.StatusBadRequest)
		return
	}

	writeJSON(w, averagesResponse{Averages: api.averages[network]})
}

func (api *portalAPI) networkCountriesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if api.rl.limitExceeded(getRemoteHost(req)) {
		writeError(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	network := checkNetwork(req.FormValue("network"))
	if network == "" {
		writeError(w, "wrong network", http.StatusBadRequest)
		return
	}

	allHosts := strings.ToLower(req.FormValue("all"))
	var all bool
	if allHosts == "" || allHosts == "true" {
		all = true
	} else if allHosts == "false" {
		all = false
	} else {
		writeError(w, "wrong all parameter", http.StatusBadRequest)
		return
	}

	countries, err := api.getCountries(network, all)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, countriesResponse{Countries: countries})
}

func writeJSON(w http.ResponseWriter, obj interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err := json.NewEncoder(w).Encode(obj)
	if _, isJsonErr := err.(*json.SyntaxError); isJsonErr {
		log.Println("ERROR: failed to encode API response:", err)
	}
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	err := json.NewEncoder(w).Encode(message)
	if _, isJsonErr := err.(*json.SyntaxError); isJsonErr {
		log.Println("ERROR: failed to encode API error response:", err)
	}
}

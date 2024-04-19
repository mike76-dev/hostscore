package main

import (
	"context"
	"database/sql"
	"encoding/json"
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
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.uber.org/zap"
)

var (
	lowBalanceThreshold  = types.Siacoins(200)
	zeroBalanceThreshold = types.Siacoins(10)
)

type APIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type hostResponse struct {
	APIResponse
	Host portalHost `json:"host"`
}

type hostsResponse struct {
	APIResponse
	Hosts []portalHost `json:"hosts"`
	More  bool         `json:"more"`
	Total int          `json:"total"`
}

type onlineHostsResponse struct {
	APIResponse
	OnlineHosts int `json:"onlineHosts"`
}

type scansResponse struct {
	APIResponse
	PublicKey types.PublicKey `json:"publicKey"`
	Scans     []scanHistory   `json:"scans"`
}

type benchmarksResponse struct {
	APIResponse
	PublicKey  types.PublicKey           `json:"publicKey"`
	Benchmarks []hostdb.BenchmarkHistory `json:"benchmarks"`
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
	APIResponse
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
	APIResponse
	PublicKey    types.PublicKey `json:"publicKey"`
	PriceChanges []priceChange   `json:"priceChanges"`
}

type averagesResponse struct {
	APIResponse
	Averages networkAverages `json:"averages"`
}

type countriesResponse struct {
	APIResponse
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
	ID           int                         `json:"id"`
	Rank         int                         `json:"rank"`
	PublicKey    types.PublicKey             `json:"publicKey"`
	FirstSeen    time.Time                   `json:"firstSeen"`
	KnownSince   uint64                      `json:"knownSince"`
	NetAddress   string                      `json:"netaddress"`
	Blocked      bool                        `json:"blocked"`
	Interactions map[string]nodeInteractions `json:"interactions"`
	IPNets       []string                    `json:"ipNets"`
	LastIPChange time.Time                   `json:"lastIPChange"`
	Score        scoreBreakdown              `json:"score"`
	Settings     rhpv2.HostSettings          `json:"settings"`
	PriceTable   rhpv3.HostPriceTable        `json:"priceTable"`
	external.IPInfo
}

type averagePrices struct {
	StoragePrice     types.Currency `json:"storagePrice"`
	Collateral       types.Currency `json:"collateral"`
	UploadPrice      types.Currency `json:"uploadPrice"`
	DownloadPrice    types.Currency `json:"downloadPrice"`
	ContractDuration uint64         `json:"contractDuration"`
	OK               bool           `json:"ok"`
}

type networkAverages struct {
	Tier1 averagePrices `json:"tier1"`
	Tier2 averagePrices `json:"tier2"`
	Tier3 averagePrices `json:"tier3"`
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

type portalAPI struct {
	router      httprouter.Router
	store       *jsonStore
	db          *sql.DB
	token       string
	log         *zap.Logger
	clients     map[string]*client.Client
	mu          sync.RWMutex
	cache       *responseCache
	hosts       map[types.PublicKey]*portalHost
	hostsZen    map[types.PublicKey]*portalHost
	stopChan    chan struct{}
	averages    networkAverages
	averagesZen networkAverages
	nodes       map[string]nodeStatus
}

func newAPI(s *jsonStore, db *sql.DB, token string, logger *zap.Logger, cache *responseCache) (*portalAPI, error) {
	api := &portalAPI{
		store:    s,
		db:       db,
		token:    token,
		log:      logger,
		clients:  make(map[string]*client.Client),
		cache:    cache,
		hosts:    map[types.PublicKey]*portalHost{},
		hostsZen: map[types.PublicKey]*portalHost{},
		stopChan: make(chan struct{}),
		nodes:    make(map[string]nodeStatus),
	}

	err := api.load()
	if err != nil {
		return nil, err
	}

	go api.doRequestStatus()
	go api.requestUpdates()
	go api.updateAverages()

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
				nodes[n].Networks["mainnet"] = networkStatus{
					Height:  status.Height,
					Balance: balanceStatus(status.Balance.Siacoins),
				}
				nodes[n].Networks["zen"] = networkStatus{
					Height:  status.HeightZen,
					Balance: balanceStatus(status.BalanceZen.Siacoins),
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

	router.GET("/host", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.hostHandler(w, req, ps)
	})
	router.GET("/hosts", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.hostsHandler(w, req, ps)
	})
	router.GET("/hosts/online", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.onlineHostsHandler(w, req, ps)
	})
	router.GET("/scans", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.scansHandler(w, req, ps)
	})
	router.GET("/benchmarks", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.benchmarksHandler(w, req, ps)
	})
	router.GET("/changes", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.changesHandler(w, req, ps)
	})
	router.GET("/service/status", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.serviceStatusHandler(w, req, ps)
	})
	router.GET("/averages", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.averagesHandler(w, req, ps)
	})
	router.GET("/countries", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.countriesHandler(w, req, ps)
	})

	api.mu.Lock()
	api.router = *router
	api.mu.Unlock()
}

func (api *portalAPI) hostHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	network := strings.ToLower(req.FormValue("network"))
	if network == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "network not provided",
		})
		return
	}
	h := req.FormValue("host")
	if h == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "host not provided",
		})
		return
	}
	var pk types.PublicKey
	err := pk.UnmarshalText([]byte(h))
	if err != nil {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "invalid public key",
		})
		return
	}
	host, ok := api.cache.getHost(network, pk)
	if !ok {
		host, err = api.getHost(network, pk)
		if err != nil {
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "host not found",
			})
			return
		}
	}
	writeJSON(w, hostResponse{
		APIResponse: APIResponse{Status: "ok"},
		Host:        host,
	})
}

func (api *portalAPI) hostsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	network := strings.ToLower(req.FormValue("network"))
	if network == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "network not provided",
		})
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
	if off != "" {
		offset, err = strconv.ParseInt(off, 10, 64)
		if err != nil {
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "invalid offset",
			})
			return
		}
	}
	lim := req.FormValue("limit")
	if lim != "" {
		limit, err = strconv.ParseInt(lim, 10, 64)
		if err != nil {
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "invalid limit",
			})
			return
		}
	}
	var sortBy sortType
	sb := req.FormValue("sort")
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
		sortBy = sortByID
	}
	order := strings.ToLower(req.FormValue("order"))
	asc := true
	if order == "desc" {
		asc = false
	}

	hosts, more, total, ok := api.cache.getHosts(network, all, int(offset), int(limit), query, country, sortBy, asc)
	if !ok {
		hosts, more, total, err = api.getHosts(network, all, int(offset), int(limit), query, country, sortBy, asc)
		if err != nil {
			api.log.Error("couldn't get hosts", zap.Error(err))
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "internal error",
			})
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
		APIResponse: APIResponse{Status: "ok"},
		Hosts:       hosts,
		More:        more,
		Total:       total,
	})
}

func (api *portalAPI) scansHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	network := strings.ToLower(req.FormValue("network"))
	if network == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "network not provided",
		})
		return
	}
	host := req.FormValue("host")
	if host == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "host not provided",
		})
		return
	}
	var pk types.PublicKey
	err := pk.UnmarshalText([]byte(host))
	if err != nil {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "invalid public key",
		})
		return
	}
	var from, to time.Time
	to = time.Now()
	f := req.FormValue("from")
	if f != "" {
		from, err = time.Parse(time.RFC3339, f)
		if err != nil {
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "invalid timestamp",
			})
			return
		}
	}
	t := req.FormValue("to")
	if t != "" {
		to, err = time.Parse(time.RFC3339, t)
		if err != nil {
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "invalid timestamp",
			})
			return
		}
	}
	var number int64
	num := req.FormValue("number")
	if num != "" {
		number, err = strconv.ParseInt(num, 10, 64)
		if err != nil {
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "invalid number",
			})
			return
		}
	}
	var successful bool
	success := strings.ToLower(req.FormValue("success"))
	if success == "true" {
		successful = true
	}
	scans, ok := api.cache.getScans(network, pk, from, to, int(number), successful)
	if !ok {
		s, err := api.getScans(network, pk, from, to, int(number), successful)
		if err != nil {
			api.log.Error("couldn't get scan history", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "internal error",
			})
			return
		}
		scans = s
		api.cache.putScans(network, pk, from, to, int(number), successful, s)
	}
	writeJSON(w, scansResponse{
		APIResponse: APIResponse{Status: "ok"},
		PublicKey:   pk,
		Scans:       scans,
	})
}

func (api *portalAPI) benchmarksHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	network := strings.ToLower(req.FormValue("network"))
	if network == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "network not provided",
		})
		return
	}
	host := req.FormValue("host")
	if host == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "host not provided",
		})
		return
	}
	var pk types.PublicKey
	err := pk.UnmarshalText([]byte(host))
	if err != nil {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "invalid public key",
		})
		return
	}
	var from, to time.Time
	to = time.Now()
	f := req.FormValue("from")
	if f != "" {
		from, err = time.Parse(time.RFC3339, f)
		if err != nil {
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "invalid timestamp",
			})
			return
		}
	}
	t := req.FormValue("to")
	if t != "" {
		to, err = time.Parse(time.RFC3339, t)
		if err != nil {
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "invalid timestamp",
			})
			return
		}
	}
	var number int64
	num := req.FormValue("number")
	if num != "" {
		number, err = strconv.ParseInt(num, 10, 64)
		if err != nil {
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "invalid number",
			})
			return
		}
	}
	var successful bool
	success := strings.ToLower(req.FormValue("success"))
	if success == "true" {
		successful = true
	}
	benchmarks, ok := api.cache.getBenchmarks(network, pk, from, to, int(number), successful)
	if !ok {
		b, err := api.getBenchmarks(network, pk, from, to, int(number), successful)
		if err != nil {
			api.log.Error("couldn't get benchmark history", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "internal error",
			})
			return
		}
		benchmarks = b
		api.cache.putBenchmarks(network, pk, from, to, int(number), successful, b)
	}
	writeJSON(w, benchmarksResponse{
		APIResponse: APIResponse{Status: "ok"},
		PublicKey:   pk,
		Benchmarks:  benchmarks,
	})
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

func (api *portalAPI) serviceStatusHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	writeJSON(w, statusResponse{
		APIResponse: APIResponse{Status: "ok"},
		Version:     build.ClientVersion,
		Nodes:       api.nodes,
	})
}

func (api *portalAPI) onlineHostsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	network := strings.ToLower(req.FormValue("network"))
	if network == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "network not provided",
		})
		return
	}
	var count int
	api.mu.RLock()
	if network == "mainnet" {
		for _, host := range api.hosts {
			if api.isOnline(*host) {
				count++
			}
		}
	} else if network == "zen" {
		for _, host := range api.hostsZen {
			if api.isOnline(*host) {
				count++
			}
		}
	}
	api.mu.RUnlock()
	writeJSON(w, onlineHostsResponse{
		APIResponse: APIResponse{Status: "ok"},
		OnlineHosts: count,
	})
}

func (api *portalAPI) changesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	network := strings.ToLower(req.FormValue("network"))
	if network == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "network not provided",
		})
		return
	}
	host := req.FormValue("host")
	if host == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "host not provided",
		})
		return
	}
	var pk types.PublicKey
	err := pk.UnmarshalText([]byte(host))
	if err != nil {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "invalid public key",
		})
		return
	}
	pcs, err := api.getPriceChanges(network, pk)
	if err != nil {
		api.log.Error("couldn't get price changes", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "internal error",
		})
		return
	}
	writeJSON(w, priceChangeResponse{
		APIResponse:  APIResponse{Status: "ok"},
		PublicKey:    pk,
		PriceChanges: pcs,
	})
}

func (api *portalAPI) averagesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	network := strings.ToLower(req.FormValue("network"))
	if network == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "network not provided",
		})
		return
	}
	if network != "mainnet" && network != "zen" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "wrong network",
		})
		return
	}
	var averages networkAverages
	if network == "mainnet" {
		averages = api.averages
	} else {
		averages = api.averagesZen
	}
	writeJSON(w, averagesResponse{
		APIResponse: APIResponse{Status: "ok"},
		Averages:    averages,
	})
}

func (api *portalAPI) countriesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	network := strings.ToLower(req.FormValue("network"))
	if network == "" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "network not provided",
		})
		return
	}
	if network != "mainnet" && network != "zen" {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "wrong network",
		})
		return
	}
	countries, err := api.getCountries(network)
	if err != nil {
		writeJSON(w, APIResponse{
			Status:  "error",
			Message: "internal error",
		})
		return
	}
	writeJSON(w, countriesResponse{
		APIResponse: APIResponse{Status: "ok"},
		Countries:   countries,
	})
}

func writeJSON(w http.ResponseWriter, obj interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err := json.NewEncoder(w).Encode(obj)
	if _, isJsonErr := err.(*json.SyntaxError); isJsonErr {
		log.Println("ERROR: failed to encode API response:", err)
	}
}

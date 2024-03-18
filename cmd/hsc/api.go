package main

import (
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
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.uber.org/zap"
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

type scansResponse struct {
	APIResponse
	PublicKey types.PublicKey      `json:"publicKey"`
	Scans     []hostdb.ScanHistory `json:"scans"`
}

type benchmarksResponse struct {
	APIResponse
	PublicKey  types.PublicKey           `json:"publicKey"`
	Benchmarks []hostdb.BenchmarkHistory `json:"benchmarks"`
}

type nodeInteractions struct {
	Uptime        time.Duration        `json:"uptime"`
	Downtime      time.Duration        `json:"downtime"`
	ScanHistory   []hostdb.HostScan    `json:"scanHistory"`
	LastBenchmark hostdb.HostBenchmark `json:"lastBenchmark"`
	LastSeen      time.Time            `json:"lastSeen"`
	ActiveHosts   int                  `json:"activeHosts"`
	hostdb.HostInteractions
}

type portalHost struct {
	ID           int                         `json:"id"`
	PublicKey    types.PublicKey             `json:"publicKey"`
	FirstSeen    time.Time                   `json:"firstSeen"`
	KnownSince   uint64                      `json:"knownSince"`
	NetAddress   string                      `json:"netaddress"`
	Blocked      bool                        `json:"blocked"`
	Interactions map[string]nodeInteractions `json:"interactions"`
	IPNets       []string                    `json:"ipNets"`
	LastIPChange time.Time                   `json:"lastIPChange"`
	Settings     rhpv2.HostSettings          `json:"settings"`
	PriceTable   rhpv3.HostPriceTable        `json:"priceTable"`
	external.IPInfo
}

type portalAPI struct {
	router   httprouter.Router
	store    *jsonStore
	db       *sql.DB
	token    string
	log      *zap.Logger
	clients  map[string]*client.Client
	mu       sync.RWMutex
	cache    *responseCache
	hosts    map[types.PublicKey]*portalHost
	hostsZen map[types.PublicKey]*portalHost
	stopChan chan struct{}
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
	}

	err := api.load()
	if err != nil {
		return nil, err
	}

	go api.requestUpdates()

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
			if len(updates.Hosts)+len(updates.Scans)+len(updates.Benchmarks) > 1000 {
				timeout = 5 * time.Second
			}
		}
	}
}

func (api *portalAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// For testing only.
	if origin := r.Header.Get("Origin"); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "*")
	}
	if r.Method == "OPTIONS" {
		return
	}

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
	router.GET("/scans", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.scansHandler(w, req, ps)
	})
	router.GET("/benchmarks", func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		api.benchmarksHandler(w, req, ps)
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
	hosts, more, total, ok := api.cache.getHosts(network, all, int(offset), int(limit), query)
	if !ok {
		hosts, more, total, err = api.getHosts(network, all, int(offset), int(limit), query)
		if err != nil {
			api.log.Error("couldn't get hosts", zap.Error(err))
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "internal error",
			})
			return
		}
		api.cache.putHosts(network, all, int(offset), int(limit), query, hosts, more, total)
	}

	// Prefetch the scans and the benchmarks.
	go func() {
		for _, h := range hosts {
			go func(h portalHost) {
				_, ok := api.cache.getScans(network, h.PublicKey, time.Now().AddDate(0, 0, -1), time.Now())
				if !ok {
					s, err := api.getScans(network, h.PublicKey, time.Now().AddDate(0, 0, -1), time.Now())
					if err != nil {
						return
					}
					api.cache.putScans(network, h.PublicKey, time.Now().AddDate(0, 0, -1), time.Now(), s)
				}
				_, ok = api.cache.getBenchmarks(network, h.PublicKey, time.Now().AddDate(0, 0, -1), time.Now())
				if !ok {
					b, err := api.getBenchmarks(network, h.PublicKey, time.Now().AddDate(0, 0, -1), time.Now())
					if err != nil {
						return
					}
					api.cache.putBenchmarks(network, h.PublicKey, time.Now().AddDate(0, 0, -1), time.Now(), b)
				}
			}(h)
		}
	}()

	// Prefetch the next bunch of hosts.
	if more {
		go func() {
			_, _, _, ok := api.cache.getHosts(network, all, int(offset+limit), int(limit), query)
			if !ok {
				h, m, t, err := api.getHosts(network, all, int(offset+limit), int(limit), query)
				if err != nil {
					return
				}
				api.cache.putHosts(network, all, int(offset+limit), int(limit), query, h, m, t)
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
	scans, ok := api.cache.getScans(network, pk, from, to)
	if !ok {
		s, err := api.getScans(network, pk, from, to)
		if err != nil {
			api.log.Error("couldn't get scan history", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "internal error",
			})
			return
		}
		scans = s
		api.cache.putScans(network, pk, from, to, s)
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
	benchmarks, ok := api.cache.getBenchmarks(network, pk, from, to)
	if !ok {
		b, err := api.getBenchmarks(network, pk, from, to)
		if err != nil {
			api.log.Error("couldn't get benchmark history", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
			writeJSON(w, APIResponse{
				Status:  "error",
				Message: "internal error",
			})
			return
		}
		benchmarks = b
		api.cache.putBenchmarks(network, pk, from, to, b)
	}
	writeJSON(w, benchmarksResponse{
		APIResponse: APIResponse{Status: "ok"},
		PublicKey:   pk,
		Benchmarks:  benchmarks,
	})
}

func writeJSON(w http.ResponseWriter, obj interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err := json.NewEncoder(w).Encode(obj)
	if _, isJsonErr := err.(*json.SyntaxError); isJsonErr {
		log.Println("ERROR: failed to encode API response:", err)
	}
}

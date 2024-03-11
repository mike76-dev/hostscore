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
	"go.sia.tech/core/types"
	"go.uber.org/zap"
)

type APIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type hostsResponse struct {
	APIResponse
	Hosts []hostdb.HostDBEntry `json:"hosts"`
	More  bool                 `json:"more"`
}

type scansResponse struct {
	APIResponse
	PublicKey types.PublicKey   `json:"publicKey"`
	Scans     []hostdb.HostScan `json:"scans"`
}

type benchmarksResponse struct {
	APIResponse
	PublicKey  types.PublicKey        `json:"publicKey"`
	Benchmarks []hostdb.HostBenchmark `json:"benchmarks"`
}

type portalAPI struct {
	router  httprouter.Router
	store   *jsonStore
	db      *sql.DB
	token   string
	log     *zap.Logger
	clients map[string]*client.Client
	mu      sync.RWMutex
	cache   *responseCache
}

func newAPI(s *jsonStore, db *sql.DB, token string, logger *zap.Logger, cache *responseCache) *portalAPI {
	return &portalAPI{
		store:   s,
		db:      db,
		token:   token,
		log:     logger,
		clients: make(map[string]*client.Client),
		cache:   cache,
	}
}

func (api *portalAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	api.router.ServeHTTP(w, r)
	api.mu.RUnlock()
}

func (api *portalAPI) buildHTTPRoutes() {
	router := httprouter.New()

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

func (api *portalAPI) hostsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	location := strings.ToLower(req.FormValue("location"))
	network := strings.ToLower(req.FormValue("network"))
	if location == "" || network == "" {
		writeError(w, "node location or network not provided", http.StatusBadRequest)
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
			writeError(w, "invalid offset", http.StatusBadRequest)
			return
		}
	}
	lim := req.FormValue("limit")
	if lim != "" {
		limit, err = strconv.ParseInt(lim, 10, 64)
		if err != nil {
			writeError(w, "invalid limit", http.StatusBadRequest)
			return
		}
	}
	client, ok := api.clients[location]
	if !ok {
		writeError(w, "node not found", http.StatusBadRequest)
		return
	}
	hosts, more, ok := api.cache.getHosts(network, all, int(offset), int(limit), query)
	if !ok {
		resp, err := client.Hosts(network, all, int(offset), int(limit), query)
		if err != nil {
			api.log.Error("couldn't get hosts", zap.Error(err))
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}
		hosts = resp.Hosts
		more = resp.More
		api.cache.putHosts(network, all, int(offset), int(limit), query, resp.Hosts, resp.More)
	}
	for i := range hosts {
		info, lastFetched, err := getLocation(api.db, hosts[i], api.token)
		if err != nil {
			api.log.Error("couldn't get host location", zap.String("host", hosts[i].NetAddress), zap.Error(err))
			continue
		}
		if hosts[i].LastIPChange.After(lastFetched) {
			newInfo, err := external.FetchIPInfo(hosts[i].NetAddress, api.token)
			if err != nil {
				api.log.Error("couldn't fetch host location", zap.String("host", hosts[i].NetAddress), zap.Error(err))
				continue
			}
			if (newInfo != external.IPInfo{}) {
				info = newInfo
				err = saveLocation(api.db, hosts[i].PublicKey, info)
				if err != nil {
					api.log.Error("couldn't update host location", zap.String("host", hosts[i].NetAddress), zap.Error(err))
					continue
				}
			} else {
				api.log.Debug("empty host location received", zap.String("host", hosts[i].NetAddress))
			}
		}
		hosts[i].IPInfo = info
	}
	writeJSON(w, hostsResponse{
		APIResponse: APIResponse{Status: "ok"},
		Hosts:       hosts,
		More:        more,
	})
}

func (api *portalAPI) scansHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	location := strings.ToLower(req.FormValue("location"))
	network := strings.ToLower(req.FormValue("network"))
	if location == "" || network == "" {
		writeError(w, "node location or network not provided", http.StatusBadRequest)
		return
	}
	host := req.FormValue("host")
	if host == "" {
		writeError(w, "host not provided", http.StatusBadRequest)
		return
	}
	var pk types.PublicKey
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
	client, ok := api.clients[location]
	if !ok {
		writeError(w, "node not found", http.StatusBadRequest)
		return
	}
	scans, ok := api.cache.getScans(location, network, pk, from, to)
	if !ok {
		s, err := client.Scans(network, pk, from, to)
		if err != nil {
			api.log.Error("couldn't get scan history", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}
		scans = s
		api.cache.putScans(location, network, pk, from, to, s)
	}
	writeJSON(w, scansResponse{
		APIResponse: APIResponse{Status: "ok"},
		PublicKey:   pk,
		Scans:       scans,
	})
}

func (api *portalAPI) benchmarksHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	location := strings.ToLower(req.FormValue("location"))
	network := strings.ToLower(req.FormValue("network"))
	if location == "" || network == "" {
		writeError(w, "node location or network not provided", http.StatusBadRequest)
		return
	}
	host := req.FormValue("host")
	if host == "" {
		writeError(w, "host not provided", http.StatusBadRequest)
		return
	}
	var pk types.PublicKey
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
	client, ok := api.clients[location]
	if !ok {
		writeError(w, "node not found", http.StatusBadRequest)
		return
	}
	benchmarks, ok := api.cache.getBenchmarks(location, network, pk, from, to)
	if !ok {
		b, err := client.Benchmarks(network, pk, from, to)
		if err != nil {
			api.log.Error("couldn't get benchmark history", zap.String("network", network), zap.Stringer("host", pk), zap.Error(err))
			writeError(w, "internal error", http.StatusInternalServerError)
			return
		}
		benchmarks = b
		api.cache.putBenchmarks(location, network, pk, from, to, b)
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

func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	err := json.NewEncoder(w).Encode(APIResponse{
		Status:  "error",
		Message: msg,
	})
	if _, isJsonErr := err.(*json.SyntaxError); isJsonErr {
		log.Println("ERROR: failed to encode API error response:", err)
	}
}

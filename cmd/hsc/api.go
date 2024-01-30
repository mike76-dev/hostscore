package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/mike76-dev/hostscore/hostdb"
	"go.sia.tech/core/types"
)

type APIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type hostsResponse struct {
	APIResponse
	Hosts []hostdb.HostDBEntry `json:"hosts"`
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
	clients map[string]*client
	mu      sync.RWMutex
}

func newAPI(s *jsonStore) *portalAPI {
	return &portalAPI{
		store:   s,
		clients: make(map[string]*client),
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
	client, ok := api.clients[location+"-"+network]
	if !ok {
		writeError(w, "node not found", http.StatusBadRequest)
		return
	}
	hosts, err := client.hosts(int(offset), int(limit))
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, hostsResponse{
		APIResponse: APIResponse{Status: "ok"},
		Hosts:       hosts,
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
	client, ok := api.clients[location+"-"+network]
	if !ok {
		writeError(w, "node not found", http.StatusBadRequest)
		return
	}
	scans, err := client.scans(pk, from, to)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, scansResponse{
		APIResponse: APIResponse{Status: "ok"},
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
	client, ok := api.clients[location+"-"+network]
	if !ok {
		writeError(w, "node not found", http.StatusBadRequest)
		return
	}
	benchmarks, err := client.benchmarks(pk, from, to)
	if err != nil {
		writeError(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, benchmarksResponse{
		APIResponse: APIResponse{Status: "ok"},
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

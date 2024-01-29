package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/julienschmidt/httprouter"
)

type portalAPI struct {
	router httprouter.Router
	mu     sync.RWMutex
}

func (api *portalAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	api.router.ServeHTTP(w, r)
	api.mu.RUnlock()
}

func (api *portalAPI) buildHTTPRoutes() {
	router := httprouter.New()

	api.mu.Lock()
	api.router = *router
	api.mu.Unlock()
}

func writeJSON(w http.ResponseWriter, obj interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	err := json.NewEncoder(w).Encode(obj)
	if _, isJsonErr := err.(*json.SyntaxError); isJsonErr {
		log.Println("ERROR: failed to encode API error response:", err)
	}
}

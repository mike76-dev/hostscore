package main

import (
	"net"
	"net/http"
	"time"
)

type inboundRequest struct {
	RemoteHost     string
	FirstTimestamp int64
	LastTimestamp  int64
	Count          int64
}

// getRemoteHost returns the address of the remote host.
func getRemoteHost(r *http.Request) (addr string) {
	addr, _, _ = net.SplitHostPort(r.RemoteAddr)
	if addr == "127.0.0.1" || addr == "localhost" {
		xff := r.Header.Values("X-Forwarded-For")
		if len(xff) > 0 {
			addr = xff[0]
		}
	}
	return
}

// rateExceeded updates the requests map and returns true if there have
// been more requests from the given IP than allowed.
func (api *portalAPI) rateExceeded(addr string) bool {
	api.mu.Lock()
	defer api.mu.Unlock()

	ir := api.requests[addr]
	now := time.Now().Unix()
	if ir.LastTimestamp == 0 || now-ir.LastTimestamp > int64(time.Minute.Seconds()) {
		ir.FirstTimestamp = now
		ir.Count = 0
	}
	ir.LastTimestamp = now
	ir.Count++
	api.requests[addr] = ir

	if ir.Count > 1 && float64(ir.Count)/float64(ir.LastTimestamp-ir.FirstTimestamp) > 10 {
		return true
	}

	return false
}

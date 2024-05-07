package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const maxRequestsPerSecond = 10

// ratelimiter keeps the API request stats and determines whether
// to allow the request or not.
type ratelimiter struct {
	requests map[string]int
	mu       sync.Mutex
}

func newRatelimiter(stopChan chan struct{}) *ratelimiter {
	rl := &ratelimiter{
		requests: make(map[string]int),
	}

	ticker := time.Tick(time.Second)
	go func() {
		for range ticker {
			select {
			case <-stopChan:
				return
			default:
			}
			rl.mu.Lock()
			rl.requests = make(map[string]int)
			rl.mu.Unlock()
		}
	}()

	return rl
}

// limitExceeded returns true if there are too many requests from the given host.
func (rl *ratelimiter) limitExceeded(addr string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.requests[addr]++
	return rl.requests[addr] > maxRequestsPerSecond
}

// getRemoteHost returns the address of the remote host.
func getRemoteHost(r *http.Request) (host string) {
	host, _, _ = net.SplitHostPort(r.RemoteAddr)
	if host == "127.0.0.1" || host == "localhost" {
		xff := r.Header.Values("X-Forwarded-For")
		if len(xff) > 0 {
			host = xff[0]
		}
	}
	return
}

package main

import (
	"net/http"
	"testing"
	"time"
)

func TestRatelimiter(t *testing.T) {
	stopChan := make(chan struct{})
	defer close(stopChan)
	rl := newRatelimiter(stopChan)

	for i := 0; i < maxRequestsPerSecond; i++ {
		if rl.limitExceeded("1.2.3.4") {
			t.Fatalf("request %d: limit exceeded too early", i+1)
		}
	}
	if !rl.limitExceeded("1.2.3.4") {
		t.Fatal("limit not exceeded after maxRequestsPerSecond requests")
	}

	// Another host has its own counter.
	if rl.limitExceeded("5.6.7.8") {
		t.Fatal("limit of one host applied to another")
	}
}

func TestRatelimiterReset(t *testing.T) {
	stopChan := make(chan struct{})
	defer close(stopChan)
	rl := newRatelimiter(stopChan)

	for i := 0; i <= maxRequestsPerSecond; i++ {
		rl.limitExceeded("1.2.3.4")
	}
	if !rl.limitExceeded("1.2.3.4") {
		t.Fatal("limit not exceeded")
	}

	// The counters are reset every second. This used to never happen:
	// the reset goroutine ignored its ticker.
	time.Sleep(1100 * time.Millisecond)
	if rl.limitExceeded("1.2.3.4") {
		t.Fatal("limit still exceeded after the reset interval")
	}
}

func TestGetRemoteHost(t *testing.T) {
	req := func(remoteAddr, xff string) *http.Request {
		r := &http.Request{RemoteAddr: remoteAddr, Header: make(http.Header)}
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}

	tests := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"direct connection", req("1.2.3.4:5678", ""), "1.2.3.4"},
		{"X-Forwarded-For ignored for remote hosts", req("1.2.3.4:5678", "8.8.8.8"), "1.2.3.4"},
		{"proxied via localhost", req("127.0.0.1:5678", "8.8.8.8"), "8.8.8.8"},
		{"localhost without proxy header", req("127.0.0.1:5678", ""), "127.0.0.1"},
	}
	for _, tt := range tests {
		if got := getRemoteHost(tt.r); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

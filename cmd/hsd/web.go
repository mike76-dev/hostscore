package main

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/api"
	"go.sia.tech/jape"
)

func startWeb(l net.Listener, a *app, password string) error {
	server := api.NewServer(a)
	api := jape.BasicAuth(password)(server)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api") {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
				api.ServeHTTP(w, r)
				return
			}
		}),
		// The API port is network-exposed: without these timeouts a
		// client holding connections open (e.g. slowloris) ties up
		// goroutines and file descriptors indefinitely.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       1 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	return srv.Serve(l)
}

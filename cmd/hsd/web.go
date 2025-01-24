package main

import (
	"net"
	"net/http"
	"strings"

	"github.com/mike76-dev/hostscore/api"
	"go.sia.tech/jape"
)

func startWeb(l net.Listener, a *app, password string) error {
	server := api.NewServer(a)
	api := jape.BasicAuth(password)(server)
	return http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
			api.ServeHTTP(w, r)
			return
		}
	}))
}

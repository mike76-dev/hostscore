package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"

	"github.com/mike76-dev/hostscore/internal/build"
	"github.com/mike76-dev/hostscore/persist"
)

// startDaemon starts the hsd server.
func startDaemon(config *persist.Config, apiPassword, dbPassword string, seeds map[string]string) error {
	fmt.Printf("hsd v%v\n", build.NodeVersion)
	if build.GitRevision == "" {
		fmt.Println("WARN: compiled without build commit or version. To compile correctly, please use the makefile")
	} else {
		fmt.Println("Git Revision " + build.GitRevision)
	}
	fmt.Println("Loading...")

	// Start listening to the API requests.
	l, err := net.Listen("tcp", config.Config.APIAddr)
	if err != nil {
		log.Fatal(err)
	}

	a := initialize(config, dbPassword, seeds)

	stops := make(map[string]func())
	for name := range seeds {
		stops[name] = a.nodes[name].Start()
	}

	log.Println("api: Listening on", l.Addr())
	go func() {
		if err := startWeb(l, a, apiPassword); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Println("ERROR: web server failed:", err)
		}
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	<-signalCh
	log.Println("Shutting down...")
	// Close the HostDB first: it waits for the scanning, benchmarking and
	// syncing goroutines and commits the store transactions, which needs
	// both the SQL database and the chain managers still available.
	a.hdb.Close()
	for name := range seeds {
		stops[name]()
	}

	a.db.Close()

	return nil
}

package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"

	"github.com/mike76-dev/hostscore/internal/build"
	"github.com/mike76-dev/hostscore/persist"
)

// startDaemon starts the hsd server.
func startDaemon(config *persist.HSDConfig, apiPassword, dbPassword string, seeds map[string]string) error {
	fmt.Printf("hsd v%v\n", build.NodeVersion)
	if build.GitRevision == "" {
		fmt.Println("WARN: compiled without build commit or version. To compile correctly, please use the makefile")
	} else {
		fmt.Println("Git Revision " + build.GitRevision)
	}
	fmt.Println("Loading...")

	// Start listening to the API requests.
	l, err := net.Listen("tcp", config.APIAddr)
	if err != nil {
		log.Fatal(err)
	}

	a := initialize(config, dbPassword, seeds)

	stops := make(map[string]func())
	for name := range seeds {
		stops[name] = a.nodes[name].Start()
	}

	log.Println("api: Listening on", l.Addr())
	go startWeb(l, a, apiPassword)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	<-signalCh
	log.Println("Shutting down...")
	for name := range seeds {
		stops[name]()
	}

	a.db.Close()

	return nil
}

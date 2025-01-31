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
func startDaemon(config *persist.HSDConfig, apiPassword, dbPassword, seed, seedZen string) error {
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

	a := initialize(config, dbPassword, seed, seedZen)

	stopMainnet := a.nodes["mainnet"].Start()
	stopZen := a.nodes["zen"].Start()
	log.Println("api: Listening on", l.Addr())
	go startWeb(l, a, apiPassword)
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	<-signalCh
	log.Println("Shutting down...")
	stopZen()
	stopMainnet()
	a.db.Close()

	return nil
}

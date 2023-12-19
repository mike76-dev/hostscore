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
func startDaemon(config *persist.HSDConfig, apiPassword string, dbPassword string) error {
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
	n, err := newNode(config, dbPassword)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("p2p: Listening on", n.s.Addr())
	stop := n.Start()
	log.Println("api: Listening on", l.Addr())
	go startWeb(l, n, apiPassword)
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	<-signalCh
	log.Println("Shutting down...")
	stop()

	return nil
}

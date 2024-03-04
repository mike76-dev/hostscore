package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"

	client "github.com/mike76-dev/hostscore/api"
	"github.com/mike76-dev/hostscore/internal/build"
)

func main() {
	log.SetFlags(0)

	dir := flag.String("dir", ".", "directory to store files in")
	portalPort := flag.String("portal", ":8080", "port number the portal server listens at")
	flag.Parse()

	err := os.MkdirAll(*dir, 0700)
	if err != nil {
		log.Fatalf("Provided parameter is invalid: %v\n", *dir)
	}

	fmt.Printf("hsc v%v\n", build.NodeVersion)
	if build.GitRevision == "" {
		fmt.Println("WARN: compiled without build commit or version. To compile correctly, please use the makefile")
	} else {
		fmt.Println("Git Revision " + build.GitRevision)
	}

	s, err := newJSONStore(*dir)
	if err != nil {
		log.Fatal(err)
	}

	l, err := net.Listen("tcp", "127.0.0.1"+*portalPort)
	if err != nil {
		log.Fatal(err)
	}

	api := newAPI(s)
	for key, node := range s.nodes {
		api.clients[key] = client.NewClient(node.Address, node.Password)
	}
	api.buildHTTPRoutes()

	closeChan := make(chan int, 1)
	srv := &http.Server{Handler: api}
	go srv.Serve(l)
	fmt.Println("Listening on", l.Addr())

	go func() {
		<-closeChan
		srv.Shutdown(context.Background())
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	<-signalCh
	fmt.Println("Shutting down...")
	closeChan <- 1
}

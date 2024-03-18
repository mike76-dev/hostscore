package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/go-sql-driver/mysql"
	client "github.com/mike76-dev/hostscore/api"
	"github.com/mike76-dev/hostscore/internal/build"
	"github.com/mike76-dev/hostscore/persist"
	"golang.org/x/term"
)

func getDBPassword() string {
	dbPassword := os.Getenv("HSC_DB_PASSWORD")
	if dbPassword != "" {
		log.Println("Using HSC_DB_PASSWORD environment variable.")
	} else {
		fmt.Print("Enter database password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			log.Fatalf("Could not read database password: %v\n", err)
		}
		dbPassword = string(pw)
	}
	return dbPassword
}

func main() {
	log.SetFlags(0)

	dir := flag.String("dir", ".", "directory to store files in")
	dbName := flag.String("db-name", "", "name of the MySQL database")
	dbUser := flag.String("db-user", "", "name of the database user")
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

	dbPassword := getDBPassword()

	log.Println("Connecting to the SQL database...")
	cfg := mysql.Config{
		User:                 *dbUser,
		Passwd:               dbPassword,
		Net:                  "tcp",
		Addr:                 "127.0.0.1:3306",
		DBName:               *dbName,
		AllowNativePasswords: true,
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatalf("Could not connect to the database: %v\n", err)
	}
	err = db.Ping()
	if err != nil {
		log.Fatalf("MySQL database not responding: %v\n", err)
	}
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	defer db.Close()

	apiToken := os.Getenv("HSC_API_TOKEN")
	if apiToken != "" {
		log.Println("Using HSC_API_TOKEN environment variable.")
	}

	s, err := newJSONStore(*dir)
	if err != nil {
		log.Fatal(err)
	}

	l, err := net.Listen("tcp", "127.0.0.1"+*portalPort)
	if err != nil {
		log.Fatal(err)
	}

	logger, closeFn, err := persist.NewFileLogger(filepath.Join(*dir, "hsc.log"))
	if err != nil {
		log.Fatal(err)
	}
	defer closeFn()

	cache := newCache()
	defer cache.close()

	api, err := newAPI(s, db, apiToken, logger, cache)
	if err != nil {
		log.Fatal(err)
	}
	defer api.close()

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

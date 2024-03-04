package main

import (
	"database/sql"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/internal/syncerutil"
	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/internal/walletutil"
	"github.com/mike76-dev/hostscore/persist"
	"github.com/mike76-dev/hostscore/syncer"
	"go.sia.tech/core/gateway"
	"go.sia.tech/coreutils"
	"go.sia.tech/coreutils/chain"
)

// Network bootstraps.
var (
	mainnetBootstrap = []string{
		"108.227.62.195:9981",
		"139.162.81.190:9991",
		"144.217.7.188:9981",
		"147.182.196.252:9981",
		"15.235.85.30:9981",
		"167.235.234.84:9981",
		"173.235.144.230:9981",
		"198.98.53.144:7791",
		"199.27.255.169:9981",
		"2.136.192.200:9981",
		"213.159.50.43:9981",
		"24.253.116.61:9981",
		"46.249.226.103:9981",
		"5.165.236.113:9981",
		"5.252.226.131:9981",
		"54.38.120.222:9981",
		"62.210.136.25:9981",
		"63.135.62.123:9981",
		"65.21.93.245:9981",
		"75.165.149.114:9981",
		"77.51.200.125:9981",
		"81.6.58.121:9981",
		"83.194.193.156:9981",
		"84.39.246.63:9981",
		"87.99.166.34:9981",
		"91.214.242.11:9981",
		"93.105.88.181:9981",
		"93.180.191.86:9981",
		"94.130.220.162:9981",
	}

	zenBootstrap = []string{
		"147.135.16.182:9881",
		"147.135.39.109:9881",
		"51.81.208.10:9881",
	}
)

type node struct {
	cm    *chain.Manager
	cmZen *chain.Manager
	s     *syncer.Syncer
	sZen  *syncer.Syncer
	w     *walletutil.Wallet
	hdb   *hostdb.HostDB
	db    *sql.DB

	Start func() (stop func())
}

func newNode(config *persist.HSDConfig, dbPassword, seed, seedZen string) (*node, error) {
	log.Println("Connecting to the SQL database...")
	cfg := mysql.Config{
		User:                 config.DBUser,
		Passwd:               dbPassword,
		Net:                  "tcp",
		Addr:                 "127.0.0.1:3306",
		DBName:               config.DBName,
		AllowNativePasswords: true,
	}
	mdb, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatalf("Could not connect to the database: %v\n", err)
	}
	err = mdb.Ping()
	if err != nil {
		log.Fatalf("MySQL database not responding: %v\n", err)
	}
	mdb.SetConnMaxLifetime(time.Minute * 3)
	mdb.SetMaxOpenConns(10)
	mdb.SetMaxIdleConns(10)

	// Make sure the path is an absolute one.
	dir, err := filepath.Abs(config.Dir)
	if err != nil {
		log.Fatalf("Provided parameter is invalid: %v\n", config.Dir)
	}

	// Create the state directory if it does not yet exist.
	// This also checks if the provided directory parameter is valid.
	err = os.MkdirAll(dir, 0700)
	if err != nil {
		log.Fatalf("Provided parameter is invalid: %v\n", dir)
	}

	// Mainnet.
	log.Println("Connecting to Mainnet...")
	mainnet, genesisBlockMainnet := chain.Mainnet()

	dirMainnet := filepath.Join(dir, "mainnet")
	err = os.MkdirAll(dirMainnet, 0700)
	if err != nil {
		log.Fatalf("Provided parameter is invalid: %v\n", dirMainnet)
	}

	bdb, err := coreutils.OpenBoltChainDB(filepath.Join(dirMainnet, "consensus.db"))
	if err != nil {
		log.Fatal(err)
	}
	dbstore, tipState, err := chain.NewDBStore(bdb, mainnet, genesisBlockMainnet)
	if err != nil {
		return nil, err
	}
	cm := chain.NewManager(dbstore, tipState)

	l, err := net.Listen("tcp", config.GatewayMainnet)
	if err != nil {
		return nil, err
	}

	// Peers will reject us if our hostname is empty or unspecified, so use loopback.
	syncerAddr := l.Addr().String()
	host, port, _ := net.SplitHostPort(syncerAddr)
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		syncerAddr = net.JoinHostPort("127.0.0.1", port)
	}

	ps, err := syncerutil.NewJSONPeerStore(filepath.Join(dirMainnet, "peers.json"))
	if err != nil {
		log.Fatal(err)
	}
	for _, peer := range mainnetBootstrap {
		ps.AddPeer(peer)
	}
	header := gateway.Header{
		GenesisID:  genesisBlockMainnet.ID(),
		UniqueID:   gateway.GenerateUniqueID(),
		NetAddress: syncerAddr,
	}
	s := syncer.New(l, cm, ps, header, syncer.WithLogger(dirMainnet))

	// Zen.
	log.Println("Connecting to Zen...")
	zen, genesisBlockZen := chain.TestnetZen()

	dirZen := filepath.Join(dir, "zen")
	err = os.MkdirAll(dirZen, 0700)
	if err != nil {
		log.Fatalf("Provided parameter is invalid: %v\n", dirZen)
	}

	bdbZen, err := coreutils.OpenBoltChainDB(filepath.Join(dirZen, "consensus.db"))
	if err != nil {
		log.Fatal(err)
	}
	dbstoreZen, tipStateZen, err := chain.NewDBStore(bdbZen, zen, genesisBlockZen)
	if err != nil {
		return nil, err
	}
	cmZen := chain.NewManager(dbstoreZen, tipStateZen)

	lZen, err := net.Listen("tcp", config.GatewayZen)
	if err != nil {
		return nil, err
	}

	// Peers will reject us if our hostname is empty or unspecified, so use loopback.
	syncerAddrZen := lZen.Addr().String()
	host, port, _ = net.SplitHostPort(syncerAddrZen)
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		syncerAddrZen = net.JoinHostPort("127.0.0.1", port)
	}

	psZen, err := syncerutil.NewJSONPeerStore(filepath.Join(dirZen, "peers.json"))
	if err != nil {
		log.Fatal(err)
	}
	for _, peer := range zenBootstrap {
		psZen.AddPeer(peer)
	}
	headerZen := gateway.Header{
		GenesisID:  genesisBlockZen.ID(),
		UniqueID:   gateway.GenerateUniqueID(),
		NetAddress: syncerAddrZen,
	}
	sZen := syncer.New(lZen, cmZen, psZen, headerZen, syncer.WithLogger(dirZen))

	log.Println("Loading wallet...")
	w, err := walletutil.NewWallet(mdb, seed, seedZen, config.Dir, cm, cmZen, s, sZen)
	if err != nil {
		return nil, err
	}

	log.Println("Loading host database...")
	hdb, errChan := hostdb.NewHostDB(mdb, config.Dir, cm, cmZen, s, sZen, w)
	if err := utils.PeekErr(errChan); err != nil {
		return nil, err
	}

	return &node{
		cm:    cm,
		cmZen: cmZen,
		s:     s,
		sZen:  sZen,
		w:     w,
		hdb:   hdb,
		db:    mdb,
		Start: func() func() {
			ch := make(chan struct{})
			go func() {
				s.Run()
				close(ch)
			}()
			chZen := make(chan struct{})
			go func() {
				sZen.Run()
				close(chZen)
			}()
			return func() {
				l.Close()
				<-ch
				lZen.Close()
				<-chZen
				hdb.Close()
				w.Close()
				bdb.Close()
				bdbZen.Close()
				mdb.Close()
			}
		},
	}, nil
}

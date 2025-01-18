package main

import (
	"context"
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
	"go.sia.tech/core/gateway"
	"go.sia.tech/coreutils"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/coreutils/syncer"
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
	mainnetBootstrap := syncer.MainnetBootstrapPeers

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
	cmLogger, cmCloseFn, err := persist.NewFileLogger(filepath.Join(dirMainnet, "cm.log"))
	if err != nil {
		log.Fatal(err)
	}
	cm := chain.NewManager(dbstore, tipState)
	chain.WithLog(cmLogger)(cm)

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
		if err := ps.AddPeer(peer); err != nil {
			log.Fatal(err)
		}
	}
	header := gateway.Header{
		GenesisID:  genesisBlockMainnet.ID(),
		UniqueID:   gateway.GenerateUniqueID(),
		NetAddress: syncerAddr,
	}
	syncerLogger, syncerCloseFn, err := persist.NewFileLogger(filepath.Join(dirMainnet, "syncer.log"))
	if err != nil {
		log.Fatal(err)
	}
	s := syncer.New(l, cm, ps, header, syncer.WithLogger(syncerLogger))

	// Zen.
	log.Println("Connecting to Zen...")
	zen, genesisBlockZen := chain.TestnetZen()
	zenBootstrap := syncer.ZenBootstrapPeers

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
	cmLoggerZen, cmCloseFnZen, err := persist.NewFileLogger(filepath.Join(dirZen, "cm.log"))
	if err != nil {
		log.Fatal(err)
	}
	cmZen := chain.NewManager(dbstoreZen, tipStateZen)
	chain.WithLog(cmLoggerZen)(cmZen)

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
		if err := psZen.AddPeer(peer); err != nil {
			log.Fatal(err)
		}
	}
	headerZen := gateway.Header{
		GenesisID:  genesisBlockZen.ID(),
		UniqueID:   gateway.GenerateUniqueID(),
		NetAddress: syncerAddrZen,
	}
	syncerLoggerZen, syncerCloseFnZen, err := persist.NewFileLogger(filepath.Join(dirZen, "syncer.log"))
	if err != nil {
		log.Fatal(err)
	}
	sZen := syncer.New(lZen, cmZen, psZen, headerZen, syncer.WithLogger(syncerLoggerZen))

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
			ctx := context.Background()
			ch := make(chan struct{})
			go func() {
				s.Run(ctx)
				close(ch)
			}()
			chZen := make(chan struct{})
			go func() {
				sZen.Run(ctx)
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
				syncerCloseFn()
				syncerCloseFnZen()
				cmCloseFn()
				cmCloseFnZen()
			}
		},
	}, nil
}

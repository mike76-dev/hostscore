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
	"github.com/mike76-dev/hostscore/persist"
	walletutil "github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/gateway"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/coreutils/syncer"
	"go.uber.org/zap/zapcore"
)

type node struct {
	chain  *chain.Manager
	syncer *syncer.Syncer
	wm     *walletutil.WalletManager
	db     *sql.DB

	Start func() (stop func())
}

type app struct {
	nodes map[string]*node
	db    *sql.DB
	hdb   *hostdb.HostDB
}

func (a *app) ChainManager(network string) *chain.Manager {
	node, ok := a.nodes[network]
	if ok {
		return node.chain
	}
	return nil
}

func (a *app) Syncer(network string) *syncer.Syncer {
	node, ok := a.nodes[network]
	if ok {
		return node.syncer
	}
	return nil
}

func (a *app) Wallet(network string) *walletutil.WalletManager {
	node, ok := a.nodes[network]
	if ok {
		return node.wm
	}
	return nil
}

func (a *app) HostDB() *hostdb.HostDB {
	return a.hdb
}

func (a *app) Networks() []string {
	var networks []string
	for network := range a.nodes {
		networks = append(networks, network)
	}
	return networks
}

func initialize(config *persist.HSDConfig, dbPassword string, seeds map[string]string) *app {
	log.Println("Connecting to the SQL database...")
	cfg := mysql.Config{
		User:                 config.DBUser,
		Passwd:               dbPassword,
		Net:                  "tcp",
		Addr:                 "127.0.0.1:3306",
		DBName:               config.DBName,
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

	a := &app{
		nodes: make(map[string]*node),
		db:    db,
	}

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

	// Connect to the networks.
	var networks []string
	for name, seed := range seeds {
		title := utils.Capitalize(name)
		log.Printf("Connecting to %s...\n", title)
		var network *consensus.Network
		var genesisBlock types.Block
		var bootstrap []string
		var gatewayAddr string
		switch name {
		case "mainnet":
			network, genesisBlock = chain.Mainnet()
			bootstrap = syncer.MainnetBootstrapPeers
			gatewayAddr = config.GatewayMainnet
		case "zen":
			network, genesisBlock = chain.TestnetZen()
			bootstrap = syncer.ZenBootstrapPeers
			gatewayAddr = config.GatewayZen
		case "anagami":
			network, genesisBlock = chain.TestnetAnagami()
			bootstrap = syncer.AnagamiBootstrapPeers
			gatewayAddr = config.GatewayAnagami
		}

		node, err := newNode(db, name, seed, dir, gatewayAddr, network, genesisBlock, bootstrap)
		if err != nil {
			log.Fatalf("Couldn't start %s node: %v\n", title, err)
		}

		a.nodes[name] = node
		log.Printf("p2p %s: Listening on %s\n", title, node.syncer.Addr())
		networks = append(networks, name)
	}

	log.Println("Loading host database...")
	hdb, errChan := hostdb.NewHostDB(db, config.Dir, a, networks)
	if err := utils.PeekErr(errChan); err != nil {
		log.Fatalf("Couldn't load host database: %v\n", err)
	}

	a.hdb = hdb

	return a
}

func newNode(db *sql.DB, name, seed, dir, p2p string, network *consensus.Network, genesis types.Block, peers []string) (*node, error) {
	dir = filepath.Join(dir, name)
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		log.Fatalf("Provided parameter is invalid: %v\n", dir)
	}

	bdb, err := coreutils.OpenBoltChainDB(filepath.Join(dir, "consensus.db"))
	if err != nil {
		return nil, err
	}

	dbstore, tipState, err := chain.NewDBStore(bdb, network, genesis)
	if err != nil {
		return nil, err
	}

	cmLogger, cmCloseFn, err := persist.NewFileLogger(filepath.Join(dir, "cm.log"), zapcore.ErrorLevel)
	if err != nil {
		return nil, err
	}

	cm := chain.NewManager(dbstore, tipState)
	chain.WithLog(cmLogger)(cm)

	l, err := net.Listen("tcp", p2p)
	if err != nil {
		return nil, err
	}

	// Peers will reject us if our hostname is empty or unspecified, so use loopback.
	syncerAddr := l.Addr().String()
	host, port, _ := net.SplitHostPort(syncerAddr)
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		syncerAddr = net.JoinHostPort("127.0.0.1", port)
	}

	ps, err := syncerutil.NewJSONPeerStore(filepath.Join(dir, "peers.json"))
	if err != nil {
		return nil, err
	}

	for _, peer := range peers {
		if err := ps.AddPeer(peer); err != nil {
			log.Fatal(err)
		}
	}

	header := gateway.Header{
		GenesisID:  genesis.ID(),
		UniqueID:   gateway.GenerateUniqueID(),
		NetAddress: syncerAddr,
	}

	syncerLogger, syncerCloseFn, err := persist.NewFileLogger(filepath.Join(dir, "syncer.log"), zapcore.ErrorLevel)
	if err != nil {
		return nil, err
	}

	s := syncer.New(l, cm, ps, header, syncer.WithLogger(syncerLogger))

	walletLogger, walletCloseFn, err := persist.NewFileLogger(filepath.Join(dir, "wallet.log"), zapcore.InfoLevel)
	if err != nil {
		return nil, err
	}

	store, _, err := walletutil.NewDBStore(db, seed, name, walletLogger)
	if err != nil {
		return nil, err
	}

	wm, err := walletutil.NewWallet(store, cm, s, walletLogger)
	if err != nil {
		return nil, err
	}

	return &node{
		chain:  cm,
		syncer: s,
		wm:     wm,
		db:     db,
		Start: func() func() {
			ch := make(chan struct{})
			go func() {
				s.Run()
				close(ch)
			}()
			return func() {
				l.Close()
				<-ch
				wm.Close()
				bdb.Close()
				walletCloseFn()
				syncerCloseFn()
				cmCloseFn()
			}
		},
	}, nil
}

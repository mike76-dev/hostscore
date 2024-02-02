package hostdb

import (
	"database/sql"
	"log"
	"path/filepath"
	"sync"
	"time"

	siasync "github.com/mike76-dev/hostscore/internal/sync"
	"github.com/mike76-dev/hostscore/internal/walletutil"
	"github.com/mike76-dev/hostscore/persist"
	"github.com/mike76-dev/hostscore/syncer"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
)

// A HostDBEntry represents one host entry in the HostDB. It
// aggregates the host's external settings and metrics with its public key.
type HostDBEntry struct {
	ID            int                        `json:"id"`
	PublicKey     types.PublicKey            `json:"publicKey"`
	FirstSeen     time.Time                  `json:"firstSeen"`
	KnownSince    uint64                     `json:"knownSince"`
	NetAddress    string                     `json:"netaddress"`
	Blocked       bool                       `json:"blocked"`
	Uptime        time.Duration              `json:"uptime"`
	Downtime      time.Duration              `json:"downtime"`
	ScanHistory   []HostScan                 `json:"scanHistory"`
	LastBenchmark HostBenchmark              `json:"lastBenchmark"`
	Interactions  HostInteractions           `json:"interactions"`
	LastSeen      time.Time                  `json:"lastSeen"`
	IPNets        []string                   `json:"ipNets"`
	LastIPChange  time.Time                  `json:"lastIPChange"`
	Revision      types.FileContractRevision `json:"-"`
	Settings      rhpv2.HostSettings         `json:"settings"`
	PriceTable    rhpv3.HostPriceTable       `json:"priceTable"`
	IPInfo
}

// HostInteractions combines historic and recent interactions.
type HostInteractions struct {
	HistoricSuccesses float64 `json:"historicSuccessfulInteractions"`
	HistoricFailures  float64 `json:"historicFailedInteractions"`
	RecentSuccesses   float64 `json:"recentSuccessfulInteractions"`
	RecentFailures    float64 `json:"recentFailedInteractions"`
	LastUpdate        uint64  `json:"-"`
}

// A HostScan contains all information measured during a host scan.
type HostScan struct {
	Timestamp  time.Time            `json:"timestamp"`
	Success    bool                 `json:"success"`
	Latency    time.Duration        `json:"latency"`
	Error      string               `json:"error"`
	Settings   rhpv2.HostSettings   `json:"settings"`
	PriceTable rhpv3.HostPriceTable `json:"priceTable"`
}

// ScanHistory combines the scan history with the host's public key.
type ScanHistory struct {
	HostScan
	PublicKey types.PublicKey `json:"publicKey"`
}

// A HostBenchmark contains the information measured during a host benchmark.
type HostBenchmark struct {
	Timestamp     time.Time     `json:"timestamp"`
	Success       bool          `json:"success"`
	Error         string        `json:"error"`
	UploadSpeed   float64       `json:"uploadSpeed"`
	DownloadSpeed float64       `json:"downloadSpeed"`
	TTFB          time.Duration `json:"ttfb"`
}

// BenchmarkHistory combines the benchmark history with the host's public key.
type BenchmarkHistory struct {
	HostBenchmark
	PublicKey types.PublicKey `json:"publicKey"`
}

// The HostDB is a database of hosts.
type HostDB struct {
	syncer *syncer.Syncer
	cm     *chain.Manager
	s      *hostDBStore
	w      *walletutil.Wallet
	log    *persist.Logger

	tg siasync.ThreadGroup
	mu sync.Mutex

	benchmarking         bool
	initialScanLatencies []time.Duration
	scanList             []*HostDBEntry
	benchmarkList        []*HostDBEntry
	scanMap              map[types.PublicKey]bool
	scanThreads          int
}

// Hosts returns a list of HostDB's hosts.
func (hdb *HostDB) Hosts(offset, limit int) (hosts []HostDBEntry) {
	return hdb.s.getHosts(offset, limit)
}

// Scans returns the host's scan history.
func (hdb *HostDB) Scans(pk types.PublicKey, from, to time.Time) (scans []HostScan, err error) {
	return hdb.s.getScans(pk, from, to)
}

// ScanHistory returns the host's scan history.
func (hdb *HostDB) ScanHistory(from, to time.Time) (history []ScanHistory, err error) {
	return hdb.s.getScanHistory(from, to)
}

// Benchmarks returns the host's benchmark history.
func (hdb *HostDB) Benchmarks(pk types.PublicKey, from, to time.Time) (benchmarks []HostBenchmark, err error) {
	return hdb.s.getBenchmarks(pk, from, to)
}

// BenchmarkHistory returns the host's benchmark history.
func (hdb *HostDB) BenchmarkHistory(from, to time.Time) (history []BenchmarkHistory, err error) {
	return hdb.s.getBenchmarkHistory(from, to)
}

// Close shuts down HostDB.
func (hdb *HostDB) Close() {
	if err := hdb.tg.Stop(); err != nil {
		hdb.log.Println("[ERROR] unable to stop threads:", err)
	}
	hdb.s.close()
}

// NewHostDB returns a new HostDB.
func NewHostDB(db *sql.DB, network, dir string, cm *chain.Manager, syncer *syncer.Syncer, w *walletutil.Wallet) (*HostDB, <-chan error) {
	errChan := make(chan error, 1)
	l, err := persist.NewFileLogger(filepath.Join(dir, "hostdb.log"))
	if err != nil {
		log.Fatal(err)
	}
	store, tip, err := newHostDBStore(db, l, network)
	if err != nil {
		errChan <- err
		return nil, errChan
	}

	// Subscribe in a goroutine to prevent blocking.
	go func() {
		defer close(errChan)
		err := cm.AddSubscriber(store, tip)
		if err != nil {
			errChan <- err
		}
	}()

	hdb := &HostDB{
		syncer:  syncer,
		cm:      cm,
		w:       w,
		s:       store,
		log:     l,
		scanMap: make(map[types.PublicKey]bool),
	}
	hdb.s.hdb = hdb

	// Start the scanning thread.
	go hdb.scanHosts()

	// Fetch host locations.
	go hdb.fetchLocations()

	return hdb, errChan
}

// online returns if the HostDB is online.
func (hdb *HostDB) online() bool {
	return len(hdb.syncer.Peers()) > 0
}

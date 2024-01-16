package hostdb

import (
	"database/sql"
	"log"
	"path/filepath"
	"sync"
	"time"

	siasync "github.com/mike76-dev/hostscore/internal/sync"
	"github.com/mike76-dev/hostscore/persist"
	"github.com/mike76-dev/hostscore/syncer"
	"go.sia.tech/core/chain"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
)

// A HostDBEntry represents one host entry in the HostDB. It
// aggregates the host's external settings and metrics with its public key.
type HostDBEntry struct {
	ID           int              `json:"id"`
	PublicKey    types.PublicKey  `json:"publicKey"`
	FirstSeen    time.Time        `json:"firstSeen"`
	KnownSince   uint64           `json:"knownSince"`
	NetAddress   string           `json:"netaddress"`
	Blocked      bool             `json:"blocked"`
	Uptime       time.Duration    `json:"uptime"`
	Downtime     time.Duration    `json:"downtime"`
	ScanHistory  []HostDBScan     `json:"scanHistory"`
	Interactions HostInteractions `json:"interactions"`
	LastSeen     time.Time        `json:"lastSeen"`
	IPNets       []string         `json:"ipNets"`
	LastIPChange time.Time        `json:"lastIPChange"`
}

// HostInteractions combines historic and recent interactions.
type HostInteractions struct {
	HistoricSuccesses float64 `json:"historicSuccessfulInteractions"`
	HistoricFailures  float64 `json:"historicFailedInteractions"`
	RecentSuccesses   float64 `json:"recentSuccessfulInteractions"`
	RecentFailures    float64 `json:"recentFailedInteractions"`
	LastUpdate        uint64  `json:"-"`
}

// A HostDBScan contains all information measured during a host scan.
type HostDBScan struct {
	Timestamp  time.Time            `json:"timestamp"`
	Success    bool                 `json:"success"`
	Latency    time.Duration        `json:"latency"`
	Error      string               `json:"error"`
	Settings   rhpv2.HostSettings   `json:"settings"`
	PriceTable rhpv3.HostPriceTable `json:"priceTable"`
}

// The HostDB is a database of hosts.
type HostDB struct {
	syncer *syncer.Syncer
	cm     *chain.Manager
	s      *hostDBStore
	log    *persist.Logger

	tg siasync.ThreadGroup
	mu sync.Mutex

	scanning             bool
	initialScanLatencies []time.Duration
	scanList             []HostDBEntry
	scanMap              map[types.PublicKey]struct{}
	scanThreads          int
}

// Hosts returns a list of HostDB's hosts.
func (hdb *HostDB) Hosts(offset, limit int) (hosts []HostDBEntry) {
	return hdb.s.getHosts(offset, limit)
}

// Close shuts down HostDB.
func (hdb *HostDB) Close() {
	if err := hdb.tg.Stop(); err != nil {
		hdb.log.Println("[ERROR] unable to stop threads:", err)
	}
	hdb.s.close()
}

// NewHostDB returns a new HostDB.
func NewHostDB(db *sql.DB, network, dir string, cm *chain.Manager, syncer *syncer.Syncer) (*HostDB, <-chan error) {
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
		s:       store,
		log:     l,
		scanMap: make(map[types.PublicKey]struct{}),
	}
	hdb.s.hdb = hdb

	// Start the scanning thread.
	go hdb.scanHosts()

	return hdb, errChan
}

// online returns if the HostDB is online.
func (hdb *HostDB) online() bool {
	return len(hdb.syncer.Peers()) > 0
}

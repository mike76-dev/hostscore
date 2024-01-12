package hostdb

import (
	"database/sql"
	"log"
	"path/filepath"
	"time"

	"github.com/mike76-dev/hostscore/persist"
	"go.sia.tech/core/chain"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
)

// A HostDBEntry represents one host entry in the HostDB. It
// aggregates the host's external settings and metrics with its public key.
type HostDBEntry struct {
	ID                     int             `json:"id"`
	PublicKey              types.PublicKey `json:"publicKey"`
	FirstSeen              time.Time       `json:"firstSeen"`
	KnownSince             uint64          `json:"knownSince"`
	NetAddress             string          `json:"netaddress"`
	Blocked                bool            `json:"blocked"`
	Uptime                 time.Duration   `json:"uptime"`
	Downtime               time.Duration   `json:"downtime"`
	ScanHistory            []HostDBScan    `json:"scanHistory"`
	SuccessfulInteractions float64         `json:"successfulInteractions"`
	FailedInteractions     float64         `json:"failedInteractions"`
	LastSeen               time.Time       `json:"lastSeen"`
	IPNets                 []string        `json:"ipNets"`
	LastIPChange           time.Time       `json:"lastIPChange"`
}

// A HostDBScan contains all information measured during a host scan.
type HostDBScan struct {
	Timestamp   time.Time            `json:"timestamp"`
	RHP2        bool                 `json:"successRHP2"`
	RHP3        bool                 `json:"successRHP3"`
	LatencyRHP2 time.Duration        `json:"latencyRHP2"`
	LatencyRHP3 time.Duration        `json:"latencyRHP3"`
	Settings    rhpv2.HostSettings   `json:"settings"`
	PriceTable  rhpv3.HostPriceTable `json:"priceTable"`
}

// The HostDB is a database of hosts.
type HostDB struct {
	cm *chain.Manager
	s  *hostDBStore

	/*initialScanComplete  bool
	initialScanLatencies []time.Duration
	scanList             []HostDBEntry
	scanMap              map[string]struct{}
	scanWait             bool
	scanningThreads      int*/
}

// Hosts returns a list of HostDB's hosts.
func (hdb *HostDB) Hosts(offset, limit int) (hosts []HostDBEntry) {
	return hdb.s.getHosts(offset, limit)
}

// Close shuts down HostDB.
func (hdb *HostDB) Close() {
	hdb.s.close()
}

// NewHostDB returns a new HostDB.
func NewHostDB(db *sql.DB, network, dir string, cm *chain.Manager) (*HostDB, <-chan error) {
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
		cm: cm,
		s:  store,
	}

	return hdb, errChan
}

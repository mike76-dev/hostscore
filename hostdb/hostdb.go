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
	PublicKey              types.PublicKey
	FirstSeen              time.Time
	KnownSince             uint64
	NetAddress             string
	Blocked                bool
	Uptime                 time.Duration
	Downtime               time.Duration
	ScanHistory            []HostDBScan
	SuccessfulInteractions float64
	FailedInteractions     float64
	LastSeen               time.Time
	IPNets                 []string
	LastIPChange           time.Time
}

// A HostDBScan contains all information measured during a host scan.
type HostDBScan struct {
	Timestamp  time.Time
	Success    bool
	Latency    time.Duration
	Settings   rhpv2.HostSettings
	PriceTable rhpv3.HostPriceTable
}

// The HostDB is a database of hosts.
type HostDB struct {
	cm ChainManager
	s  *hostDBStore

	/*initialScanComplete  bool
	initialScanLatencies []time.Duration
	scanList             []HostDBEntry
	scanMap              map[string]struct{}
	scanWait             bool
	scanningThreads      int*/
}

type ChainManager interface {
	AddSubscriber(s chain.Subscriber, tip types.ChainIndex) error
	BestIndex(height uint64) (types.ChainIndex, bool)
}

// Close shuts down HostDB.
func (hdb *HostDB) Close() {
	hdb.s.close()
}

// NewHostDB returns a new HostDB.
func NewHostDB(db *sql.DB, network, dir string, cm ChainManager) (*HostDB, <-chan error) {
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

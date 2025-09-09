package hostdb

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/external"
	siasync "github.com/mike76-dev/hostscore/internal/sync"
	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/persist"
	walletutil "github.com/mike76-dev/hostscore/wallet"
	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/chain"
	"go.sia.tech/coreutils/syncer"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"lukechampine.com/frand"
)

// NodeStore defines the interface for accessing the node components.
type NodeStore interface {
	ChainManager(network string) *chain.Manager
	Syncer(network string) *syncer.Syncer
	Wallet(network string) *walletutil.WalletManager
	HostDB() *HostDB
	Networks() []string
}

// A HostDBEntry represents one host entry in the HostDB. It
// aggregates the host's external settings and metrics with its public key.
type HostDBEntry struct {
	ID              int                          `json:"id"`
	Network         string                       `json:"network"`
	PublicKey       types.PublicKey              `json:"publicKey"`
	FirstSeen       time.Time                    `json:"firstSeen"`
	KnownSince      uint64                       `json:"knownSince"`
	NetAddress      string                       `json:"netaddress"`
	Blocked         bool                         `json:"blocked"`
	V2              bool                         `json:"v2"`
	Uptime          time.Duration                `json:"uptime"`
	Downtime        time.Duration                `json:"downtime"`
	ScanHistory     []HostScan                   `json:"scanHistory"`
	LastBenchmark   HostBenchmark                `json:"lastBenchmark"`
	Interactions    HostInteractions             `json:"interactions"`
	LastSeen        time.Time                    `json:"lastSeen"`
	IPNets          []string                     `json:"ipNets"`
	ActiveHosts     int                          `json:"activeHosts"`
	LastIPChange    time.Time                    `json:"lastIPChange"`
	Revision        types.FileContractRevision   `json:"-"`
	V2Revision      types.V2FileContractRevision `json:"-"`
	V2Settings      rhpv4.HostSettings           `json:"v2Settings,omitempty"`
	SiamuxAddresses []string                     `json:"siamuxAddresses"`
	external.IPInfo
}

// HostInteractions summarizes the historic interactions with the host.
type HostInteractions struct {
	Successes  float64 `json:"successes"`
	Failures   float64 `json:"failures"`
	LastUpdate uint64  `json:"-"`
}

// A HostScan contains all information measured during a host scan.
type HostScan struct {
	ID         int64              `json:"-"`
	Timestamp  time.Time          `json:"timestamp"`
	Success    bool               `json:"success"`
	Latency    time.Duration      `json:"latency"`
	Error      string             `json:"error"`
	V2Settings rhpv4.HostSettings `json:"v2Settings,omitempty"`
}

// ScanHistoryEntry combines the scan history with the host's public key.
type ScanHistoryEntry struct {
	HostScan
	PublicKey types.PublicKey `json:"publicKey"`
	Network   string          `json:"network"`
	Node      string          `json:"node"`
}

// A HostBenchmark contains the information measured during a host benchmark.
type HostBenchmark struct {
	ID            int64         `json:"-"`
	Timestamp     time.Time     `json:"timestamp"`
	Success       bool          `json:"success"`
	Error         string        `json:"error"`
	UploadSpeed   float64       `json:"uploadSpeed"`
	DownloadSpeed float64       `json:"downloadSpeed"`
	TTFB          time.Duration `json:"ttfb"`
}

// BenchmarkHistoryEntry combines the benchmark history with the host's public key.
type BenchmarkHistoryEntry struct {
	HostBenchmark
	PublicKey types.PublicKey `json:"publicKey"`
	Network   string          `json:"network"`
	Node      string          `json:"node"`
}

// UpdateID is the ID of a HostUpdate.
type UpdateID = [8]byte

// HostUpdates represents a batch of updates sent to the client.
type HostUpdates struct {
	ID         UpdateID                `json:"id"`
	Hosts      []HostDBEntry           `json:"hosts"`
	Scans      []ScanHistoryEntry      `json:"scans"`
	Benchmarks []BenchmarkHistoryEntry `json:"benchmarks"`
}

// The HostDB is a database of hosts.
type HostDB struct {
	nodes        NodeStore
	stores       map[string]*hostDBStore
	unsubscribes map[string]func()
	log          *zap.Logger
	closeFn      func()

	tg siasync.ThreadGroup
	mu sync.Mutex

	benchmarking     bool
	scanList         []*HostDBEntry
	benchmarkList    []*HostDBEntry
	scanMap          map[types.PublicKey]bool
	scanThreads      int
	benchmarkThreads int
	priceLimits      hostDBPriceLimits
	blockedDomains   *blockedDomains
}

// RecentUpdates returns a list of the most recent updates since the last retrieval.
func (hdb *HostDB) RecentUpdates() (HostUpdates, error) {
	var id UpdateID
	frand.Read(id[:])

	var hosts []HostDBEntry
	var scans []ScanHistoryEntry
	var benchmarks []BenchmarkHistoryEntry

	for _, store := range hdb.stores {
		updates, err := store.getRecentUpdates(id)
		if err != nil {
			return HostUpdates{}, err
		}

		hosts = append(hosts, updates.Hosts...)
		scans = append(scans, updates.Scans...)
		benchmarks = append(benchmarks, updates.Benchmarks...)
	}

	return HostUpdates{
		ID:         id,
		Hosts:      hosts,
		Scans:      scans,
		Benchmarks: benchmarks,
	}, nil
}

// FinalizeUpdates updates the timestamps after the client confirms the data receipt.
func (hdb *HostDB) FinalizeUpdates(id UpdateID) error {
	var err error
	for _, store := range hdb.stores {
		err = utils.ComposeErrors(err, store.finalizeUpdates(id))
	}
	return err
}

// Close shuts down HostDB.
func (hdb *HostDB) Close() {
	if err := hdb.tg.Stop(); err != nil {
		hdb.log.Error("unable to stop threads", zap.Error(err))
	}

	for network := range hdb.stores {
		hdb.unsubscribes[network]()
		hdb.stores[network].close()
	}

	hdb.closeFn()
}

// loadBlockedDomains loads the list of blocked domains.
func loadBlockedDomains(db *sql.DB) (*blockedDomains, error) {
	var domains []string
	rows, err := db.Query("SELECT dom FROM hdb_domains")
	if err != nil {
		return nil, utils.AddContext(err, "couldn't query blocked domains")
	}
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			rows.Close()
			return nil, utils.AddContext(err, "couldn't scan filtered domain")
		}
		domains = append(domains, domain)
	}
	rows.Close()
	return newBlockedDomains(domains), nil
}

func syncStore(store *hostDBStore, cm *chain.Manager, index types.ChainIndex) error {
	for index != cm.Tip() {
		_, caus, err := cm.UpdatesSince(index, 1000)
		if err != nil {
			return fmt.Errorf("failed to subscribe to chain manager: height %d, error %w", cm.Tip().Height, err)
		} else if err := store.updateChainState(caus, len(caus) > 0 && caus[len(caus)-1].State.Index == cm.Tip()); err != nil {
			return fmt.Errorf("failed to update chain state: %w", err)
		}
		if len(caus) > 0 {
			index = caus[len(caus)-1].State.Index
		}
	}
	return nil
}

// NewHostDB returns a new HostDB.
func NewHostDB(db *sql.DB, dir string, nodes NodeStore, networks []string) (*HostDB, <-chan error) {
	errChan := make(chan error, 1)
	l, closeFn, err := persist.NewFileLogger(filepath.Join(dir, "hostdb.log"), zapcore.InfoLevel)
	if err != nil {
		log.Fatal(err)
	}

	domains, err := loadBlockedDomains(db)
	if err != nil {
		errChan <- err
		return nil, errChan
	}

	hdb := &HostDB{
		nodes:        nodes,
		stores:       make(map[string]*hostDBStore),
		unsubscribes: make(map[string]func()),
		log:          l,
		closeFn:      closeFn,
		scanMap:      make(map[types.PublicKey]bool),
		priceLimits: hostDBPriceLimits{
			maxContractPrice: maxContractPrice,
			maxUploadPrice:   maxUploadPriceSC,
			maxDownloadPrice: maxDownloadPriceSC,
			maxStoragePrice:  maxStoragePriceSC,
		},
		blockedDomains: domains,
	}

	for _, network := range networks {
		store, tip, err := newHostDBStore(db, l, network, domains)
		if err != nil {
			errChan <- err
			return nil, errChan
		}

		store.hdb = hdb
		hdb.stores[network] = store

		// Subscribe in a goroutine to prevent blocking.
		go func() {
			for hdb.nodes.ChainManager(network).Tip().Height <= tip.Height {
				time.Sleep(5 * time.Second)
			}
			if err := syncStore(store, hdb.nodes.ChainManager(network), tip); err != nil {
				index, _ := hdb.nodes.ChainManager(network).BestIndex(tip.Height - 1)
				if err := syncStore(store, hdb.nodes.ChainManager(network), index); err != nil {
					l.Fatal("failed to subscribe to chain manager", zap.String("network", network), zap.Error(err))
				}
			}

			reorgChan := make(chan types.ChainIndex, 1)
			hdb.unsubscribes[network] = hdb.nodes.ChainManager(network).OnReorg(func(index types.ChainIndex) {
				select {
				case reorgChan <- index:
				default:
				}
			})

			for range reorgChan {
				lastTip := store.tip
				if err := syncStore(store, hdb.nodes.ChainManager(network), lastTip); err != nil {
					l.Error("failed to sync store", zap.String("network", network), zap.Error(err))
				}
			}
		}()
	}

	// Fetch SC rate.
	go hdb.updateSCRate()

	// Start the scanning thread.
	go hdb.scanHosts()

	// Periodically prune old scans and benchmarks.
	go hdb.pruneOldRecords()

	return hdb, errChan
}

// online returns if the HostDB is online.
func (hdb *HostDB) online(network string) bool {
	return len(hdb.nodes.Syncer(network).Peers()) > 0
}

// updateSCRate periodically fetches the SC exchange rate.
func (hdb *HostDB) updateSCRate() {
	if err := hdb.tg.Add(); err != nil {
		hdb.log.Error("couldn't add thread", zap.Error(err))
		return
	}
	defer hdb.tg.Done()

	for {
		rate, err := external.FetchSCRate()
		if err != nil {
			hdb.log.Error("couldn't fetch SC exchange rate", zap.Error(err))
		}

		if rate != 0 {
			hdb.mu.Lock()
			if hdb.priceLimits.maxUploadPrice.Siacoins()*rate > maxUploadPriceUSD {
				hdb.priceLimits.maxUploadPrice = utils.FromFloat(maxUploadPriceUSD / rate)
			} else {
				hdb.priceLimits.maxUploadPrice = maxUploadPriceSC
			}
			if hdb.priceLimits.maxDownloadPrice.Siacoins()*rate > maxDownloadPriceUSD {
				hdb.priceLimits.maxDownloadPrice = utils.FromFloat(maxDownloadPriceUSD / rate)
			} else {
				hdb.priceLimits.maxDownloadPrice = maxDownloadPriceSC
			}
			if hdb.priceLimits.maxStoragePrice.Mul64(1e12).Mul64(30*144).Siacoins()*rate > maxDownloadPriceUSD {
				hdb.priceLimits.maxStoragePrice = utils.FromFloat(maxStoragePriceUSD / rate).Div64(1e12).Div64(30 * 144)
			} else {
				hdb.priceLimits.maxStoragePrice = maxStoragePriceSC
			}
			hdb.mu.Unlock()
		}

		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(10 * time.Minute):
		}
	}
}

func isSynced(s *syncer.Syncer) bool {
	var count int
	for _, p := range s.Peers() {
		if p.Synced() {
			count++
		}
	}
	return count >= 5
}

// synced returns true if HostDB is synced to the blockchain.
func (hdb *HostDB) synced(network string) bool {
	return isSynced(hdb.nodes.Syncer(network)) && time.Since(hdb.nodes.ChainManager(network).TipState().PrevTimestamps[0]) < 24*time.Hour
}

// pruneOldRecords periodically cleans the database from old scans and benchmarks.
func (hdb *HostDB) pruneOldRecords() {
	if err := hdb.tg.Add(); err != nil {
		hdb.log.Error("couldn't add thread", zap.Error(err))
		return
	}
	defer hdb.tg.Done()

	for {
		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(24 * time.Hour):
		}

		for network, store := range hdb.stores {
			if err := store.pruneOldRecords(); err != nil {
				hdb.log.Error("couldn't prune old records", zap.String("network", network), zap.Error(err))
			}
		}
	}
}

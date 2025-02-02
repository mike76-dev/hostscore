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
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
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
	Settings        rhpv2.HostSettings           `json:"settings,omitempty"`
	V2Settings      rhpv4.HostSettings           `json:"v2Settings,omitempty"`
	PriceTable      rhpv3.HostPriceTable         `json:"priceTable,omitempty"`
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
	ID         int64                `json:"-"`
	Timestamp  time.Time            `json:"timestamp"`
	Success    bool                 `json:"success"`
	Latency    time.Duration        `json:"latency"`
	Error      string               `json:"error"`
	V2         bool                 `json:"v2"`
	Settings   rhpv2.HostSettings   `json:"settings,omitempty"`
	V2Settings rhpv4.HostSettings   `json:"v2Settings,omitempty"`
	PriceTable rhpv3.HostPriceTable `json:"priceTable,omitempty"`
}

// ScanHistory combines the scan history with the host's public key.
type ScanHistory struct {
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

// BenchmarkHistory combines the benchmark history with the host's public key.
type BenchmarkHistory struct {
	HostBenchmark
	PublicKey types.PublicKey `json:"publicKey"`
	Network   string          `json:"network"`
	Node      string          `json:"node"`
}

// UpdateID is the ID of a HostUpdate.
type UpdateID = [8]byte

// HostUpdates represents a batch of updates sent to the client.
type HostUpdates struct {
	ID         UpdateID           `json:"id"`
	Hosts      []HostDBEntry      `json:"hosts"`
	Scans      []ScanHistory      `json:"scans"`
	Benchmarks []BenchmarkHistory `json:"benchmarks"`
}

// The HostDB is a database of hosts.
type HostDB struct {
	nodes          NodeStore
	s              *hostDBStore
	sZen           *hostDBStore
	unsubscribe    func()
	unsubscribeZen func()
	log            *zap.Logger
	closeFn        func()

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

	updates, err := hdb.s.getRecentUpdates(id)
	if err != nil {
		return HostUpdates{}, err
	}

	updatesZen, err := hdb.sZen.getRecentUpdates(id)
	if err != nil {
		return HostUpdates{}, err
	}

	updates.Hosts = append(updates.Hosts, updatesZen.Hosts...)
	updates.Scans = append(updates.Scans, updatesZen.Scans...)
	updates.Benchmarks = append(updates.Benchmarks, updatesZen.Benchmarks...)

	return updates, nil
}

// FinalizeUpdates updates the timestamps after the client confirms the data receipt.
func (hdb *HostDB) FinalizeUpdates(id UpdateID) error {
	return utils.ComposeErrors(hdb.s.finalizeUpdates(id), hdb.sZen.finalizeUpdates(id))
}

// Close shuts down HostDB.
func (hdb *HostDB) Close() {
	if err := hdb.tg.Stop(); err != nil {
		hdb.log.Error("unable to stop threads", zap.Error(err))
	}
	hdb.unsubscribe()
	hdb.unsubscribeZen()
	hdb.s.close()
	hdb.sZen.close()
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
func NewHostDB(db *sql.DB, dir string, nodes NodeStore) (*HostDB, <-chan error) {
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

	store, tip, err := newHostDBStore(db, l, "mainnet", domains)
	if err != nil {
		errChan <- err
		return nil, errChan
	}
	storeZen, tipZen, err := newHostDBStore(db, l, "zen", domains)
	if err != nil {
		errChan <- err
		return nil, errChan
	}

	hdb := &HostDB{
		nodes:   nodes,
		s:       store,
		sZen:    storeZen,
		log:     l,
		closeFn: closeFn,
		scanMap: make(map[types.PublicKey]bool),
		priceLimits: hostDBPriceLimits{
			maxContractPrice:     maxContractPrice,
			maxUploadPrice:       maxUploadPriceSC,
			maxDownloadPrice:     maxDownloadPriceSC,
			maxStoragePrice:      maxStoragePriceSC,
			maxBaseRPCPrice:      maxBaseRPCPriceSC,
			maxSectorAccessPrice: maxSectorAccessPriceSC,
		},
		blockedDomains: domains,
	}
	hdb.s.hdb = hdb
	hdb.sZen.hdb = hdb

	// Subscribe in a goroutine to prevent blocking.
	go func() {
		for hdb.nodes.ChainManager("mainnet").Tip().Height <= tip.Height {
			time.Sleep(5 * time.Second)
		}
		if err := syncStore(hdb.s, hdb.nodes.ChainManager("mainnet"), tip); err != nil {
			index, _ := hdb.nodes.ChainManager("mainnet").BestIndex(tip.Height - 1)
			if err := syncStore(hdb.s, hdb.nodes.ChainManager("mainnet"), index); err != nil {
				l.Fatal("failed to subscribe to chain manager", zap.String("network", "mainnet"), zap.Error(err))
			}
		}

		reorgChan := make(chan types.ChainIndex, 1)
		hdb.unsubscribe = hdb.nodes.ChainManager("mainnet").OnReorg(func(index types.ChainIndex) {
			select {
			case reorgChan <- index:
			default:
			}
		})

		for range reorgChan {
			lastTip := hdb.s.tip
			if err := syncStore(hdb.s, hdb.nodes.ChainManager("mainnet"), lastTip); err != nil {
				l.Error("failed to sync store", zap.String("network", "mainnet"), zap.Error(err))
			}
		}
	}()

	go func() {
		for hdb.nodes.ChainManager("zen").Tip().Height <= tipZen.Height {
			time.Sleep(5 * time.Second)
		}
		if err := syncStore(hdb.sZen, hdb.nodes.ChainManager("zen"), tipZen); err != nil {
			index, _ := hdb.nodes.ChainManager("zen").BestIndex(tipZen.Height - 1)
			if err := syncStore(hdb.sZen, hdb.nodes.ChainManager("zen"), index); err != nil {
				l.Fatal("failed to subscribe to chain manager", zap.String("network", "zen"), zap.Error(err))
			}
		}

		reorgChan := make(chan types.ChainIndex, 1)
		hdb.unsubscribeZen = hdb.nodes.ChainManager("zen").OnReorg(func(index types.ChainIndex) {
			select {
			case reorgChan <- index:
			default:
			}
		})

		for range reorgChan {
			lastTip := hdb.sZen.tip
			if err := syncStore(hdb.sZen, hdb.nodes.ChainManager("zen"), lastTip); err != nil {
				l.Error("failed to sync store", zap.String("network", "zen"), zap.Error(err))
			}
		}
	}()

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
		rates, err := external.FetchSCRates()
		if err != nil {
			hdb.log.Error("couldn't fetch SC exchange rates", zap.Error(err))
		}

		if rates != nil {
			rate := rates["usd"]
			if rate != 0 {
				hdb.mu.Lock()
				if hdb.priceLimits.maxUploadPrice.Siacoins()*rate > maxUploadPriceUSD {
					hdb.priceLimits.maxUploadPrice = utils.FromFloat(maxUploadPriceUSD / rate)
				}
				if hdb.priceLimits.maxDownloadPrice.Siacoins()*rate > maxDownloadPriceUSD {
					hdb.priceLimits.maxDownloadPrice = utils.FromFloat(maxDownloadPriceUSD / rate)
				}
				if hdb.priceLimits.maxStoragePrice.Mul64(1e12).Mul64(30*144).Siacoins()*rate > maxDownloadPriceUSD {
					hdb.priceLimits.maxStoragePrice = utils.FromFloat(maxStoragePriceUSD / rate).Div64(1e12).Div64(30 * 144)
				}
				if hdb.priceLimits.maxBaseRPCPrice.Siacoins()*rate > maxBaseRPCPriceUSD {
					hdb.priceLimits.maxBaseRPCPrice = utils.FromFloat(maxBaseRPCPriceUSD / rate)
				}
				if hdb.priceLimits.maxSectorAccessPrice.Siacoins()*rate > maxSectorAccessPriceUSD {
					hdb.priceLimits.maxSectorAccessPrice = utils.FromFloat(maxSectorAccessPriceUSD / rate)
				}
				hdb.mu.Unlock()
			}
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

		if err := hdb.s.pruneOldRecords(); err != nil {
			hdb.log.Error("couldn't prune old records", zap.String("network", "mainnet"), zap.Error(err))
		}

		if err := hdb.sZen.pruneOldRecords(); err != nil {
			hdb.log.Error("couldn't prune old records", zap.String("network", "zen"), zap.Error(err))
		}
	}
}

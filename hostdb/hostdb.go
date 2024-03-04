package hostdb

import (
	"database/sql"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/external"
	siasync "github.com/mike76-dev/hostscore/internal/sync"
	"github.com/mike76-dev/hostscore/internal/utils"
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
	Network       string                     `json:"network"`
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
	syncer    *syncer.Syncer
	syncerZen *syncer.Syncer
	cm        *chain.Manager
	cmZen     *chain.Manager
	s         *hostDBStore
	sZen      *hostDBStore
	w         *walletutil.Wallet
	log       *persist.Logger

	tg siasync.ThreadGroup
	mu sync.Mutex

	benchmarking         bool
	initialScanLatencies []time.Duration
	scanList             []*HostDBEntry
	benchmarkList        []*HostDBEntry
	scanMap              map[types.PublicKey]bool
	scanThreads          int
	priceLimits          hostDBPriceLimits
	blockedDomains       *blockedDomains
}

// Hosts returns a list of HostDB's hosts.
func (hdb *HostDB) Hosts(network string, all bool, offset, limit int, query string) (hosts []HostDBEntry, more bool) {
	if network == "zen" {
		return hdb.sZen.getHosts(all, offset, limit, query)
	}
	return hdb.s.getHosts(all, offset, limit, query)
}

// Scans returns the host's scan history.
func (hdb *HostDB) Scans(network string, pk types.PublicKey, from, to time.Time) (scans []HostScan, err error) {
	if network == "zen" {
		return hdb.sZen.getScans(pk, from, to)
	}
	return hdb.s.getScans(pk, from, to)
}

// ScanHistory returns the host's scan history.
func (hdb *HostDB) ScanHistory(network string, from, to time.Time) (history []ScanHistory, err error) {
	if network == "zen" {
		return hdb.sZen.getScanHistory(from, to)
	}
	return hdb.s.getScanHistory(from, to)
}

// Benchmarks returns the host's benchmark history.
func (hdb *HostDB) Benchmarks(network string, pk types.PublicKey, from, to time.Time) (benchmarks []HostBenchmark, err error) {
	if network == "zen" {
		return hdb.sZen.getBenchmarks(pk, from, to)
	}
	return hdb.s.getBenchmarks(pk, from, to)
}

// BenchmarkHistory returns the host's benchmark history.
func (hdb *HostDB) BenchmarkHistory(network string, from, to time.Time) (history []BenchmarkHistory, err error) {
	if network == "zen" {
		return hdb.sZen.getBenchmarkHistory(from, to)
	}
	return hdb.s.getBenchmarkHistory(from, to)
}

// Close shuts down HostDB.
func (hdb *HostDB) Close() {
	if err := hdb.tg.Stop(); err != nil {
		hdb.log.Println("[ERROR] unable to stop threads:", err)
	}
	hdb.s.close()
	hdb.sZen.close()
	hdb.log.Close()
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

// NewHostDB returns a new HostDB.
func NewHostDB(db *sql.DB, dir string, cm *chain.Manager, cmZen *chain.Manager, syncer *syncer.Syncer, syncerZen *syncer.Syncer, w *walletutil.Wallet) (*HostDB, <-chan error) {
	errChan := make(chan error, 1)
	l, err := persist.NewFileLogger(filepath.Join(dir, "hostdb.log"))
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

	// Subscribe in a goroutine to prevent blocking.
	go func() {
		defer close(errChan)
		err := cm.AddSubscriber(store, tip)
		if err != nil {
			errChan <- err
		}
		err = cmZen.AddSubscriber(storeZen, tipZen)
		if err != nil {
			errChan <- err
		}
	}()

	hdb := &HostDB{
		syncer:    syncer,
		syncerZen: syncerZen,
		cm:        cm,
		cmZen:     cmZen,
		w:         w,
		s:         store,
		sZen:      storeZen,
		log:       l,
		scanMap:   make(map[types.PublicKey]bool),
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

	// Fetch SC rate.
	go hdb.updateSCRate()

	// Start the scanning thread.
	go hdb.scanHosts()

	// Fetch host locations.
	go hdb.fetchLocations()

	return hdb, errChan
}

// online returns if the HostDB is online.
func (hdb *HostDB) online(network string) bool {
	if network == "zen" {
		return len(hdb.syncerZen.Peers()) > 0
	}
	return len(hdb.syncer.Peers()) > 0
}

func (hdb *HostDB) saveLocation(host *HostDBEntry, info IPInfo) error {

	if host.Network == "zen" {
		return hdb.sZen.updateHostLocation(host, info)
	}
	return hdb.s.updateHostLocation(host, info)
}

// updateSCRate periodically fetches the SC exchange rate.
func (hdb *HostDB) updateSCRate() {
	if err := hdb.tg.Add(); err != nil {
		hdb.log.Println("[ERROR] couldn't add thread", err)
		return
	}
	defer hdb.tg.Done()

	for {
		rates, err := external.FetchSCRates()
		if err != nil {
			hdb.log.Println("[ERROR] couldn't fetch SC exchange rates", err)
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

// synced returns true if HostDB is synced to the blockchain.
func (hdb *HostDB) synced(network string) bool {
	if network == "zen" {
		return hdb.syncerZen.Synced() && time.Since(hdb.cmZen.TipState().PrevTimestamps[0]) < 24*time.Hour
	}
	return hdb.syncer.Synced() && time.Since(hdb.cm.TipState().PrevTimestamps[0]) < 24*time.Hour
}

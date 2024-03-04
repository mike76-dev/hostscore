package hostdb

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/rhp"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
)

const (
	scanInterval      = 30 * time.Minute
	scanCheckInterval = 1 * time.Second
	maxScanThreads    = 100
	minScans          = 25
)

// queueScan will add a host to the queue to be scanned.
func (hdb *HostDB) queueScan(host *HostDBEntry) {
	// If this entry is already in the scan pool, can return immediately.
	hdb.mu.Lock()
	_, exists := hdb.scanMap[host.PublicKey]
	if exists {
		hdb.mu.Unlock()
		return
	}
	// Put the entry in the scan list.
	var interval time.Duration
	if host.Network == "zen" {
		interval = hdb.sZen.calculateScanInterval(host)
	} else {
		interval = hdb.s.calculateScanInterval(host)
	}
	toBenchmark := len(host.ScanHistory) > 0 && time.Since(host.ScanHistory[len(host.ScanHistory)-1].Timestamp) < interval
	hdb.scanMap[host.PublicKey] = toBenchmark
	if toBenchmark {
		hdb.benchmarkList = append(hdb.benchmarkList, host)
	} else {
		hdb.scanList = append(hdb.scanList, host)
	}
	hdb.mu.Unlock()
}

// scanHost will connect to a host and grab the settings and the price
// table as well as adjust the info.
func (hdb *HostDB) scanHost(host *HostDBEntry) {
	// Resolve the host's used subnets and update the timestamp if they
	// changed. We only update the timestamp if resolving the ipNets was
	// successful.
	ipNets, err := utils.LookupIPNets(host.NetAddress)
	if err == nil && !utils.EqualIPNets(ipNets, host.IPNets) {
		host.IPNets = ipNets
		host.LastIPChange = time.Now()
	}

	// Update historic interactions of the host if necessary.
	hdb.mu.Lock()
	hdb.updateHostHistoricInteractions(host)
	hdb.mu.Unlock()

	var settings rhpv2.HostSettings
	var pt rhpv3.HostPriceTable
	var latency time.Duration
	var success bool
	var errMsg string
	var start time.Time
	err = func() error {
		timeout := 2 * time.Minute
		hdb.mu.Lock()
		if len(hdb.initialScanLatencies) > minScans {
			hdb.log.Printf("[ERROR] initialScanLatencies should never be greater than %d\n", minScans)
		}
		if len(hdb.initialScanLatencies) == minScans {
			timeout = hdb.initialScanLatencies[len(hdb.initialScanLatencies)/2]
			timeout *= 5
			if timeout > 2*time.Minute {
				timeout = 2 * time.Minute
			}
		}
		hdb.mu.Unlock()

		// Create a context and set up its cancelling.
		ctx, cancel := context.WithTimeout(context.Background(), timeout+4*time.Minute)
		connCloseChan := make(chan struct{})
		go func() {
			select {
			case <-hdb.tg.StopChan():
			case <-connCloseChan:
			}
			cancel()
		}()
		defer close(connCloseChan)

		// Initiate RHP2 protocol.
		start = time.Now()
		err := rhp.WithTransportV2(ctx, host.NetAddress, host.PublicKey, func(t *rhpv2.Transport) error {
			var err error
			settings, err = rhp.RPCSettings(ctx, t)
			return err
		})
		latency = time.Since(start)
		if err == nil {
			success = true

			// Initiate RHP3 protocol.
			err = rhp.WithTransportV3(ctx, settings.SiamuxAddr(), host.PublicKey, func(t *rhpv3.Transport) error {
				var err error
				pt, err = rhp.RPCPriceTable(ctx, t, func(pt rhpv3.HostPriceTable) (rhpv3.PaymentMethod, error) {
					return nil, nil
				})
				return err
			})
		}

		return err
	}()
	if err != nil && strings.Contains(err.Error(), "canceled") {
		// Shutting down.
		return
	}
	if err == nil {
		hdb.IncrementSuccessfulInteractions(host)
	} else {
		errMsg = err.Error()
		hdb.IncrementFailedInteractions(host)
	}

	scan := HostScan{
		Timestamp:  start,
		Success:    success,
		Latency:    latency,
		Error:      errMsg,
		Settings:   settings,
		PriceTable: pt,
	}

	// Update the host database.
	if host.Network == "zen" {
		err = hdb.sZen.updateScanHistory(host, scan)
	} else {
		err = hdb.s.updateScanHistory(host, scan)
	}
	if err != nil {
		hdb.log.Println("[ERROR] couldn't update scan history:", err)
	}

	// Add the scan to the initialScanLatencies if it was successful.
	if success && len(hdb.initialScanLatencies) < 25 {
		hdb.initialScanLatencies = append(hdb.initialScanLatencies, latency)
		// If the slice has reached its maximum size we sort it.
		if len(hdb.initialScanLatencies) == 25 {
			sort.Slice(hdb.initialScanLatencies, func(i, j int) bool {
				return hdb.initialScanLatencies[i] < hdb.initialScanLatencies[j]
			})
		}
	}

	// Delete the host from scanMap.
	hdb.mu.Lock()
	delete(hdb.scanMap, host.PublicKey)
	hdb.scanThreads--
	hdb.mu.Unlock()
}

// scanHosts is an ongoing function which will scan the full set of hosts
// periodically.
func (hdb *HostDB) scanHosts() {
	if err := hdb.tg.Add(); err != nil {
		hdb.log.Println("[ERROR] couldn't add a thread:", err)
		return
	}
	defer hdb.tg.Done()

	for {
		if hdb.synced("mainnet") || hdb.synced("zen") {
			break
		}
		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(scanCheckInterval):
		}
	}

	for {
		if hdb.synced("mainnet") {
			hdb.s.getHostsForScan()
		}
		if hdb.synced("zen") {
			hdb.sZen.getHostsForScan()
		}

		for len(hdb.scanList) > 0 {
			hdb.mu.Lock()
			if hdb.scanThreads < maxScanThreads {
				hdb.scanThreads++
				entry := hdb.scanList[0]
				hdb.scanList = hdb.scanList[1:]
				go func() {
					if err := hdb.tg.Add(); err != nil {
						hdb.mu.Unlock()
						return
					}
					defer hdb.tg.Done()
					hdb.scanHost(entry)
				}()
			} else {
				hdb.mu.Unlock()
				break
			}
			hdb.mu.Unlock()

		}

		for len(hdb.benchmarkList) > 0 {
			hdb.mu.Lock()
			if !hdb.benchmarking {
				hdb.benchmarking = true
				entry := hdb.benchmarkList[0]
				hdb.benchmarkList = hdb.benchmarkList[1:]
				go func() {
					if err := hdb.tg.Add(); err != nil {
						hdb.mu.Unlock()
						return
					}
					defer hdb.tg.Done()
					hdb.benchmarkHost(entry)
				}()
			} else {
				hdb.mu.Unlock()
				break
			}
			hdb.mu.Unlock()
		}

		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(scanCheckInterval):
		}
	}
}

// calculateScanInterval calculates a scan interval depending on how long ago
// the host was seen online.
func (s *hostDBStore) calculateScanInterval(host *HostDBEntry) time.Duration {
	if host.LastSeen.IsZero() || len(host.ScanHistory) == 0 {
		return scanInterval // 30 minutes
	}

	num := s.lastFailedScans(host)
	if num > 18 && (host.LastSeen.IsZero() || time.Since(host.LastSeen) >= 21*24*time.Hour) {
		return math.MaxInt64 // never
	}
	if num > 15 {
		return scanInterval * 48 // 24 hours
	}
	if num > 11 {
		return scanInterval * 32 // 16 hours
	}
	if num > 9 {
		return scanInterval * 16 // 8 hours
	}
	if num > 7 {
		return scanInterval * 8 // 4 hours
	}
	if num > 5 {
		return scanInterval * 4 // 2 hours
	}
	if num > 3 {
		return scanInterval * 2 // 1 hour
	}
	return math.MaxInt64
}

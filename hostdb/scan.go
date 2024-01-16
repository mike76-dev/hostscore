package hostdb

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/rhp"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
)

const (
	scanBatchSize     = 1000
	scanInterval      = 30 * time.Minute
	scanCheckInterval = 30 * time.Second
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
	hdb.scanMap[host.PublicKey] = struct{}{}
	hdb.mu.Unlock()
	hdb.scanList = append(hdb.scanList, *host)

	// Check if any thread is currently emptying the waitlist. If not, spawn a
	// thread to empty the waitlist.
	if hdb.scanning {
		// Another thread is emptying the scan list, nothing to worry about.
		return
	}

	// Nobody is emptying the scan map, create and run a scan thread.
	hdb.scanning = true
	go func() {
		scanPool := make(chan HostDBEntry)
		defer close(scanPool)

		if hdb.tg.Add() != nil {
			// Hostdb is shutting down, don't spin up another thread.  It is
			// okay to leave scanning set to true as that will not affect
			// shutdown.
			return
		}
		defer hdb.tg.Done()

		// Due to the patterns used to spin up scanning threads, it's possible
		// that we get to this point while all scanning threads are currently
		// used up, completing jobs that were sent out by the previous pool
		// managing thread. This thread is at risk of deadlocking if there's
		// not at least one scanning thread accepting work that it created
		// itself, so we use a starterThread exception and spin up
		// one-thread-too-many on the first iteration to ensure that we do not
		// deadlock.
		starterThread := false
		for {
			// If the scanList is empty, this thread can spin down.
			hdb.mu.Lock()
			if len(hdb.scanList) == 0 {
				// Scan map is empty, can exit. Let the world know that nobody
				// is emptying the scan list anymore.
				hdb.scanning = false
				hdb.mu.Unlock()
				return
			}

			// Get the next host, shrink the scan list.
			entry := hdb.scanList[0]
			hdb.scanList = hdb.scanList[1:]

			// Try to send this entry to an existing idle worker (non-blocking).
			select {
			case scanPool <- entry:
				hdb.mu.Unlock()
				continue
			default:
			}

			// Create new worker thread.
			if hdb.scanThreads < maxScanThreads || !starterThread {
				starterThread = true
				hdb.scanThreads++
				if err := hdb.tg.Add(); err != nil {
					hdb.mu.Unlock()
					return
				}
				go func() {
					defer hdb.tg.Done()
					hdb.probeHosts(scanPool)
					hdb.mu.Lock()
					hdb.scanThreads--
					hdb.mu.Unlock()
				}()
			}
			hdb.mu.Unlock()

			// Block while waiting for an opening in the scan pool.
			select {
			case scanPool <- entry:
				continue
			case <-hdb.tg.StopChan():
				return
			}
		}
	}()
}

// scanHost will connect to a host and grab the settings and the price
// table as well as adjust the info.
func (hdb *HostDB) scanHost(host HostDBEntry) {
	// Resolve the host's used subnets and update the timestamp if they
	// changed. We only update the timestamp if resolving the ipNets was
	// successful.
	ipNets, err := utils.LookupIPNets(host.NetAddress)
	if err == nil && !utils.EqualIPNets(ipNets, host.IPNets) {
		host.IPNets = ipNets
		host.LastIPChange = time.Now()
	}
	if err != nil {
		hdb.log.Println("[ERROR] failed to look up IP nets:", err)
	}

	// Update historic interactions of the host if necessary.
	hdb.mu.Lock()
	hdb.updateHostHistoricInteractions(&host)
	hdb.mu.Unlock()

	var settings rhpv2.HostSettings
	var pt rhpv3.HostPriceTable
	var latency time.Duration
	var errMsg string
	var success bool
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
			if err != nil {
				errMsg = err.Error()
			}
		} else {
			errMsg = err.Error()
		}

		return nil
	}()
	if err != nil && strings.Contains(errMsg, "operation was canceled") {
		// Shutting down.
		return
	}
	if err != nil {
		hdb.log.Printf("[DEBUG] scan of %s failed: %v\n", host.NetAddress, err)
	}

	scan := HostDBScan{
		Timestamp:  start,
		Success:    success,
		Latency:    latency,
		Error:      errMsg,
		Settings:   settings,
		PriceTable: pt,
	}

	// Update the host database.
	err = hdb.s.updateScanHistory(host, scan)
	if err != nil {
		hdb.log.Println("[ERROR] couldn't update scan history:", err)
	}

	// Delete the host from scanMap.
	hdb.mu.Lock()
	delete(hdb.scanMap, host.PublicKey)
	hdb.mu.Unlock()

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
}

// waitForScans is a helper function that blocks until the hostDB's scanList is
// empty.
func (hdb *HostDB) waitForScans() {
	for {
		hdb.mu.Lock()
		length := len(hdb.scanList)
		hdb.mu.Unlock()
		if length == 0 {
			break
		}
		select {
		case <-hdb.tg.StopChan():
		case <-time.After(scanCheckInterval):
		}
	}
}

// probeHosts pulls hosts from the thread pool and runs a scan on them.
func (hdb *HostDB) probeHosts(scanPool <-chan HostDBEntry) {
	for host := range scanPool {
		// Block until hostdb has internet connectivity.
		for {
			if hdb.online() {
				break
			}
			select {
			case <-time.After(time.Second * 30):
				continue
			case <-hdb.tg.StopChan():
				return
			}
		}

		// There appears to be internet connectivity, continue with the scan.
		hdb.scanHost(host)
	}
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
		if hdb.syncer.Synced() {
			break
		}
		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(scanCheckInterval):
		}
	}

	for {
		hdb.s.getHostsForScan()
		hdb.waitForScans()

		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(scanCheckInterval):
		}
	}
}

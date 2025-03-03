package hostdb

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/rhp"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	rhpv4 "go.sia.tech/core/rhp/v4"
	rhpv4utils "go.sia.tech/coreutils/rhp/v4"
	"go.uber.org/zap"
)

const (
	scanInterval        = 30 * time.Minute
	scanBatchSize       = 20
	maxScanThreads      = 1000
	maxBenchmarkThreads = 20
	minScans            = 25
)

// queueScan will add a host to the queue to be scanned.
func (hdb *HostDB) queueScan(host *HostDBEntry) {
	store, ok := hdb.stores[host.Network]
	if !ok {
		panic("wrong host network")
	}

	// If this entry is already in the scan pool, can return immediately.
	hdb.mu.Lock()
	_, exists := hdb.scanMap[host.PublicKey]
	if exists {
		hdb.mu.Unlock()
		return
	}

	// Put the entry in the scan list.
	interval := store.calculateScanInterval(host)
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
	store, ok := hdb.stores[host.Network]
	if !ok {
		panic("wrong host network")
	}

	// Resolve the host's used subnets and update the timestamp if they
	// changed. We only update the timestamp if resolving the ipNets was
	// successful.
	ipNets, err := utils.LookupIPNets(host.NetAddress)
	if err == nil && !utils.EqualIPNets(ipNets, host.IPNets) {
		host.IPNets = ipNets
		host.LastIPChange = time.Now()
	}

	state := hdb.nodes.ChainManager(host.Network).TipState()
	var settings rhpv2.HostSettings
	var pt rhpv3.HostPriceTable
	var v2Settings rhpv4.HostSettings
	var latency time.Duration
	var success bool
	var errMsg string
	var start time.Time
	err = func() error {
		// Create a context and set up its cancelling.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		connCloseChan := make(chan struct{})
		go func() {
			select {
			case <-hdb.tg.StopChan():
			case <-connCloseChan:
			}
			cancel()
		}()
		defer close(connCloseChan)

		start = time.Now()
		var err error
		if host.V2 {
			// Initiate RHP4 protocol.
			err = rhp.WithTransportV4(ctx, host.SiamuxAddresses[0], host.PublicKey, func(t rhpv4utils.TransportClient) error {
				var err error
				v2Settings, err = rhpv4utils.RPCSettings(ctx, t)
				return err
			})
			latency = time.Since(start)
			success = err == nil
		} else if state.Index.Height < state.Network.HardforkV2.RequireHeight {
			// Initiate RHP2 protocol.
			err = rhp.WithTransportV2(ctx, host.NetAddress, host.PublicKey, func(t *rhpv2.Transport) error {
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
		} else {
			return errors.New("V1 hosts not allowed")
		}

		return err
	}()
	if err != nil && strings.Contains(err.Error(), "canceled") {
		// Shutting down.
		return
	}
	if err == nil {
		host.Interactions.Successes++
	} else {
		errMsg = err.Error()
		// If we are offline it probably wasn't the host's fault.
		if hdb.online(host.Network) {
			host.Interactions.Failures++
		}
	}

	scan := HostScan{
		Timestamp:  start,
		Success:    success,
		Latency:    latency,
		Error:      errMsg,
		V2:         host.V2,
		Settings:   settings,
		V2Settings: v2Settings,
		PriceTable: pt,
	}

	// Update the host database.
	if err := store.updateScanHistory(host, scan); err != nil {
		hdb.log.Error("couldn't update scan history", zap.Error(err))
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
		hdb.log.Error("couldn't add a thread", zap.Error(err))
		return
	}
	defer hdb.tg.Done()

	for {
		var synced bool
		for network := range hdb.stores {
			if hdb.synced(network) {
				synced = true
				break
			}
		}
		if synced {
			break
		}
		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(time.Second):
		}
	}

	for {
		for network, store := range hdb.stores {
			if hdb.synced(network) {
				store.getHostsForScan()
			}
		}

		for len(hdb.scanList) > 0 {
			hdb.mu.Lock()
			if hdb.scanThreads < maxScanThreads {
				hdb.scanThreads++
				batchSize := scanBatchSize
				if batchSize > len(hdb.scanList) {
					batchSize = len(hdb.scanList)
				}
				list := hdb.scanList[:batchSize]
				hdb.scanList = hdb.scanList[batchSize:]
				hdb.mu.Unlock()
				go func() {
					for _, entry := range list {
						hdb.scanHost(entry)
					}
				}()
			} else {
				hdb.mu.Unlock()
				break
			}
		}

		for len(hdb.benchmarkList) > 0 {
			hdb.mu.Lock()
			if hdb.benchmarkThreads < maxBenchmarkThreads {
				hdb.benchmarkThreads++
				entry := hdb.benchmarkList[0]
				hdb.benchmarkList = hdb.benchmarkList[1:]
				hdb.mu.Unlock()
				go hdb.benchmarkHost(entry)
			} else {
				hdb.mu.Unlock()
				break
			}
		}

		select {
		case <-hdb.tg.StopChan():
			return
		case <-time.After(5 * time.Second):
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
	if num > 18 && host.LastSeen.IsZero() {
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
	return scanInterval
}

package main

import (
	"math"
	"sync"
	"time"

	"github.com/mike76-dev/hostscore/hostdb"
	"go.sia.tech/core/types"
)

const (
	hostsExpireThreshold      = 10 * time.Minute
	scansExpireThreshold      = 30 * time.Minute
	benchmarksExpireThreshold = 30 * time.Minute

	hostsRequestsLimit      = 100
	scansRequestsLimit      = 1000
	benchmarksRequestsLimit = 1000
)

type cachedHosts struct {
	hosts    []hostdb.HostDBEntry
	more     bool
	total    int
	network  string
	all      bool
	offset   int
	limit    int
	query    string
	modified time.Time
}

type cachedScans struct {
	scans     []hostdb.HostScan
	location  string
	network   string
	publicKey types.PublicKey
	from      time.Time
	to        time.Time
	modified  time.Time
}

type cachedBenchmarks struct {
	benchmarks []hostdb.HostBenchmark
	location   string
	network    string
	publicKey  types.PublicKey
	from       time.Time
	to         time.Time
	modified   time.Time
}

type responseCache struct {
	hosts      []cachedHosts
	scans      []cachedScans
	benchmarks []cachedBenchmarks
	mu         sync.Mutex
	stopChan   chan struct{}
}

func newCache() *responseCache {
	rc := &responseCache{
		stopChan: make(chan struct{}),
	}
	go rc.prune()
	return rc
}

func (rc *responseCache) close() {
	close(rc.stopChan)
}

func (rc *responseCache) prune() {
	for {
		select {
		case <-rc.stopChan:
			return
		case <-time.After(30 * time.Minute):
		}
		rc.mu.Lock()
		var wg sync.WaitGroup
		wg.Add(3)
		go func() {
			i := 0
			for {
				if i >= len(rc.hosts) {
					break
				}
				if time.Since(rc.hosts[i].modified) > hostsExpireThreshold {
					rc.hosts = append(rc.hosts[:i], rc.hosts[i+1:]...)
				} else {
					i++
				}
			}
			wg.Done()
		}()
		go func() {
			i := 0
			for {
				if i >= len(rc.scans) {
					break
				}
				if time.Since(rc.scans[i].modified) > scansExpireThreshold {
					rc.scans = append(rc.scans[:i], rc.scans[i+1:]...)
				} else {
					i++
				}
			}
			wg.Done()
		}()
		go func() {
			i := 0
			for {
				if i >= len(rc.benchmarks) {
					break
				}
				if time.Since(rc.benchmarks[i].modified) > benchmarksExpireThreshold {
					rc.benchmarks = append(rc.benchmarks[:i], rc.benchmarks[i+1:]...)
				} else {
					i++
				}
			}
			wg.Done()
		}()
		wg.Wait()
		rc.mu.Unlock()
	}
}

func (rc *responseCache) getHosts(network string, all bool, offset, limit int, query string) (hosts []hostdb.HostDBEntry, more bool, total int, ok bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for _, ch := range rc.hosts {
		if ch.network == network && ch.all == all && ch.offset == offset && ch.limit == limit && ch.query == query && time.Since(ch.modified) < hostsExpireThreshold {
			hosts = ch.hosts
			more = ch.more
			total = ch.total
			ok = true
			return
		}
	}
	return
}

func (rc *responseCache) putHosts(network string, all bool, offset, limit int, query string, hosts []hostdb.HostDBEntry, more bool, total int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.hosts = append(rc.hosts, cachedHosts{
		hosts:    hosts,
		more:     more,
		total:    total,
		network:  network,
		all:      all,
		offset:   offset,
		limit:    limit,
		query:    query,
		modified: time.Now(),
	})
	if len(rc.hosts) > hostsRequestsLimit {
		rc.hosts = rc.hosts[1:]
	}
}

func (rc *responseCache) getScans(location, network string, pk types.PublicKey, from, to time.Time) (scans []hostdb.HostScan, ok bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for _, cs := range rc.scans {
		if cs.location == location && cs.network == network && cs.publicKey == pk && math.Abs(from.Sub(cs.from).Seconds()) < float64(scansExpireThreshold) && math.Abs(to.Sub(cs.to).Seconds()) < float64(scansExpireThreshold) && time.Since(cs.modified) < scansExpireThreshold {
			scans = cs.scans
			ok = true
			return
		}
	}
	return
}

func (rc *responseCache) putScans(location, network string, pk types.PublicKey, from, to time.Time, scans []hostdb.HostScan) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.scans = append(rc.scans, cachedScans{
		scans:     scans,
		location:  location,
		network:   network,
		publicKey: pk,
		from:      from,
		to:        to,
		modified:  time.Now(),
	})
	if len(rc.scans) > scansRequestsLimit {
		rc.scans = rc.scans[1:]
	}
}

func (rc *responseCache) getBenchmarks(location, network string, pk types.PublicKey, from, to time.Time) (benchmarks []hostdb.HostBenchmark, ok bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for _, cb := range rc.benchmarks {
		if cb.location == location && cb.network == network && cb.publicKey == pk && math.Abs(from.Sub(cb.from).Seconds()) < float64(benchmarksExpireThreshold) && math.Abs(to.Sub(cb.to).Seconds()) < float64(benchmarksExpireThreshold) && time.Since(cb.modified) < benchmarksExpireThreshold {
			benchmarks = cb.benchmarks
			ok = true
			return
		}
	}
	return
}

func (rc *responseCache) putBenchmarks(location, network string, pk types.PublicKey, from, to time.Time, benchmarks []hostdb.HostBenchmark) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.benchmarks = append(rc.benchmarks, cachedBenchmarks{
		benchmarks: benchmarks,
		location:   location,
		network:    network,
		publicKey:  pk,
		from:       from,
		to:         to,
		modified:   time.Now(),
	})
	if len(rc.benchmarks) > benchmarksRequestsLimit {
		rc.benchmarks = rc.benchmarks[1:]
	}
}

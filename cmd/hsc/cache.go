package main

import (
	"sync"
	"time"

	"go.sia.tech/core/types"
)

const (
	hostsExpireThreshold = 10 * time.Minute
	cachedHostsLimit     = 2000
)

type cachedHosts struct {
	hosts    []portalHost
	more     bool
	total    int
	network  string
	all      bool
	offset   int
	limit    int
	query    string
	country  string
	sortBy   sortType
	asc      bool
	modified time.Time
}

type responseCache struct {
	hosts    []cachedHosts
	count    int
	mu       sync.Mutex
	stopChan chan struct{}
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
		i := 0
		rc.count = 0
		for {
			if i >= len(rc.hosts) {
				break
			}
			if time.Since(rc.hosts[i].modified) > hostsExpireThreshold {
				rc.hosts = append(rc.hosts[:i], rc.hosts[i+1:]...)
			} else {
				rc.count += len(rc.hosts[i].hosts)
				i++
			}
		}
		rc.mu.Unlock()
	}
}

func (rc *responseCache) getHost(network string, pk types.PublicKey) (host portalHost, ok bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for _, ch := range rc.hosts {
		if ch.network != network {
			continue
		}
		for _, h := range ch.hosts {
			if h.PublicKey == pk {
				return h, true
			}
		}
	}
	return
}

func (rc *responseCache) getHosts(network string, all bool, offset, limit int, query, country string, sortBy sortType, asc bool) (hosts []portalHost, more bool, total int, ok bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	for _, ch := range rc.hosts {
		if ch.network == network &&
			ch.all == all &&
			ch.offset == offset &&
			ch.limit == limit &&
			ch.query == query &&
			ch.country == country &&
			ch.sortBy == sortBy &&
			ch.asc == asc &&
			time.Since(ch.modified) < hostsExpireThreshold {
			hosts = ch.hosts
			more = ch.more
			total = ch.total
			ok = true
			return
		}
	}
	return
}

func (rc *responseCache) putHosts(network string, all bool, offset, limit int, query, country string, sortBy sortType, asc bool, hosts []portalHost, more bool, total int) {
	if len(hosts) > cachedHostsLimit {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.count += len(hosts)
	for rc.count > cachedHostsLimit {
		rc.count -= len(rc.hosts[0].hosts)
		if len(rc.hosts) > 0 {
			rc.hosts = rc.hosts[1:]
		} else {
			rc.hosts = nil
		}
	}
	rc.hosts = append(rc.hosts, cachedHosts{
		hosts:    hosts,
		more:     more,
		total:    total,
		network:  network,
		all:      all,
		offset:   offset,
		limit:    limit,
		query:    query,
		country:  country,
		sortBy:   sortBy,
		asc:      asc,
		modified: time.Now(),
	})
}

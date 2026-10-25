package main

import (
	"testing"
	"time"

	"go.sia.tech/core/types"
)

func TestResponseCacheGetHosts(t *testing.T) {
	rc := newCache()
	defer rc.close()

	hosts := []portalHost{{ID: 1}, {ID: 2}}
	rc.putHosts("mainnet", true, 0, 10, "", "", noSort, true, hosts, true, 42)

	got, more, total, ok := rc.getHosts("mainnet", true, 0, 10, "", "", noSort, true)
	if !ok {
		t.Fatal("expected a cache hit")
	}
	if len(got) != 2 || !more || total != 42 {
		t.Errorf("got %d hosts, more %v, total %d; want 2, true, 42", len(got), more, total)
	}

	// Any differing query parameter must miss.
	misses := []struct {
		name string
		call func() bool
	}{
		{"network", func() bool { _, _, _, ok := rc.getHosts("zen", true, 0, 10, "", "", noSort, true); return ok }},
		{"all", func() bool { _, _, _, ok := rc.getHosts("mainnet", false, 0, 10, "", "", noSort, true); return ok }},
		{"offset", func() bool { _, _, _, ok := rc.getHosts("mainnet", true, 10, 10, "", "", noSort, true); return ok }},
		{"limit", func() bool { _, _, _, ok := rc.getHosts("mainnet", true, 0, 20, "", "", noSort, true); return ok }},
		{"query", func() bool { _, _, _, ok := rc.getHosts("mainnet", true, 0, 10, "q", "", noSort, true); return ok }},
		{"country", func() bool { _, _, _, ok := rc.getHosts("mainnet", true, 0, 10, "", "DE", noSort, true); return ok }},
		{"sortBy", func() bool { _, _, _, ok := rc.getHosts("mainnet", true, 0, 10, "", "", sortByRank, true); return ok }},
		{"asc", func() bool { _, _, _, ok := rc.getHosts("mainnet", true, 0, 10, "", "", noSort, false); return ok }},
	}
	for _, tt := range misses {
		if tt.call() {
			t.Errorf("different %s: expected a cache miss", tt.name)
		}
	}
}

func TestResponseCacheExpiry(t *testing.T) {
	rc := newCache()
	defer rc.close()

	rc.putHosts("mainnet", true, 0, 10, "", "", noSort, true, []portalHost{{ID: 1}}, false, 1)

	// Age the entry past the expiry threshold.
	rc.mu.Lock()
	rc.hosts[0].modified = time.Now().Add(-hostsExpireThreshold - time.Minute)
	rc.mu.Unlock()

	if _, _, _, ok := rc.getHosts("mainnet", true, 0, 10, "", "", noSort, true); ok {
		t.Error("expected an expired entry to miss")
	}
}

func TestResponseCacheGetHost(t *testing.T) {
	rc := newCache()
	defer rc.close()

	pk := types.PublicKey{1, 2, 3}
	rc.putHosts("mainnet", true, 0, 10, "", "", noSort, true, []portalHost{{ID: 7, PublicKey: pk}}, false, 1)

	host, ok := rc.getHost("mainnet", pk)
	if !ok || host.ID != 7 {
		t.Errorf("getHost = (%v, %v), want host 7", host.ID, ok)
	}
	if _, ok := rc.getHost("zen", pk); ok {
		t.Error("expected a miss on another network")
	}
	if _, ok := rc.getHost("mainnet", types.PublicKey{9}); ok {
		t.Error("expected a miss on an unknown key")
	}
}

func TestResponseCacheEviction(t *testing.T) {
	rc := newCache()
	defer rc.close()

	// An oversized response is not cached at all.
	oversized := make([]portalHost, cachedHostsLimit+1)
	rc.putHosts("mainnet", true, 0, 10, "", "", noSort, true, oversized, false, len(oversized))
	if _, _, _, ok := rc.getHosts("mainnet", true, 0, 10, "", "", noSort, true); ok {
		t.Fatal("expected an oversized response not to be cached")
	}

	// Two entries beyond the total limit together: the older one is evicted.
	batch := make([]portalHost, cachedHostsLimit*3/4)
	rc.putHosts("mainnet", true, 0, 10, "", "", noSort, true, batch, false, len(batch))
	rc.putHosts("zen", true, 0, 10, "", "", noSort, true, batch, false, len(batch))

	if _, _, _, ok := rc.getHosts("mainnet", true, 0, 10, "", "", noSort, true); ok {
		t.Error("expected the older entry to be evicted")
	}
	if _, _, _, ok := rc.getHosts("zen", true, 0, 10, "", "", noSort, true); !ok {
		t.Error("expected the newer entry to survive")
	}
}

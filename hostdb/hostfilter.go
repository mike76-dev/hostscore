package hostdb

import (
	"net"
	"strings"
	"sync"
)

type blockedDomains struct {
	domains map[string]struct{}
	mu      sync.Mutex
}

func newBlockedDomains(domains []string) *blockedDomains {
	blocked := &blockedDomains{
		domains: make(map[string]struct{}),
	}
	blocked.addDomains(domains)
	return blocked
}

func (bd *blockedDomains) addDomains(domains []string) {
	bd.mu.Lock()
	defer bd.mu.Unlock()
	for _, domain := range domains {
		bd.domains[domain] = struct{}{}

		addrs, err := net.LookupHost(domain)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			bd.domains[addr] = struct{}{}
		}
	}
}

func (bd *blockedDomains) isBlocked(addr string) bool {
	bd.mu.Lock()
	defer bd.mu.Unlock()

	// See if this specific host:port combination was blocked.
	if _, blocked := bd.domains[addr]; blocked {
		return true
	}

	// Now check if this host was blocked.
	hostname, _, _ := net.SplitHostPort(addr)
	_, blocked := bd.domains[hostname]
	if blocked {
		return true
	}

	ip := net.ParseIP(hostname)
	if ip != nil {
		for domain := range bd.domains {
			_, ipnet, _ := net.ParseCIDR(domain)
			if ipnet != nil && ipnet.Contains(ip) {
				return true
			}
		}
	}

	// Check for subdomains being blocked by a root domain.
	//
	// Split the hostname into elements.
	elements := strings.Split(hostname, ".")
	if len(elements) <= 1 {
		return blocked
	}

	// Check domains.
	//
	// We want to stop at the second last element so that the last domain
	// we check is of the format domain.com. This is to protect the user
	// from accidentally submitting `com`, or some other TLD, and blocking
	// every host in the HostDB.
	for i := 0; i < len(elements)-1; i++ {
		domainToCheck := strings.Join(elements[i:], ".")
		if _, blocked := bd.domains[domainToCheck]; blocked {
			return true
		}
	}
	return false
}

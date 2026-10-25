package hostdb

import "testing"

func TestBlockedDomains(t *testing.T) {
	// Only IPs, CIDRs and unresolvable names are used, so the DNS lookup in
	// addDomains never adds extra entries.
	domains := newBlockedDomains([]string{
		"evilhost.invalid",
		"8.8.8.8",
		"1.2.3.4:9981",
		"10.0.0.0/8",
	})

	blocked := []string{
		"evilhost.invalid:9981",          // blocked domain
		"sub.evilhost.invalid:9981",      // subdomain of a blocked domain
		"deep.sub.evilhost.invalid:9981", // deeper subdomain
		"8.8.8.8:9981",                   // blocked IP, any port
		"8.8.8.8:1234",
		"1.2.3.4:9981",           // blocked host:port combination
		"10.1.2.3:9981",          // inside a blocked subnet
		"10.255.0.1:9981",        // still inside 10.0.0.0/8
		"[::ffff:10.0.0.1]:9981", // IPv4-mapped address inside the subnet
	}
	for _, addr := range blocked {
		if !domains.isBlocked(addr) {
			t.Errorf("isBlocked(%q) = false, want true", addr)
		}
	}

	allowed := []string{
		"goodhost.invalid:9981",
		"notevilhost.invalid:9981", // suffix of the name, not a subdomain
		"8.8.4.4:9981",
		"1.2.3.4:9982",  // same host, different port than the blocked combination
		"11.0.0.1:9981", // outside the blocked subnet
		"invalid:9981",  // bare TLD must not match the blocked domain's TLD
	}
	for _, addr := range allowed {
		if domains.isBlocked(addr) {
			t.Errorf("isBlocked(%q) = true, want false", addr)
		}
	}
}

func TestBlockedDomainsAddDomains(t *testing.T) {
	domains := newBlockedDomains(nil)
	addr := "badhost.invalid:9981"
	if domains.isBlocked(addr) {
		t.Fatalf("isBlocked(%q) = true before adding", addr)
	}
	domains.addDomains([]string{"badhost.invalid"})
	if !domains.isBlocked(addr) {
		t.Errorf("isBlocked(%q) = false after adding", addr)
	}
}

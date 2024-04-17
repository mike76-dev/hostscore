package utils

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// IsLoopback returns true for IP addresses that are on the same machine.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// IsLocal returns true if the input IP address belongs to a local address
// range such as 192.168.x.x or 127.x.x.x.
func IsLocal(addr string) bool {
	// Loopback counts as private.
	if IsLoopback(addr) {
		return true
	}

	// Grab the IP address of the net address. If there is an error parsing,
	// return false, as it's not a private ip address range.
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = ""
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	// Determine whether or not the ip is in a CIDR that is considered to be
	// local.
	localCIDRs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",
		"169.254.0.0/16",
		"fd00::/8",
	}
	for _, cidr := range localCIDRs {
		_, ipnet, _ := net.ParseCIDR(cidr)
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

// IsValid is an extension to IsStdValid that also forbids the loopback address.
//
// NOTE: IsValid is being phased out in favor of allowing the loopback address
// but verifying through other means that the connection is not to yourself
// (which is the original reason that the loopback address was banned).
func IsValid(addr string) error {
	// Check the loopback address.
	if IsLoopback(addr) {
		return errors.New("host is a loopback address")
	}
	return IsStdValid(addr)
}

// IsStdValid returns an error if the NetAddress is invalid. A valid NetAddress
// is of the form "host:port", such that "host" is either a valid IPv4/IPv6
// address or a valid hostname, and "port" is an integer in the range [1,65535].
// Valid IPv4 addresses, IPv6 addresses, and hostnames are detailed in RFCs 791,
// 2460, and 952, respectively. Loopback addresses are allowed.
func IsStdValid(addr string) error {
	// Verify the port number.
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return errors.New("port is not an integer")
	} else if portInt < 1 || portInt > 65535 {
		return errors.New("port is invalid")
	}

	// Loopback addresses don't always pass the requirements below, and
	// therefore must be checked separately.
	if IsLoopback(addr) {
		return nil
	}

	// First try to parse host as an IP address; if that fails, assume it is a
	// hostname.
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			return errors.New("host is the unspecified address")
		}
	} else {
		// Hostnames can have a trailing dot (which indicates that the hostname is
		// fully qualified), but we ignore it for validation purposes.
		host = strings.TrimSuffix(host, ".")
		if len(host) < 1 || len(host) > 253 {
			return errors.New("invalid hostname length")
		}
		labels := strings.Split(host, ".")
		if len(labels) == 1 {
			return errors.New("unqualified hostname")
		}
		for _, label := range labels {
			if len(label) < 1 || len(label) > 63 {
				return errors.New("hostname contains label with invalid length")
			}
			if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return errors.New("hostname contains label that starts or ends with a hyphen")
			}
			for _, r := range strings.ToLower(label) {
				isLetter := 'a' <= r && r <= 'z'
				isNumber := '0' <= r && r <= '9'
				isHyphen := r == '-'
				if !(isLetter || isNumber || isHyphen) {
					return errors.New("host contains invalid characters")
				}
			}
		}
	}

	return nil
}

// LookupIPNets returns string representations of the CIDR subnets.
func LookupIPNets(addr string) (ipNets []string, err error) {
	// Lookup the IP addresses.
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = ""
	}
	addresses, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}

	// Get the subnets of the addresses.
	for _, ip := range addresses {
		// Set the filterRange according to the type of IP address.
		var filterRange int
		if ip.To4() != nil {
			filterRange = 24
		} else {
			filterRange = 54
		}

		// Get the subnet.
		_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), filterRange))
		if err != nil {
			return nil, err
		}
		// Add the subnet to the host.
		ipNets = append(ipNets, ipnet.String())
	}
	return
}

// EqualIPNets checks if two slices of IP subnets contain the same subnets.
func EqualIPNets(ipNetsA, ipNetsB []string) bool {
	// Check the length first.
	if len(ipNetsA) != len(ipNetsB) {
		return false
	}

	// Create a map of all the subnets in ipNetsA.
	mapNetsA := make(map[string]struct{})
	for _, subnet := range ipNetsA {
		mapNetsA[subnet] = struct{}{}
	}

	// Make sure that all the subnets from ipNetsB are in the map.
	for _, subnet := range ipNetsB {
		if _, exists := mapNetsA[subnet]; !exists {
			return false
		}
	}
	return true
}

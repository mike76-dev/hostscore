package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/hostdb"
	"go.sia.tech/core/types"
	"go.sia.tech/jape"
)

type client struct {
	c jape.Client
}

func newClient(addr, password string) *client {
	return &client{c: jape.Client{
		BaseURL:  addr,
		Password: password,
	}}
}

func (c *client) hosts(offset, limit int) (hosts []hostdb.HostDBEntry, err error) {
	err = c.c.GET(fmt.Sprintf("/hostdb/hosts?offset=%d&limit=%d", offset, limit), &hosts)
	return
}

func (c *client) scans(pk types.PublicKey, from, to time.Time) (scans []hostdb.HostScan, err error) {
	err = c.c.GET(fmt.Sprintf("/hostdb/scans?host=%s&from=%v&to=%v", pk, encodeTime(from), encodeTime(to)), &scans)
	return
}

func (c *client) benchmarks(pk types.PublicKey, from, to time.Time) (benchmarks []hostdb.HostBenchmark, err error) {
	err = c.c.GET(fmt.Sprintf("/hostdb/benchmarks?host=%s&from=%v&to=%v", pk, encodeTime(from), encodeTime(to)), &benchmarks)
	return
}

func encodeTime(t time.Time) string {
	b, err := t.MarshalText()
	if err != nil {
		return ""
	}
	return strings.Replace(string(b), "+", "%2B", 1)
}

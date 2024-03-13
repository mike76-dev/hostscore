package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/jape"
)

// A Client provides methods for interacting with a hsd API server.
type Client struct {
	c jape.Client
}

// TxpoolTransactions returns all transactions in the transaction pool.
func (c *Client) TxpoolTransactions(network string) (txns []types.Transaction, v2txns []types.V2Transaction, err error) {
	var resp TxpoolTransactionsResponse
	err = c.c.GET("/txpool/transactions?network="+network, &resp)
	return resp.Transactions, resp.V2Transactions, err
}

// TxpoolFee returns the recommended fee (per weight unit) to ensure a high
// probability of inclusion in the next block.
func (c *Client) TxpoolFee(network string) (resp types.Currency, err error) {
	err = c.c.GET("/txpool/fee?network="+network, &resp)
	return
}

// ConsensusNetwork returns the node's network metadata.
func (c *Client) ConsensusNetwork(network string) (resp *consensus.Network, err error) {
	resp = new(consensus.Network)
	err = c.c.GET("/consensus/network?network="+network, resp)
	return
}

// ConsensusTip returns the current tip index.
func (c *Client) ConsensusTip(network string) (resp ConsensusTipResponse, err error) {
	err = c.c.GET("/consensus/tip?network="+network, &resp)
	return
}

// ConsensusTipState returns the current tip state.
func (c *Client) ConsensusTipState(network string) (resp consensus.State, err error) {
	err = c.c.GET("/consensus/tipstate?network="+network, &resp)
	if err != nil {
		return
	}
	resp.Network, err = c.ConsensusNetwork(network)
	return
}

// SyncerPeers returns the current peers of the syncer.
func (c *Client) SyncerPeers(network string) (resp []GatewayPeer, err error) {
	err = c.c.GET("/syncer/peers?network="+network, &resp)
	return
}

// Address returns the address controlled by the wallet.
func (c *Client) Address(network string) (resp types.Address, err error) {
	err = c.c.GET("/wallet/address?network="+network, &resp)
	return
}

// Balance returns the wallet balance.
func (c *Client) Balance(network string) (resp WalletBalanceResponse, err error) {
	err = c.c.GET("/wallet/balance?network="+network, &resp)
	return
}

// PoolTransactions returns all txpool transactions relevant to the wallet.
func (c *Client) PoolTransactions(network string) (resp []wallet.PoolTransaction, err error) {
	err = c.c.GET("/wallet/txpool?network="+network, &resp)
	return
}

// Outputs returns the set of unspent outputs controlled by the wallet.
func (c *Client) Outputs(network string) (sc []types.SiacoinElement, sf []types.SiafundElement, err error) {
	var resp WalletOutputsResponse
	err = c.c.GET("/wallet/outputs?network="+network, &resp)
	return resp.SiacoinOutputs, resp.SiafundOutputs, err
}

// Hosts returns a list of HostDB hosts.
func (c *Client) Hosts(network string, all bool, offset, limit int, query string) (resp HostdbHostsResponse, err error) {
	var allHosts string
	if all {
		allHosts = "true"
	} else {
		allHosts = "false"
	}
	err = c.c.GET(fmt.Sprintf("/hostdb/hosts?network=%s&all=%s&offset=%d&limit=%d&query=%s", network, allHosts, offset, limit, query), &resp)
	return
}

// Host returns the information about a particular host.
func (c *Client) Host(network string, pk types.PublicKey) (host hostdb.HostDBEntry, err error) {
	err = c.c.GET(fmt.Sprintf("/hostdb/host?network=%s&host=%s", network, pk), &host)
	return
}

// Scans returns a list of host scans.
func (c *Client) Scans(network string, pk types.PublicKey, from, to time.Time) (scans []hostdb.HostScan, err error) {
	err = c.c.GET(fmt.Sprintf("/hostdb/scans?network=%s&host=%s&from=%v&to=%v", network, pk, encodeTime(from), encodeTime(to)), &scans)
	return
}

// Benchmarks returns a list of host benchmarks.
func (c *Client) Benchmarks(network string, pk types.PublicKey, from, to time.Time) (benchmarks []hostdb.HostBenchmark, err error) {
	err = c.c.GET(fmt.Sprintf("/hostdb/benchmarks?network=%s&host=%s&from=%v&to=%v", network, pk, encodeTime(from), encodeTime(to)), &benchmarks)
	return
}

func encodeTime(t time.Time) string {
	b, err := t.MarshalText()
	if err != nil {
		return ""
	}
	return strings.Replace(string(b), "+", "%2B", 1)
}

// NewClient returns a client that communicates with a hsd server listening
// on the specified address.
func NewClient(addr, password string) *Client {
	return &Client{c: jape.Client{
		BaseURL:  addr,
		Password: password,
	}}
}

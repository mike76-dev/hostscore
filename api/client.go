package api

import (
	"fmt"

	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/jape"
)

// A Client provides methods for interacting with a hsd API server.
type Client struct {
	c jape.Client
	n *consensus.Network // for ConsensusTipState
}

// TxpoolTransactions returns all transactions in the transaction pool.
func (c *Client) TxpoolTransactions() (txns []types.Transaction, v2txns []types.V2Transaction, err error) {
	var resp TxpoolTransactionsResponse
	err = c.c.GET("/txpool/transactions", &resp)
	return resp.Transactions, resp.V2Transactions, err
}

// TxpoolFee returns the recommended fee (per weight unit) to ensure a high
// probability of inclusion in the next block.
func (c *Client) TxpoolFee() (resp types.Currency, err error) {
	err = c.c.GET("/txpool/fee", &resp)
	return
}

// ConsensusNetwork returns the node's network metadata.
func (c *Client) ConsensusNetwork() (resp *consensus.Network, err error) {
	resp = new(consensus.Network)
	err = c.c.GET("/consensus/network", resp)
	return
}

// ConsensusTip returns the current tip index.
func (c *Client) ConsensusTip() (resp ConsensusTipResponse, err error) {
	err = c.c.GET("/consensus/tip", &resp)
	return
}

// ConsensusTipState returns the current tip state.
func (c *Client) ConsensusTipState() (resp consensus.State, err error) {
	if c.n == nil {
		c.n, err = c.ConsensusNetwork()
		if err != nil {
			return
		}
	}
	err = c.c.GET("/consensus/tipstate", &resp)
	resp.Network = c.n
	return
}

// SyncerPeers returns the current peers of the syncer.
func (c *Client) SyncerPeers() (resp []GatewayPeer, err error) {
	err = c.c.GET("/syncer/peers", &resp)
	return
}

// Address returns the address controlled by the wallet.
func (c *Client) Address() (resp types.Address, err error) {
	err = c.c.GET("/wallet/address", &resp)
	return
}

// Balance returns the wallet balance.
func (c *Client) Balance() (resp WalletBalanceResponse, err error) {
	err = c.c.GET("/wallet/balance", &resp)
	return
}

// PoolTransactions returns all txpool transactions relevant to the wallet.
func (c *Client) PoolTransactions() (resp []wallet.PoolTransaction, err error) {
	err = c.c.GET("/wallet/txpool", &resp)
	return
}

// Outputs returns the set of unspent outputs controlled by the wallet.
func (c *Client) Outputs() (sc []types.SiacoinElement, sf []types.SiafundElement, err error) {
	var resp WalletOutputsResponse
	err = c.c.GET("/wallet/outputs", &resp)
	return resp.SiacoinOutputs, resp.SiafundOutputs, err
}

// Hosts returns a list of HostDB hosts.
func (c *Client) Hosts(offset, limit int) (resp []hostdb.HostDBEntry, err error) {
	err = c.c.GET(fmt.Sprintf("/hostdb/hosts?offset=%d&limit=%d", offset, limit), &resp)
	return
}

// NewClient returns a client that communicates with a hsd server listening
// on the specified address.
func NewClient(addr, password string) *Client {
	return &Client{c: jape.Client{
		BaseURL:  addr,
		Password: password,
	}}
}

package api

import (
	"encoding/hex"

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

// NodeStatus returns the status of the node.
func (c *Client) NodeStatus() (resp NodeStatusResponse, err error) {
	err = c.c.GET("/node/status", &resp)
	return
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

// Updates returns a list of most recent HostDB updates.
func (c *Client) Updates() (resp hostdb.HostUpdates, err error) {
	err = c.c.GET("/hostdb/updates", &resp)
	return
}

// FinalizeUpdates confirms the receipt of the HostDB updates.
func (c *Client) FinalizeUpdates(id hostdb.UpdateID) error {
	return c.c.GET("/hostdb/updates/confirm?id="+hex.EncodeToString(id[:]), nil)
}

// NewClient returns a client that communicates with a hsd server listening
// on the specified address.
func NewClient(addr, password string) *Client {
	return &Client{c: jape.Client{
		BaseURL:  addr,
		Password: password,
	}}
}

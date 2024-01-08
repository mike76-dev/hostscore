package api

import (
	"fmt"
	"time"

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

// TxpoolBroadcast broadcasts a set of transaction to the network.
func (c *Client) TxpoolBroadcast(txns []types.Transaction, v2txns []types.V2Transaction) (err error) {
	err = c.c.POST("/txpool/broadcast", TxpoolBroadcastRequest{txns, v2txns}, nil)
	return
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
func (c *Client) ConsensusTip() (resp types.ChainIndex, err error) {
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

// SyncerConnect adds the address as a peer of the syncer.
func (c *Client) SyncerConnect(addr string) (err error) {
	err = c.c.POST("/syncer/connect", addr, nil)
	return
}

// SyncerBroadcastBlock broadcasts a block to all peers.
func (c *Client) SyncerBroadcastBlock(b types.Block) (err error) {
	err = c.c.POST("/syncer/broadcast/block", b, nil)
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

// Events returns all events relevant to the wallet.
func (c *Client) Events(offset, limit int) (resp []wallet.Event, err error) {
	err = c.c.GET(fmt.Sprintf("/wallet/events?offset=%d&limit=%d", offset, limit), &resp)
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

// Reserve reserves a set outputs for use in a transaction.
func (c *Client) Reserve(sc []types.SiacoinOutputID, sf []types.SiafundOutputID, duration time.Duration) (err error) {
	err = c.c.POST("/wallet/reserve", WalletReserveRequest{
		SiacoinOutputs: sc,
		SiafundOutputs: sf,
		Duration:       duration,
	}, nil)
	return
}

// Release releases a set of previously-reserved outputs.
func (c *Client) Release(sc []types.SiacoinOutputID, sf []types.SiafundOutputID) (err error) {
	err = c.c.POST("/wallet/release", WalletReleaseRequest{
		SiacoinOutputs: sc,
		SiafundOutputs: sf,
	}, nil)
	return
}

// Fund funds a siacoin transaction.
func (c *Client) Fund(txn types.Transaction, amount types.Currency, changeAddr types.Address) (resp WalletFundResponse, err error) {
	err = c.c.POST("/wallet/fund", WalletFundRequest{
		Transaction:   txn,
		Amount:        amount,
		ChangeAddress: changeAddr,
	}, &resp)
	return
}

// FundSF funds a siafund transaction.
func (c *Client) FundSF(txn types.Transaction, amount uint64, changeAddr, claimAddr types.Address) (resp WalletFundResponse, err error) {
	err = c.c.POST("/wallet/fundsf", WalletFundSFRequest{
		Transaction:   txn,
		Amount:        amount,
		ChangeAddress: changeAddr,
		ClaimAddress:  claimAddr,
	}, &resp)
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

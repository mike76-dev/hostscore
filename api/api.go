package api

import (
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/wallet"
)

// A GatewayPeer is a currently connected peer.
type GatewayPeer struct {
	Addr    string `json:"addr"`
	Inbound bool   `json:"inbound"`
	Version string `json:"version"`
}

// NetworkStatus describes the current status of the network.
type NetworkStatus struct {
	Network string         `json:"network"`
	Height  uint64         `json:"height"`
	Balance wallet.Balance `json:"balance"`
}

// NodeStatusResponse is the response type for /node/status.
type NodeStatusResponse struct {
	Version  string          `json:"version"`
	Networks []NetworkStatus `json:"networks"`
}

// ConsensusTipResponse is the response type for /consensus/tip.
type ConsensusTipResponse struct {
	Height  uint64        `json:"height"`
	BlockID types.BlockID `json:"id"`
	Synced  bool          `json:"synced"`
}

// TxpoolTransactionsResponse is the response type for /txpool/transactions.
type TxpoolTransactionsResponse struct {
	Transactions   []types.Transaction   `json:"transactions"`
	V2Transactions []types.V2Transaction `json:"v2transactions"`
}

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

// NodeStatusResponse is the response type for /node/status.
type NodeStatusResponse struct {
	Version    string         `json:"version"`
	Height     uint64         `json:"heightMainnet"`
	HeightZen  uint64         `json:"heightZen"`
	Balance    wallet.Balance `json:"balanceMainnet"`
	BalanceZen wallet.Balance `json:"balanceZen"`
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

package api

import (
	"go.sia.tech/core/types"
)

// A GatewayPeer is a currently-connected peer.
type GatewayPeer struct {
	Addr    string `json:"addr"`
	Inbound bool   `json:"inbound"`
	Version string `json:"version"`
}

// Balance combines mature and immature values.
type Balance struct {
	Siacoins         types.Currency `json:"siacoins"`
	ImmatureSiacoins types.Currency `json:"immatureSiacoins"`
}

// NodeStatusResponse is the response type for /node/status.
type NodeStatusResponse struct {
	Version    string  `json:"version"`
	Height     uint64  `json:"heightMainnet"`
	HeightZen  uint64  `json:"heightZen"`
	Balance    Balance `json:"balanceMainnet"`
	BalanceZen Balance `json:"balanceZen"`
}

// ConsensusTipResponse is the response type for /consensus/tip.
type ConsensusTipResponse struct {
	Network string        `json:"network"`
	Height  uint64        `json:"height"`
	BlockID types.BlockID `json:"id"`
	Synced  bool          `json:"synced"`
}

// TxpoolTransactionsResponse is the response type for /txpool/transactions.
type TxpoolTransactionsResponse struct {
	Transactions   []types.Transaction   `json:"transactions"`
	V2Transactions []types.V2Transaction `json:"v2transactions"`
}

// WalletBalanceResponse is the response type for /wallet/balance.
type WalletBalanceResponse struct {
	Network          string         `json:"network"`
	Siacoins         types.Currency `json:"siacoins"`
	ImmatureSiacoins types.Currency `json:"immatureSiacoins"`
	Siafunds         uint64         `json:"siafunds"`
}

// WalletOutputsResponse is the response type for /wallet/outputs.
type WalletOutputsResponse struct {
	Network        string                 `json:"network"`
	SiacoinOutputs []types.SiacoinElement `json:"siacoinOutputs"`
	SiafundOutputs []types.SiafundElement `json:"siafundOutputs"`
}

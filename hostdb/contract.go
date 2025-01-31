package hostdb

import (
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/wallet"
)

// calculateFunding calculates the funding of a benchmarking contract.
func calculateFunding(settings rhpv2.HostSettings, txnFee types.Currency) (funding, collateral types.Currency) {
	contractCost := settings.ContractPrice
	downloadCost := settings.DownloadBandwidthPrice
	uploadCost := settings.UploadBandwidthPrice
	storageCost := settings.StoragePrice

	numBenchmarks := contractDuration / (6 * benchmarkInterval / time.Hour)
	dataSize := benchmarkBatchSize * numBenchmarks

	downloadCost = downloadCost.Mul64(uint64(dataSize))
	uploadCost = uploadCost.Mul64(uint64(dataSize))
	storageCost = storageCost.Mul64(uint64(dataSize)).Mul64(contractDuration)

	contractCost = contractCost.Add(txnFee)
	funding = contractCost.Add(downloadCost)
	funding = funding.Add(uploadCost)
	funding = funding.Add(storageCost)

	collateral = rhpv2.ContractFormationCollateral(contractDuration, uint64(dataSize), settings)

	return
}

// calculateFundingV2 calculates the funding of a V2 benchmarking contract.
func calculateFundingV2(prices rhpv4.HostPrices, txnFee types.Currency) (funding, collateral types.Currency) {
	numBenchmarks := contractDuration / (6 * benchmarkInterval / time.Hour)
	dataSize := benchmarkBatchSize * numBenchmarks

	uploadCost := prices.RPCAppendSectorsCost(uint64(dataSize), contractDuration)
	downloadCost := prices.RPCReadSectorCost(uint64(dataSize))

	contractCost := prices.ContractPrice.Add(txnFee)
	funding = uploadCost.Add(downloadCost).RenterCost()
	funding = funding.Add(contractCost)

	collateral = uploadCost.HostRiskedCollateral()

	return
}

// prepareContractFormation creates a new contract and a formation
// transaction set.
func (hdb *HostDB) prepareContractFormation(host *HostDBEntry) ([]types.Transaction, error) {
	if host.Network != "mainnet" && host.Network != "zen" {
		panic("wrong host network")
	}

	var blockHeight uint64
	if host.Network == "zen" {
		blockHeight = hdb.sZen.tip.Height
	} else {
		blockHeight = hdb.s.tip.Height
	}
	state := hdb.nodes.ChainManager(host.Network).TipState()
	txnFee := hdb.nodes.ChainManager(host.Network).RecommendedFee().Mul64(4)
	ourKey := hdb.nodes.Wallet(host.Network).Key()
	ourAddr := hdb.nodes.Wallet(host.Network).Address()
	settings := host.Settings

	funding, collateral := calculateFunding(settings, txnFee.Mul64(2048))
	fc := rhpv2.PrepareContractFormation(ourKey.PublicKey(), host.PublicKey, funding, collateral, blockHeight+contractDuration, settings, ourAddr)
	cost := rhpv2.ContractFormationCost(state, fc, settings.ContractPrice)

	txn := types.Transaction{
		FileContracts: []types.FileContract{fc},
	}
	txnFee = txnFee.Mul64(state.TransactionWeight(txn))
	txn.MinerFees = []types.Currency{txnFee}
	cost = cost.Add(txnFee)

	toSign, err := hdb.nodes.Wallet(host.Network).FundTransaction(&txn, cost, true)
	if err != nil {
		return nil, utils.AddContext(err, "unable to fund transaction")
	}

	cf := wallet.ExplicitCoveredFields(txn)
	hdb.nodes.Wallet(host.Network).SignTransaction(&txn, toSign, cf)

	return append(hdb.nodes.ChainManager(host.Network).UnconfirmedParents(txn), txn), nil
}

package hostdb

import (
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/consensus"
	rhpv2 "go.sia.tech/core/rhp/v2"
	"go.sia.tech/core/types"
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

// prepareContractFormation creates a new contract and a formation
// transaction set.
func (hdb *HostDB) prepareContractFormation(host *HostDBEntry) ([]types.Transaction, error) {
	if host.Network != "mainnet" && host.Network != "zen" {
		panic("wrong host network")
	}

	var blockHeight uint64
	var state consensus.State
	var txnFee types.Currency
	if host.Network == "zen" {
		blockHeight = hdb.sZen.tip.Height
		state = hdb.cmZen.TipState()
		txnFee = hdb.cmZen.RecommendedFee().Mul64(3)
	} else {
		blockHeight = hdb.s.tip.Height
		state = hdb.cm.TipState()
		txnFee = hdb.cm.RecommendedFee().Mul64(3)
	}
	settings := host.Settings
	ourKey := hdb.w.Key(host.Network)
	ourAddr := hdb.w.Address(host.Network)

	funding, collateral := calculateFunding(settings, txnFee.Mul64(2048))
	fc := rhpv2.PrepareContractFormation(ourKey.PublicKey(), host.PublicKey, funding, collateral, blockHeight+contractDuration, settings, ourAddr)
	cost := rhpv2.ContractFormationCost(state, fc, settings.ContractPrice)

	txn := types.Transaction{
		FileContracts: []types.FileContract{fc},
	}
	txnFee = txnFee.Mul64(state.TransactionWeight(txn))
	txn.MinerFees = []types.Currency{txnFee}
	cost = cost.Add(txnFee)

	parents, toSign, err := hdb.w.Fund(host.Network, &txn, cost, true)
	if err != nil {
		return nil, utils.AddContext(err, "unable to fund transaction")
	}

	cf := wallet.ExplicitCoveredFields(txn)
	hdb.w.Sign(host.Network, &txn, toSign, cf)

	return append(parents, txn), nil
}

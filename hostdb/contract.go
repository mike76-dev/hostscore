package hostdb

import (
	"time"

	"github.com/mike76-dev/hostscore/internal/utils"
	rhpv2 "go.sia.tech/core/rhp/v2"
	"go.sia.tech/core/types"
)

// calculateFunding calculates the funding of a benchmarking contract.
func (hdb *HostDB) calculateFunding(settings rhpv2.HostSettings) (funding, collateral types.Currency) {
	contractCost := settings.ContractPrice
	downloadCost := settings.DownloadBandwidthPrice
	uploadCost := settings.UploadBandwidthPrice
	storageCost := settings.StoragePrice

	numBenchmarks := contractDuration / (6 * benchmarkInterval / time.Hour)
	dataSize := benchmarkBatchSize * numBenchmarks

	downloadCost = downloadCost.Mul64(uint64(dataSize)).Div64(contractDuration)
	uploadCost = uploadCost.Mul64(uint64(dataSize)).Div64(contractDuration)
	storageCost = storageCost.Mul64(uint64(dataSize))

	txnFee := hdb.cm.RecommendedFee().Mul64(2048).Mul64(3)
	contractCost = contractCost.Add(txnFee)

	funding = contractCost.Add(downloadCost)
	funding = funding.Add(uploadCost)
	funding = funding.Add(storageCost)

	collateral = rhpv2.ContractFormationCollateral(contractDuration, uint64(dataSize), settings)

	return
}

// prepareContractFormation creates a new contract and a formation
// transaction set.
func (hdb *HostDB) prepareContractFormation(host HostDBEntry) ([]types.Transaction, error) {
	blockHeight := hdb.s.tip.Height
	settings := host.Settings
	ourKey := hdb.w.Key()
	ourAddr := hdb.w.Address()

	funding, collateral := hdb.calculateFunding(settings)
	fc := rhpv2.PrepareContractFormation(ourKey.PublicKey(), host.PublicKey, funding, collateral, blockHeight+contractDuration, settings, ourAddr)
	cost := rhpv2.ContractFormationCost(hdb.cm.TipState(), fc, settings.ContractPrice)

	txn := types.Transaction{
		FileContracts: []types.FileContract{fc},
	}
	txnFee := hdb.cm.RecommendedFee().Mul64(2048)
	txn.MinerFees = []types.Currency{txnFee}
	cost = cost.Add(txnFee)

	parents, toSign, err := hdb.w.Fund(&txn, cost)
	if err != nil {
		return nil, utils.AddContext(err, "unable to fund transaction")
	}

	var cf types.CoveredFields
	for i := range txn.SiacoinInputs {
		cf.SiacoinInputs = append(cf.SiacoinInputs, uint64(i))
	}
	for i := range txn.SiacoinOutputs {
		cf.SiacoinOutputs = append(cf.SiacoinOutputs, uint64(i))
	}
	for i := range txn.FileContracts {
		cf.FileContracts = append(cf.FileContracts, uint64(i))
	}
	for i := range txn.MinerFees {
		cf.MinerFees = append(cf.MinerFees, uint64(i))
	}

	err = hdb.w.Sign(&txn, toSign, cf)
	if err != nil {
		hdb.w.Release(append(parents, txn))
		return nil, utils.AddContext(err, "unable to sign transaction")
	}

	return append(parents, txn), nil
}

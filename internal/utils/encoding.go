package utils

import (
	"time"

	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
)

// EncodeSettings encodes the host's settings.
func EncodeSettings(hs *rhpv2.HostSettings, e *types.Encoder) {
	e.WriteBool(hs.AcceptingContracts)
	e.WriteUint64(hs.MaxDownloadBatchSize)
	e.WriteUint64(hs.MaxDuration)
	e.WriteUint64(hs.MaxReviseBatchSize)
	e.WriteString(hs.NetAddress)
	e.WriteUint64(hs.RemainingStorage)
	e.WriteUint64(hs.SectorSize)
	e.WriteUint64(hs.TotalStorage)
	hs.Address.EncodeTo(e)
	e.WriteUint64(hs.WindowSize)
	types.V1Currency(hs.Collateral).EncodeTo(e)
	types.V1Currency(hs.MaxCollateral).EncodeTo(e)
	types.V1Currency(hs.BaseRPCPrice).EncodeTo(e)
	types.V1Currency(hs.ContractPrice).EncodeTo(e)
	types.V1Currency(hs.DownloadBandwidthPrice).EncodeTo(e)
	types.V1Currency(hs.SectorAccessPrice).EncodeTo(e)
	types.V1Currency(hs.StoragePrice).EncodeTo(e)
	types.V1Currency(hs.UploadBandwidthPrice).EncodeTo(e)
	e.WriteUint64(uint64(hs.EphemeralAccountExpiry.Seconds()))
	types.V1Currency(hs.MaxEphemeralAccountBalance).EncodeTo(e)
	e.WriteUint64(hs.RevisionNumber)
	e.WriteString(hs.Version)
	e.WriteString(hs.Release)
	e.WriteString(hs.SiaMuxPort)
}

// DecodeSettings decodes the host's settings.
func DecodeSettings(hs *rhpv2.HostSettings, d *types.Decoder) {
	hs.AcceptingContracts = d.ReadBool()
	hs.MaxDownloadBatchSize = d.ReadUint64()
	hs.MaxDuration = d.ReadUint64()
	hs.MaxReviseBatchSize = d.ReadUint64()
	hs.NetAddress = d.ReadString()
	hs.RemainingStorage = d.ReadUint64()
	hs.SectorSize = d.ReadUint64()
	hs.TotalStorage = d.ReadUint64()
	hs.Address.DecodeFrom(d)
	hs.WindowSize = d.ReadUint64()
	(*types.V1Currency)(&hs.Collateral).DecodeFrom(d)
	(*types.V1Currency)(&hs.MaxCollateral).DecodeFrom(d)
	(*types.V1Currency)(&hs.BaseRPCPrice).DecodeFrom(d)
	(*types.V1Currency)(&hs.ContractPrice).DecodeFrom(d)
	(*types.V1Currency)(&hs.DownloadBandwidthPrice).DecodeFrom(d)
	(*types.V1Currency)(&hs.SectorAccessPrice).DecodeFrom(d)
	(*types.V1Currency)(&hs.StoragePrice).DecodeFrom(d)
	(*types.V1Currency)(&hs.UploadBandwidthPrice).DecodeFrom(d)
	hs.EphemeralAccountExpiry = time.Duration(d.ReadUint64()) * time.Second
	(*types.V1Currency)(&hs.MaxEphemeralAccountBalance).DecodeFrom(d)
	hs.RevisionNumber = d.ReadUint64()
	hs.Version = d.ReadString()
	// COMPAT
	one := d.ReadString()
	two := d.ReadString()
	if d.Err() == nil {
		hs.Release = one
		hs.SiaMuxPort = two
	} else {
		hs.SiaMuxPort = one
	}
}

// EncodePriceTable encodes the host's price table.
func EncodePriceTable(pt *rhpv3.HostPriceTable, e *types.Encoder) {
	e.Write(pt.UID[:])
	e.WriteUint64(uint64(pt.Validity.Seconds()))
	e.WriteUint64(pt.HostBlockHeight)
	types.V1Currency(pt.UpdatePriceTableCost).EncodeTo(e)
	types.V1Currency(pt.AccountBalanceCost).EncodeTo(e)
	types.V1Currency(pt.FundAccountCost).EncodeTo(e)
	types.V1Currency(pt.LatestRevisionCost).EncodeTo(e)
	types.V1Currency(pt.SubscriptionMemoryCost).EncodeTo(e)
	types.V1Currency(pt.SubscriptionNotificationCost).EncodeTo(e)
	types.V1Currency(pt.InitBaseCost).EncodeTo(e)
	types.V1Currency(pt.MemoryTimeCost).EncodeTo(e)
	types.V1Currency(pt.DownloadBandwidthCost).EncodeTo(e)
	types.V1Currency(pt.UploadBandwidthCost).EncodeTo(e)
	types.V1Currency(pt.DropSectorsBaseCost).EncodeTo(e)
	types.V1Currency(pt.DropSectorsUnitCost).EncodeTo(e)
	types.V1Currency(pt.HasSectorBaseCost).EncodeTo(e)
	types.V1Currency(pt.ReadBaseCost).EncodeTo(e)
	types.V1Currency(pt.ReadLengthCost).EncodeTo(e)
	types.V1Currency(pt.RenewContractCost).EncodeTo(e)
	types.V1Currency(pt.RevisionBaseCost).EncodeTo(e)
	types.V1Currency(pt.SwapSectorBaseCost).EncodeTo(e)
	types.V1Currency(pt.WriteBaseCost).EncodeTo(e)
	types.V1Currency(pt.WriteLengthCost).EncodeTo(e)
	types.V1Currency(pt.WriteStoreCost).EncodeTo(e)
	types.V1Currency(pt.TxnFeeMinRecommended).EncodeTo(e)
	types.V1Currency(pt.TxnFeeMaxRecommended).EncodeTo(e)
	types.V1Currency(pt.ContractPrice).EncodeTo(e)
	types.V1Currency(pt.CollateralCost).EncodeTo(e)
	types.V1Currency(pt.MaxCollateral).EncodeTo(e)
	e.WriteUint64(pt.MaxDuration)
	e.WriteUint64(pt.WindowSize)
	e.WriteUint64(pt.RegistryEntriesLeft)
	e.WriteUint64(pt.RegistryEntriesTotal)
}

// DecodePriceTable decodes the host's price table.
func DecodePriceTable(pt *rhpv3.HostPriceTable, d *types.Decoder) {
	d.Read(pt.UID[:])
	pt.Validity = time.Duration(d.ReadUint64()) * time.Second
	pt.HostBlockHeight = d.ReadUint64()
	(*types.V1Currency)(&pt.UpdatePriceTableCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.AccountBalanceCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.FundAccountCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.LatestRevisionCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.SubscriptionMemoryCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.SubscriptionNotificationCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.InitBaseCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.MemoryTimeCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.DownloadBandwidthCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.UploadBandwidthCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.DropSectorsBaseCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.DropSectorsUnitCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.HasSectorBaseCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.ReadBaseCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.ReadLengthCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.RenewContractCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.RevisionBaseCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.SwapSectorBaseCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.WriteBaseCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.WriteLengthCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.WriteStoreCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.TxnFeeMinRecommended).DecodeFrom(d)
	(*types.V1Currency)(&pt.TxnFeeMaxRecommended).DecodeFrom(d)
	(*types.V1Currency)(&pt.ContractPrice).DecodeFrom(d)
	(*types.V1Currency)(&pt.CollateralCost).DecodeFrom(d)
	(*types.V1Currency)(&pt.MaxCollateral).DecodeFrom(d)
	pt.MaxDuration = d.ReadUint64()
	pt.WindowSize = d.ReadUint64()
	pt.RegistryEntriesLeft = d.ReadUint64()
	pt.RegistryEntriesTotal = d.ReadUint64()
}

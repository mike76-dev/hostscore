package hostdb

import (
	"errors"
	"fmt"

	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
)

const (
	contractDuration = 7 * 144 // 7 days
)

// hostDBPriceLimits are meant to protect the node from malicious hosts
// or from SC price spikes.
type hostDBPriceLimits struct {
	maxContractPrice     types.Currency
	maxUploadPrice       types.Currency
	maxDownloadPrice     types.Currency
	maxStoragePrice      types.Currency
	maxBaseRPCPrice      types.Currency
	maxSectorAccessPrice types.Currency
}

const (
	maxBaseRPCPriceVsBandwidth      = uint64(40e3)
	maxSectorAccessPriceVsBandwidth = uint64(400e3)
)

var (
	maxContractPrice   = types.Siacoins(1)                                // 1 SC
	maxUploadPriceSC   = types.Siacoins(1000)                             // 1 KS/TB
	maxDownloadPriceSC = types.Siacoins(3000)                             // 3 KS/TB
	maxStoragePriceSC  = types.Siacoins(1000).Div64(1e12).Div64(30 * 144) // 1 KS/TB/month

	maxBaseRPCPriceSC      = types.Siacoins(1).Div64(100) // 10 mS
	maxSectorAccessPriceSC = types.Siacoins(1).Div64(100) // 10 mS

	maxUploadPriceUSD   = 6.0  // 6 USD/TB
	maxDownloadPriceUSD = 18.0 // 18 USD/TB
	maxStoragePriceUSD  = 6.0  // 6 USD/TB/month

	maxBaseRPCPriceUSD      = 6e-5
	maxSectorAccessPriceUSD = 6e-5
)

// checkGougingV1 performs a number of gouging checks before forming
// a contract with the V1 host.
func checkGougingV1(hs *rhpv2.HostSettings, pt *rhpv3.HostPriceTable, limits hostDBPriceLimits) (err error) {
	// Host settings checks.
	if hs != nil && (*hs != rhpv2.HostSettings{}) {
		if err = checkContractGougingRHPv2(*hs); err != nil {
			return
		}
		if err = checkPriceGougingHS(*hs, limits); err != nil {
			return
		}
	}

	// Price table checks.
	if pt != nil && (*pt != rhpv3.HostPriceTable{}) {
		if err = checkDownloadGougingRHPv3(*pt, limits); err != nil {
			return
		}
		if err = checkPriceGougingPT(*pt, limits); err != nil {
			return
		}
		if err = checkUploadGougingRHPv3(*pt, limits); err != nil {
			return
		}
		if err = checkContractGougingRHPv3(*pt); err != nil {
			return
		}
	}

	return nil
}

// checkGougingV2 performs a number of gouging checks before forming
// a contract with the V2 host.
func checkGougingV2(hs *rhpv4.HostSettings, limits hostDBPriceLimits) error {
	// Upload gouging.
	if hs.Prices.StoragePrice.Cmp(limits.maxStoragePrice) > 0 {
		return fmt.Errorf("storage price exceeds max storage price: %v > %v", hs.Prices.StoragePrice, limits.maxStoragePrice)
	}

	if hs.Prices.IngressPrice.Cmp(limits.maxUploadPrice) > 0 {
		return fmt.Errorf("ingress price exceeds max upload price: %v > %v", hs.Prices.IngressPrice, limits.maxUploadPrice)
	}

	// Download gouging.
	if hs.Prices.EgressPrice.Cmp(limits.maxDownloadPrice) > 0 {
		return fmt.Errorf("egress price exceeds max download price: %v > %v", hs.Prices.EgressPrice, limits.maxDownloadPrice)
	}

	// General gouging.
	if hs.Prices.ContractPrice.Cmp(limits.maxContractPrice) > 0 {
		return fmt.Errorf("contract price exceeds max contract price: %v > %v", hs.Prices.ContractPrice, limits.maxContractPrice)
	}

	if hs.MaxCollateral.IsZero() {
		return errors.New("MaxCollateral of the host is 0")
	}

	return nil
}

func checkPriceGougingHS(hs rhpv2.HostSettings, limits hostDBPriceLimits) error {
	// Check base RPC price.
	if hs.BaseRPCPrice.Cmp(limits.maxBaseRPCPrice) > 0 {
		return fmt.Errorf("base RPC price exceeds limit: %v > %v", hs.BaseRPCPrice, limits.maxBaseRPCPrice)
	}

	maxBaseRPCPrice, overflow := hs.DownloadBandwidthPrice.Mul64WithOverflow(maxBaseRPCPriceVsBandwidth)
	if overflow {
		return fmt.Errorf("download price too high: %v", hs.DownloadBandwidthPrice)
	}
	if hs.BaseRPCPrice.Cmp(maxBaseRPCPrice) > 0 {
		return fmt.Errorf("base RPC price too high: %v > %v", hs.BaseRPCPrice, maxBaseRPCPrice)
	}

	// Check sector access price.
	if hs.DownloadBandwidthPrice.IsZero() {
		hs.DownloadBandwidthPrice = types.NewCurrency64(1)
	}

	if hs.SectorAccessPrice.Cmp(limits.maxSectorAccessPrice) > 0 {
		return fmt.Errorf("sector access price exceeds limit: %v > %v", hs.SectorAccessPrice, limits.maxSectorAccessPrice)
	}

	maxSectorAccessPrice, overflow := hs.DownloadBandwidthPrice.Mul64WithOverflow(maxSectorAccessPriceVsBandwidth)
	if overflow {
		return fmt.Errorf("download price too high: %v", hs.DownloadBandwidthPrice)
	}
	if hs.SectorAccessPrice.Cmp(maxSectorAccessPrice) > 0 {
		return fmt.Errorf("sector access price too high: %v > %v", hs.SectorAccessPrice, maxSectorAccessPrice)
	}

	// Check max storage price.
	if hs.StoragePrice.Cmp(limits.maxStoragePrice) > 0 {
		return fmt.Errorf("storage price exceeds limit: %v > %v", hs.StoragePrice, limits.maxStoragePrice)
	}

	// Check contract price.
	if hs.ContractPrice.Cmp(limits.maxContractPrice) > 0 {
		return fmt.Errorf("contract price exceeds limit: %v > %v", hs.ContractPrice, limits.maxContractPrice)
	}

	return nil
}

// checkUnusedDefaults check if the default settings were not manipulated with.
func checkUnusedDefaults(pt rhpv3.HostPriceTable) error {
	// Check ReadLengthCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.ReadLengthCost) < 0 {
		return fmt.Errorf("ReadLengthCost of host is %v but should be %v", pt.ReadLengthCost, types.NewCurrency64(1))
	}

	// Check WriteLengthCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.WriteLengthCost) < 0 {
		return fmt.Errorf("WriteLengthCost of %v exceeds 1H", pt.WriteLengthCost)
	}

	// Check AccountBalanceCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.AccountBalanceCost) < 0 {
		return fmt.Errorf("AccountBalanceCost of %v exceeds 1H", pt.AccountBalanceCost)
	}

	// Check FundAccountCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.FundAccountCost) < 0 {
		return fmt.Errorf("FundAccountCost of %v exceeds 1H", pt.FundAccountCost)
	}

	// Check UpdatePriceTableCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.UpdatePriceTableCost) < 0 {
		return fmt.Errorf("UpdatePriceTableCost of %v exceeds 1H", pt.UpdatePriceTableCost)
	}

	// Check HasSectorBaseCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.HasSectorBaseCost) < 0 {
		return fmt.Errorf("HasSectorBaseCost of %v exceeds 1H", pt.HasSectorBaseCost)
	}

	// Check MemoryTimeCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.MemoryTimeCost) < 0 {
		return fmt.Errorf("MemoryTimeCost of %v exceeds 1H", pt.MemoryTimeCost)
	}

	// Check DropSectorsBaseCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.DropSectorsBaseCost) < 0 {
		return fmt.Errorf("DropSectorsBaseCost of %v exceeds 1H", pt.DropSectorsBaseCost)
	}

	// Check DropSectorsUnitCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.DropSectorsUnitCost) < 0 {
		return fmt.Errorf("DropSectorsUnitCost of %v exceeds 1H", pt.DropSectorsUnitCost)
	}

	// Check SwapSectorBaseCost - should be 1H as it's unused by hosts.
	if types.NewCurrency64(1).Cmp(pt.SwapSectorBaseCost) < 0 {
		return fmt.Errorf("SwapSectorBaseCost of %v exceeds 1H", pt.SwapSectorBaseCost)
	}

	// Check SubscriptionMemoryCost - expect 1H default.
	if types.NewCurrency64(1).Cmp(pt.SubscriptionMemoryCost) < 0 {
		return fmt.Errorf("SubscriptionMemoryCost of %v exceeds 1H", pt.SubscriptionMemoryCost)
	}

	// Check SubscriptionNotificationCost - expect 1H default.
	if types.NewCurrency64(1).Cmp(pt.SubscriptionNotificationCost) < 0 {
		return fmt.Errorf("SubscriptionNotificationCost of %v exceeds 1H", pt.SubscriptionNotificationCost)
	}

	// Check RenewContractCost - expect 100nS default.
	if types.Siacoins(1).Mul64(100).Div64(1e9).Cmp(pt.RenewContractCost) < 0 {
		return fmt.Errorf("RenewContractCost of %v exceeds 100nS", pt.RenewContractCost)
	}

	// Check RevisionBaseCost - expect 0H default.
	if types.ZeroCurrency.Cmp(pt.RevisionBaseCost) < 0 {
		return fmt.Errorf("RevisionBaseCost of %v exceeds 0H", pt.RevisionBaseCost)
	}

	return nil
}

// checkPriceGougingPT checks the price table.
func checkPriceGougingPT(pt rhpv3.HostPriceTable, limits hostDBPriceLimits) error {
	// Check unused defaults.
	if err := checkUnusedDefaults(pt); err != nil {
		return err
	}

	// Check base RPC price.
	if limits.maxBaseRPCPrice.Cmp(pt.InitBaseCost) < 0 {
		return fmt.Errorf("init base cost exceeds limit: %v > %v", pt.InitBaseCost, limits.maxBaseRPCPrice)
	}

	// Check contract price.
	if pt.ContractPrice.Cmp(limits.maxContractPrice) > 0 {
		return fmt.Errorf("contract price exceeds limit: %v > %v", pt.ContractPrice, limits.maxContractPrice)
	}

	// Check storage price.
	if pt.WriteStoreCost.Cmp(limits.maxStoragePrice) > 0 {
		return fmt.Errorf("storage price exceeds limit: %v > %v", pt.WriteStoreCost, limits.maxStoragePrice)
	}

	// Check max collateral.
	if pt.MaxCollateral.IsZero() {
		return errors.New("MaxCollateral of the host is 0")
	}

	// check LatestRevisionCost - expect sane value
	twoKiBMax, overflow := limits.maxDownloadPrice.Mul64WithOverflow(2048)
	if overflow {
		twoKiBMax = types.MaxCurrency
	}
	maxRevisionCost, overflow := limits.maxBaseRPCPrice.AddWithOverflow(twoKiBMax)
	if overflow {
		maxRevisionCost = types.MaxCurrency
	}
	if pt.LatestRevisionCost.Cmp(maxRevisionCost) > 0 {
		return fmt.Errorf("LatestRevisionCost of %v exceeds maximum cost of %v", pt.LatestRevisionCost, maxRevisionCost)
	}

	return nil
}

func checkContractGougingRHPv2(hs rhpv2.HostSettings) error {
	if hs.MaxDuration < contractDuration {
		return fmt.Errorf("max contract duration is too low: %v < %v", hs.MaxDuration, contractDuration)
	}

	if !hs.AcceptingContracts {
		return errors.New("host is not accepting contracts")
	}

	return nil
}

func checkContractGougingRHPv3(pt rhpv3.HostPriceTable) error {
	if pt.MaxDuration < contractDuration {
		return fmt.Errorf("max contract duration is too low: %v < %v", pt.MaxDuration, contractDuration)
	}
	return nil
}

// checkDownloadGougingRHPv3 checks the price table.
func checkDownloadGougingRHPv3(pt rhpv3.HostPriceTable, limits hostDBPriceLimits) error {
	sectorDownloadPrice, overflow := sectorReadCostRHPv3(pt)
	if overflow {
		return errors.New("overflow detected when computing sector download price")
	}
	dpptb, overflow := sectorDownloadPrice.Mul64WithOverflow(1 << 40 / rhpv2.SectorSize) // sectors per TiB
	if overflow {
		return errors.New("overflow detected when computing download price per TiB")
	}
	if dpptb.Cmp(limits.maxDownloadPrice) > 0 {
		return fmt.Errorf("download price exceeds limit: %v > %v", dpptb, limits.maxDownloadPrice)
	}
	return nil
}

// checkUploadGougingRHPv3 checks the price table.
func checkUploadGougingRHPv3(pt rhpv3.HostPriceTable, limits hostDBPriceLimits) error {
	sectorUploadPricePerMonth, overflow := sectorUploadCostRHPv3(pt)
	if overflow {
		return errors.New("overflow detected when computing sector price")
	}
	uploadPrice, overflow := sectorUploadPricePerMonth.Mul64WithOverflow(1 << 40 / rhpv2.SectorSize) // sectors per TiB
	if overflow {
		return errors.New("overflow detected when computing upload price per TiB")
	}
	if uploadPrice.Cmp(limits.maxUploadPrice) > 0 {
		return fmt.Errorf("upload price exceeds limit: %v > %v", uploadPrice, limits.maxUploadPrice)
	}
	return nil
}

// sectorReadCostRHPv3 calculates the cost of reading a sector.
func sectorReadCostRHPv3(pt rhpv3.HostPriceTable) (types.Currency, bool) {
	// Base.
	base, overflow := pt.ReadLengthCost.Mul64WithOverflow(rhpv2.SectorSize)
	if overflow {
		return types.ZeroCurrency, true
	}
	base, overflow = base.AddWithOverflow(pt.ReadBaseCost)
	if overflow {
		return types.ZeroCurrency, true
	}
	base, overflow = base.AddWithOverflow(pt.InitBaseCost)
	if overflow {
		return types.ZeroCurrency, true
	}
	// Bandwidth.
	ingress, overflow := pt.UploadBandwidthCost.Mul64WithOverflow(32)
	if overflow {
		return types.ZeroCurrency, true
	}
	egress, overflow := pt.DownloadBandwidthCost.Mul64WithOverflow(rhpv2.SectorSize)
	if overflow {
		return types.ZeroCurrency, true
	}
	// Total.
	total, overflow := base.AddWithOverflow(ingress)
	if overflow {
		return types.ZeroCurrency, true
	}
	total, overflow = total.AddWithOverflow(egress)
	if overflow {
		return types.ZeroCurrency, true
	}
	return total, false
}

// sectorUploadCostRHPv3 calculates the cost of uploading a sector per month.
func sectorUploadCostRHPv3(pt rhpv3.HostPriceTable) (types.Currency, bool) {
	// Write.
	writeCost, overflow := pt.WriteLengthCost.Mul64WithOverflow(rhpv2.SectorSize)
	if overflow {
		return types.ZeroCurrency, true
	}
	writeCost, overflow = writeCost.AddWithOverflow(pt.WriteBaseCost)
	if overflow {
		return types.ZeroCurrency, true
	}
	writeCost, overflow = writeCost.AddWithOverflow(pt.InitBaseCost)
	if overflow {
		return types.ZeroCurrency, true
	}
	// Bandwidth.
	ingress, overflow := pt.UploadBandwidthCost.Mul64WithOverflow(rhpv2.SectorSize)
	if overflow {
		return types.ZeroCurrency, true
	}
	// Total.
	total, overflow := writeCost.AddWithOverflow(ingress)
	if overflow {
		return types.ZeroCurrency, true
	}
	return total, false
}

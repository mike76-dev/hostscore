package hostdb

import (
	"errors"
	"fmt"

	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
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

// checkGouging performs a number of gouging checks before forming
// a contract with the host.
func checkGouging(hs *rhpv2.HostSettings, pt *rhpv3.HostPriceTable, limits hostDBPriceLimits) (err error) {
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

func checkPriceGougingHS(hs rhpv2.HostSettings, limits hostDBPriceLimits) error {
	// Check base RPC price.
	if hs.BaseRPCPrice.Cmp(limits.maxBaseRPCPrice) > 0 {
		return fmt.Errorf("base RPC price exceeds limit: %v > %v", hs.BaseRPCPrice, limits.maxBaseRPCPrice)
	}

	// Check sector access price.
	if hs.SectorAccessPrice.Cmp(limits.maxSectorAccessPrice) > 0 {
		return fmt.Errorf("sector access price exceeds limit: %v > %v", hs.SectorAccessPrice, limits.maxSectorAccessPrice)
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

// checkPriceGougingPT checks the price table.
func checkPriceGougingPT(pt rhpv3.HostPriceTable, limits hostDBPriceLimits) error {
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

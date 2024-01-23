package hostdb

import (
	"errors"
	"fmt"

	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
)

const (
	maxBaseRPCPriceVsBandwidth      = uint64(40e3)
	maxSectorAccessPriceVsBandwidth = uint64(400e3)

	contractDuration = 7 * 144 // 7 days
)

var (
	maxContractPrice = types.Siacoins(1)                                // 1 SC
	maxUploadPrice   = types.Siacoins(1000)                             // 1 KS/TB
	maxDownloadPrice = types.Siacoins(3000)                             // 3 KS/TB
	maxStoragePrice  = types.Siacoins(1000).Div64(1e12).Div64(30 * 144) // 1 KS/TB/month

	maxBaseRPCPrice      = types.Siacoins(1).Div64(100) // 10 mS
	maxSectorAccessPrice = types.Siacoins(1).Div64(100) // 10 mS
)

// checkGouging performs a number of gouging checks before forming
// a contract with the host.
func checkGouging(height uint64, hs *rhpv2.HostSettings, pt *rhpv3.HostPriceTable) (err error) {
	if hs == nil {
		return errors.New("host's settings unknown")
	}
	if pt == nil {
		return errors.New("host has no price table")
	}

	// Host settings checks.
	if err = checkContractGougingRHPv2(*hs); err != nil {
		return
	}
	if err = checkPriceGougingHS(*hs); err != nil {
		return
	}

	// Price table checks.
	if pt != nil {
		if err = checkDownloadGougingRHPv3(*pt); err != nil {
			return
		}
		if err = checkPriceGougingPT(height, *pt); err != nil {
			return
		}
		if err = checkUploadGougingRHPv3(*pt); err != nil {
			return
		}
		if err = checkContractGougingRHPv3(*pt); err != nil {
			return
		}
	}

	return nil
}

func checkPriceGougingHS(hs rhpv2.HostSettings) error {
	// Check base RPC price.
	if hs.BaseRPCPrice.Cmp(maxBaseRPCPrice) > 0 {
		return fmt.Errorf("base RPC price exceeds limit: %v > %v", hs.BaseRPCPrice, maxBaseRPCPrice)
	}
	maxBaseRPCPrice := hs.DownloadBandwidthPrice.Mul64(maxBaseRPCPriceVsBandwidth)
	if hs.BaseRPCPrice.Cmp(maxBaseRPCPrice) > 0 {
		return errors.New("base RPC price too high")
	}

	// Check sector access price.
	if hs.SectorAccessPrice.Cmp(maxSectorAccessPrice) > 0 {
		return fmt.Errorf("sector access price exceeds limit: %v > %v", hs.SectorAccessPrice, maxSectorAccessPrice)
	}
	if hs.DownloadBandwidthPrice.IsZero() {
		hs.DownloadBandwidthPrice = types.NewCurrency64(1)
	}
	maxSAPrice := hs.DownloadBandwidthPrice.Mul64(maxSectorAccessPriceVsBandwidth)
	if hs.SectorAccessPrice.Cmp(maxSAPrice) > 0 {
		return errors.New("sector access price too high")
	}

	// Check max storage price.
	if hs.StoragePrice.Cmp(maxStoragePrice) > 0 {
		return fmt.Errorf("storage price exceeds limit: %v > %v", hs.StoragePrice, maxStoragePrice)
	}

	// Check contract price.
	if hs.ContractPrice.Cmp(maxContractPrice) > 0 {
		return fmt.Errorf("contract price exceeds limit: %v > %v", hs.ContractPrice, maxContractPrice)
	}

	return nil
}

// checkPriceGougingPT checks the price table.
func checkPriceGougingPT(height uint64, pt rhpv3.HostPriceTable) error {
	// Check base RPC price.
	if maxBaseRPCPrice.Cmp(pt.InitBaseCost) < 0 {
		return fmt.Errorf("init base cost exceeds limit: %v > %v", pt.InitBaseCost, maxBaseRPCPrice)
	}

	// Check contract price.
	if pt.ContractPrice.Cmp(maxContractPrice) > 0 {
		return fmt.Errorf("contract price exceeds limit: %v > %v", pt.ContractPrice, maxContractPrice)
	}

	// Check storage price.
	if pt.WriteStoreCost.Cmp(maxStoragePrice) > 0 {
		return fmt.Errorf("storage price exceeds limit: %v > %v", pt.WriteStoreCost, maxStoragePrice)
	}

	// Check block height.
	if pt.HostBlockHeight < height-6 {
		return errors.New("host is not synced")
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
func checkDownloadGougingRHPv3(pt rhpv3.HostPriceTable) error {
	sectorDownloadPrice, overflow := sectorReadCostRHPv3(pt)
	if overflow {
		return errors.New("overflow detected when computing sector download price")
	}
	dpptb, overflow := sectorDownloadPrice.Mul64WithOverflow(1 << 40 / rhpv2.SectorSize) // sectors per TiB
	if overflow {
		return errors.New("overflow detected when computing download price per TiB")
	}
	if dpptb.Cmp(maxDownloadPrice) > 0 {
		return fmt.Errorf("download price exceeds limit: %v > %v", dpptb, maxDownloadPrice)
	}
	return nil
}

// checkUploadGougingRHPv3 checks the price table.
func checkUploadGougingRHPv3(pt rhpv3.HostPriceTable) error {
	sectorUploadPricePerMonth, overflow := sectorUploadCostRHPv3(pt)
	if overflow {
		return errors.New("overflow detected when computing sector price")
	}
	uploadPrice, overflow := sectorUploadPricePerMonth.Mul64WithOverflow(1 << 40 / rhpv2.SectorSize) // sectors per TiB
	if overflow {
		return errors.New("overflow detected when computing upload price per TiB")
	}
	if uploadPrice.Cmp(maxUploadPrice) > 0 {
		return fmt.Errorf("upload price exceeds limit: %v > %v", uploadPrice, maxUploadPrice)
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

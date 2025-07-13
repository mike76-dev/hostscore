package hostdb

import (
	"errors"
	"fmt"

	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
)

const (
	contractDuration = 7 * 144 // 7 days
)

// hostDBPriceLimits are meant to protect the node from malicious hosts
// or from SC price spikes.
type hostDBPriceLimits struct {
	maxContractPrice types.Currency
	maxUploadPrice   types.Currency
	maxDownloadPrice types.Currency
	maxStoragePrice  types.Currency
}

var (
	maxContractPrice   = types.Siacoins(1)                                // 1 SC
	maxUploadPriceSC   = types.Siacoins(1500)                             // 1.5 KS/TB
	maxDownloadPriceSC = types.Siacoins(4500)                             // 4.5 KS/TB
	maxStoragePriceSC  = types.Siacoins(1500).Div64(1e12).Div64(30 * 144) // 1.5 KS/TB/month

	maxUploadPriceUSD   = 6.0  // 6 USD/TB
	maxDownloadPriceUSD = 18.0 // 18 USD/TB
	maxStoragePriceUSD  = 6.0  // 6 USD/TB/month
)

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

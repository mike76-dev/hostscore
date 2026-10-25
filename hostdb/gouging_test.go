package hostdb

import (
	"strings"
	"testing"

	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
)

func TestCheckGougingV2(t *testing.T) {
	limits := hostDBPriceLimits{
		maxContractPrice: types.Siacoins(1),
		maxUploadPrice:   types.Siacoins(100),
		maxDownloadPrice: types.Siacoins(300),
		maxStoragePrice:  types.Siacoins(100),
	}
	okSettings := rhpv4.HostSettings{
		MaxCollateral: types.Siacoins(1000),
		Prices: rhpv4.HostPrices{
			ContractPrice: types.Siacoins(1),
			StoragePrice:  types.Siacoins(100),
			IngressPrice:  types.Siacoins(100),
			EgressPrice:   types.Siacoins(300),
		},
	}

	if err := checkGougingV2(&okSettings, limits); err != nil {
		t.Errorf("prices at the limits: got %v, want nil", err)
	}

	// Uninitialized limits (exchange rate not fetched yet) must be rejected
	// even for a host with zero prices.
	if err := checkGougingV2(&okSettings, hostDBPriceLimits{maxContractPrice: types.Siacoins(1)}); err == nil {
		t.Error("uninitialized limits: got nil, want an error")
	}

	tests := []struct {
		name   string
		mutate func(*rhpv4.HostSettings)
		substr string
	}{
		{"storage price", func(hs *rhpv4.HostSettings) { hs.Prices.StoragePrice = types.Siacoins(101) }, "storage price"},
		{"ingress price", func(hs *rhpv4.HostSettings) { hs.Prices.IngressPrice = types.Siacoins(101) }, "ingress price"},
		{"egress price", func(hs *rhpv4.HostSettings) { hs.Prices.EgressPrice = types.Siacoins(301) }, "egress price"},
		{"contract price", func(hs *rhpv4.HostSettings) { hs.Prices.ContractPrice = types.Siacoins(2) }, "contract price"},
		{"zero max collateral", func(hs *rhpv4.HostSettings) { hs.MaxCollateral = types.ZeroCurrency }, "MaxCollateral"},
	}
	for _, tt := range tests {
		hs := okSettings
		tt.mutate(&hs)
		err := checkGougingV2(&hs, limits)
		if err == nil {
			t.Errorf("%s: got nil, want an error", tt.name)
		} else if !strings.Contains(err.Error(), tt.substr) {
			t.Errorf("%s: got %q, want it to mention %q", tt.name, err, tt.substr)
		}
	}
}

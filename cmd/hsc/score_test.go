package main

import (
	"math"
	"testing"

	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
)

func TestCollateralScore(t *testing.T) {
	tests := []struct {
		name                                        string
		uploadSectorCost, maxCollateral, collateral types.Currency
		want                                        float64
	}{
		{
			name: "zero max collateral",
			want: 0,
		},
		{
			name:             "generous collateral",
			uploadSectorCost: types.NewCurrency64(1),
			maxCollateral:    types.Siacoins(1e6),
			collateral:       types.NewCurrency64(1),
			want:             1,
		},
		{
			name:             "collateral below cutoff",
			uploadSectorCost: types.Siacoins(1),
			maxCollateral:    types.NewCurrency64(1),
			collateral:       types.NewCurrency64(1),
			want:             math.SmallestNonzeroFloat64,
		},
		{
			// regression: collateralCost.Mul64(dataPerHost).Mul64(contractPeriod)
			// overflowed 128 bits and panicked on host-supplied prices
			name:             "huge collateral price",
			uploadSectorCost: types.NewCurrency64(1),
			maxCollateral:    types.Siacoins(1e6),
			collateral:       types.Siacoins(1), // 1 SC per byte per block
			want:             1,
		},
		{
			// regression: overflow panic in storageCost/cutoff computation
			name:             "huge upload cost",
			uploadSectorCost: types.MaxCurrency,
			maxCollateral:    types.Siacoins(1e6),
			collateral:       types.NewCurrency64(1),
			want:             math.SmallestNonzeroFloat64,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := collateralScore(tt.uploadSectorCost, tt.maxCollateral, tt.collateral); got != tt.want {
				t.Errorf("collateralScore(%v, %v, %v) = %v, want %v",
					tt.uploadSectorCost, tt.maxCollateral, tt.collateral, got, tt.want)
			}
		})
	}
}

// TestSectorsToBytes is a regression test: converting a host-supplied
// sector count to bytes used to wrap on overflow.
func TestSectorsToBytes(t *testing.T) {
	tests := []struct {
		sectors uint64
		want    uint64
	}{
		{0, 0},
		{1, 1 << 22},
		{1<<42 - 1, (1<<42 - 1) << 22}, // largest non-clamped value
		{1 << 42, math.MaxUint64},      // used to wrap to 0
		{math.MaxUint64, math.MaxUint64},
	}
	for _, tt := range tests {
		if got := sectorsToBytes(tt.sectors); got != tt.want {
			t.Errorf("sectorsToBytes(%d) = %d, want %d", tt.sectors, got, tt.want)
		}
	}
}

// TestCalculateCosts is a regression test: deriving cost figures from
// stored host-supplied prices used to panic on Currency overflow, letting
// a malicious host crash the portal.
func TestCalculateCosts(t *testing.T) {
	var settings rhpv4.HostSettings
	settings.Prices.StoragePrice = types.Siacoins(1).Div64(1e12).Div64(30 * 144)
	settings.Prices.IngressPrice = types.Siacoins(1).Div64(1e12)
	settings.Prices.EgressPrice = types.Siacoins(1).Div64(1e12)
	if _, ok := calculateCosts(settings); !ok {
		t.Error("expected sane prices to compute")
	}

	for _, set := range []func(*rhpv4.HostPrices){
		func(p *rhpv4.HostPrices) { p.StoragePrice = types.MaxCurrency },
		func(p *rhpv4.HostPrices) { p.IngressPrice = types.MaxCurrency },
		func(p *rhpv4.HostPrices) { p.EgressPrice = types.MaxCurrency },
		func(p *rhpv4.HostPrices) { p.Collateral = types.MaxCurrency },
	} {
		var s rhpv4.HostSettings
		set(&s.Prices)
		if _, ok := calculateCosts(s); ok {
			t.Error("expected overflowing prices to be rejected")
		}
	}
}

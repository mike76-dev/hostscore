package utils

import (
	"math"
	"testing"

	"go.sia.tech/core/types"
)

func mustParseCurrency(t *testing.T, s string) types.Currency {
	t.Helper()
	c, err := types.ParseCurrency(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestFromFloat(t *testing.T) {
	tests := []struct {
		f    float64
		want types.Currency
	}{
		{0, types.ZeroCurrency},
		{-1, types.ZeroCurrency},
		{math.NaN(), types.ZeroCurrency},
		{math.Inf(1), types.ZeroCurrency},
		{1e-25, types.ZeroCurrency}, // less than half a hasting
		{1e-24, types.NewCurrency64(1)},
		{0.3, mustParseCurrency(t, "300mS")}, // regression: used to lose precision
		{0.1, mustParseCurrency(t, "100mS")},
		{1, types.Siacoins(1)},
		{1.5, mustParseCurrency(t, "1.5SC")},
		{1e6, types.Siacoins(1e6)},
		{123.456, mustParseCurrency(t, "123.456SC")},
	}
	for _, tt := range tests {
		if got := FromFloat(tt.f); !got.Equals(tt.want) {
			t.Errorf("FromFloat(%v) = %v, want %v", tt.f, got, tt.want)
		}
	}
}

func TestMulFloat(t *testing.T) {
	tests := []struct {
		c    types.Currency
		f    float64
		want types.Currency
	}{
		{types.Siacoins(1), 0, types.ZeroCurrency},
		{types.Siacoins(1), -2, types.ZeroCurrency},
		{types.Siacoins(1), math.NaN(), types.ZeroCurrency},
		{types.Siacoins(1), 0.3, mustParseCurrency(t, "300mS")}, // regression: used to lose precision
		{types.Siacoins(100), 0.15, types.Siacoins(15)},
		{types.Siacoins(2), 1.5, types.Siacoins(3)},
		{types.NewCurrency64(3), 0.5, types.NewCurrency64(2)}, // rounds to nearest
		{types.NewCurrency64(1), 0.4, types.ZeroCurrency},
		// regression: results over 128 bits used to panic; now clamp
		{types.MaxCurrency, 2, types.MaxCurrency},
	}
	for _, tt := range tests {
		if got := MulFloat(tt.c, tt.f); !got.Equals(tt.want) {
			t.Errorf("MulFloat(%v, %v) = %v, want %v", tt.c, tt.f, got, tt.want)
		}
	}
}

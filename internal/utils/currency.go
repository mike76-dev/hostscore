package utils

import (
	"encoding/binary"
	"math"
	"math/big"
	"strconv"

	"go.sia.tech/core/types"
)

// ratFromFloat converts f to a big.Rat using the shortest decimal
// representation of f, so that e.g. 0.3 becomes exactly 3/10 rather than
// its binary approximation. It returns false if f is not a finite
// non-negative number.
func ratFromFloat(f float64) (*big.Rat, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return nil, false
	}
	return new(big.Rat).SetString(strconv.FormatFloat(f, 'f', -1, 64))
}

// ratToCurrency converts r to a types.Currency, rounding to the nearest
// hasting and clamping at types.MaxCurrency.
func ratToCurrency(r *big.Rat) types.Currency {
	if r.Sign() <= 0 {
		return types.ZeroCurrency
	}
	// Round to nearest: (2*num + denom) / (2*denom).
	num := new(big.Int).Lsh(r.Num(), 1)
	num.Add(num, r.Denom())
	denom := new(big.Int).Lsh(r.Denom(), 1)
	i := num.Quo(num, denom)
	if i.BitLen() > 128 {
		return types.MaxCurrency
	}
	buf := make([]byte, 16)
	i.FillBytes(buf)
	return types.NewCurrency(binary.BigEndian.Uint64(buf[8:]), binary.BigEndian.Uint64(buf[:8]))
}

// MulFloat multiplies a types.Currency by a float64 value.
func MulFloat(c types.Currency, f float64) types.Currency {
	r, ok := ratFromFloat(f)
	if !ok {
		return types.ZeroCurrency
	}
	r.Mul(r, new(big.Rat).SetInt(c.Big()))
	return ratToCurrency(r)
}

// FromFloat converts f Siacoins to a types.Currency value.
func FromFloat(f float64) types.Currency {
	r, ok := ratFromFloat(f)
	if !ok {
		return types.ZeroCurrency
	}
	r.Mul(r, new(big.Rat).SetInt(types.HastingsPerSiacoin.Big()))
	return ratToCurrency(r)
}

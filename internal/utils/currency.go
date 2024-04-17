package utils

import (
	"encoding/binary"
	"math/big"

	"go.sia.tech/core/types"
)

// MulFloat multiplies a types.Currency by a float64 value.
func MulFloat(c types.Currency, f float64) types.Currency {
	x := new(big.Rat).SetInt(c.Big())
	y := new(big.Rat).SetFloat64(f)
	x = x.Mul(x, y)
	nBuf := make([]byte, 16)
	n := x.Num().Bytes()
	copy(nBuf[16-len(n):], n[:])
	num := types.NewCurrency(binary.BigEndian.Uint64(nBuf[8:]), binary.BigEndian.Uint64(nBuf[:8]))
	dBuf := make([]byte, 16)
	d := x.Denom().Bytes()
	copy(dBuf[16-len(d):], d[:])
	denom := types.NewCurrency(binary.BigEndian.Uint64(dBuf[8:]), binary.BigEndian.Uint64(dBuf[:8]))
	return num.Div(denom)
}

// FromFloat converts f Siacoins to a types.Currency value.
func FromFloat(f float64) types.Currency {
	if f < 1e-24 {
		return types.ZeroCurrency
	}
	h := new(big.Rat).SetInt(types.HastingsPerSiacoin.Big())
	r := new(big.Rat).Mul(h, new(big.Rat).SetFloat64(f))
	nBuf := make([]byte, 16)
	n := r.Num().Bytes()
	copy(nBuf[16-len(n):], n[:])
	num := types.NewCurrency(binary.BigEndian.Uint64(nBuf[8:]), binary.BigEndian.Uint64(nBuf[:8]))
	dBuf := make([]byte, 16)
	d := r.Denom().Bytes()
	copy(dBuf[16-len(d):], d[:])
	denom := types.NewCurrency(binary.BigEndian.Uint64(dBuf[8:]), binary.BigEndian.Uint64(dBuf[:8]))
	return num.Div(denom)
}

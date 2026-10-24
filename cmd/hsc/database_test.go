package main

import (
	"bytes"
	"math"
	"testing"

	"go.sia.tech/core/types"
)

// TestClampRange is a regression test: offset+limit used to be computed on
// unclamped user-supplied values, so e.g. offset=1, limit=MaxInt64 wrapped
// negative, skipped the clamp, and panicked slicing hosts[1 : negative].
func TestClampRange(t *testing.T) {
	tests := []struct {
		offset, limit, total  int
		wantOffset, wantLimit int
	}{
		{0, -1, 10, 0, 10},                    // "all"
		{3, 4, 10, 3, 4},                      // normal page
		{3, 100, 10, 3, 7},                    // limit past the end
		{20, 5, 10, 10, 0},                    // offset past the end
		{-5, 5, 10, 0, 5},                     // negative offset
		{1, math.MaxInt, 10, 1, 9},            // used to overflow and panic
		{math.MaxInt, math.MaxInt, 10, 10, 0}, // both huge
		{0, 0, 10, 0, 0},                      // empty page
		{0, -1, 0, 0, 0},                      // empty set
	}
	for _, tt := range tests {
		gotOffset, gotLimit := clampRange(tt.offset, tt.limit, tt.total)
		if gotOffset != tt.wantOffset || gotLimit != tt.wantLimit {
			t.Errorf("clampRange(%d, %d, %d) = (%d, %d), want (%d, %d)",
				tt.offset, tt.limit, tt.total, gotOffset, gotLimit, tt.wantOffset, tt.wantLimit)
		}
	}
}

// TestDecodePrice is a regression test: getPriceChanges used to wrap the
// wrong (always-nil) error variable on decode failures, silently returning
// no results, and choked on the empty price blobs stored for non-V2 hosts.
func TestDecodePrice(t *testing.T) {
	var buf bytes.Buffer
	e := types.NewEncoder(&buf)
	types.V1Currency(types.Siacoins(42)).EncodeTo(e)
	e.Flush()

	var c types.Currency
	if err := decodePrice(buf.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if !c.Equals(types.Siacoins(42)) {
		t.Errorf("decodePrice = %v, want 42 SC", c)
	}

	if err := decodePrice(nil, &c); err != nil {
		t.Fatal(err)
	}
	if !c.IsZero() {
		t.Errorf("empty blob decoded to %v, want zero", c)
	}

	if err := decodePrice([]byte{1, 2, 3}, &c); err == nil {
		t.Error("expected an error for a truncated blob")
	}
}

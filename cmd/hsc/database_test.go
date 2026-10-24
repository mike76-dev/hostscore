package main

import (
	"math"
	"testing"
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

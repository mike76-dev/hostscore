package main

import (
	"math"
	"testing"
	"time"

	"github.com/mike76-dev/hostscore/hostdb"
	rhpv4 "go.sia.tech/core/rhp/v4"
	"go.sia.tech/core/types"
	rhpv4utils "go.sia.tech/coreutils/rhp/v4"
)

// approx reports whether two scores are equal within a small tolerance.
func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

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

func TestPriceAdjustmentScore(t *testing.T) {
	// All three prices are set to the same multiple of their respective
	// threshold (half the maximum), so the average equals the per-price score.
	priceAt := func(num, denom uint64) (d, u, s types.Currency) {
		return maxDownloadPrice.Div64(2).Mul64(num).Div64(denom),
			maxUploadPrice.Div64(2).Mul64(num).Div64(denom),
			maxStoragePrice.Div64(2).Mul64(num).Div64(denom)
	}

	tests := []struct {
		name       string
		num, denom uint64
		want       float64
	}{
		{"free host", 0, 1, 1.0},
		{"half the threshold", 1, 2, 1.0},
		{"exactly at the threshold", 1, 1, 0.5},
		{"twice the threshold", 2, 1, 1.5 / 9}, // 1.5 / 3^2
	}
	for _, tt := range tests {
		d, u, s := priceAt(tt.num, tt.denom)
		if got := priceAdjustmentScore(d, u, s); !approx(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestStorageRemainingScore(t *testing.T) {
	tests := []struct {
		remaining uint64
		want      float64
	}{
		{0, 0},
		{4 * dataPerHost, 1},    // expected storage matches the need exactly
		{8 * dataPerHost, 1},    // more than needed
		{2 * dataPerHost, 0.25}, // half the need -> ratio 0.5 squared
		{dataPerHost, 0.0625},   // quarter of the need
	}
	for _, tt := range tests {
		if got := storageRemainingScore(tt.remaining); !approx(got, tt.want) {
			t.Errorf("storageRemainingScore(%d) = %v, want %v", tt.remaining, got, tt.want)
		}
	}
}

func TestAgeScore(t *testing.T) {
	const day = 24 * time.Hour
	tests := []struct {
		name       string
		knownSince time.Time
		want       float64
	}{
		{"never seen", time.Time{}, 0},
		{"older than 128 days", time.Now().Add(-200 * day), 1},
		{"between 64 and 128 days", time.Now().Add(-70 * day), 1 / 1.5},
		{"brand new", time.Now(), 1 / 972.0}, // divided by every factor
	}
	for _, tt := range tests {
		if got := ageScore(tt.knownSince); !approx(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestUptimeScore(t *testing.T) {
	scan := func(success bool) portalScan {
		return portalScan{Timestamp: time.Now(), Success: success}
	}

	// Special cases for short scan histories.
	tests := []struct {
		name    string
		history []portalScan
		want    float64
	}{
		{"no scans", nil, 0.25},
		{"one successful scan", []portalScan{scan(true)}, 0.75},
		{"one failed scan", []portalScan{scan(false)}, 0.25},
		{"two successful scans", []portalScan{scan(true), scan(true)}, 0.85},
		{"one of two scans failed", []portalScan{scan(true), scan(false)}, 0.5},
		{"two failed scans", []portalScan{scan(false), scan(false)}, 0.05},
	}
	for _, tt := range tests {
		if got := uptimeScore(0, 0, tt.history); !approx(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}

	perfect := []portalScan{scan(true), scan(true), scan(true)}
	if got := uptimeScore(100*time.Hour, 0, perfect); got != 1 {
		t.Errorf("perfect uptime: got %v, want 1", got)
	}

	// 50% downtime over 10 scans: forgiveness lifts the ratio to 0.7,
	// and the penalty is 0.7^60.
	mixed := make([]portalScan, 10)
	for i := range mixed {
		mixed[i] = scan(i%2 == 0)
	}
	want := math.Pow(0.7, 60)
	if got := uptimeScore(50*time.Hour, 50*time.Hour, mixed); !approx(got, want) {
		t.Errorf("half downtime: got %v, want %v", got, want)
	}
}

func TestInteractionScore(t *testing.T) {
	// No interactions yet: the 30/1 prior applies.
	if got, want := interactionScore(0, 0), math.Pow(30.0/31.0, 10); !approx(got, want) {
		t.Errorf("no interactions: got %v, want %v", got, want)
	}

	reliable := interactionScore(1000, 0)
	unreliable := interactionScore(0, 1000)
	neutral := interactionScore(0, 0)
	if !(reliable > neutral && neutral > unreliable) {
		t.Errorf("want reliable (%v) > neutral (%v) > unreliable (%v)", reliable, neutral, unreliable)
	}
	if reliable <= 0.99 {
		t.Errorf("reliable host: got %v, want close to 1", reliable)
	}
	if unreliable >= 0.01 {
		t.Errorf("unreliable host: got %v, want close to 0", unreliable)
	}
}

func TestVersionScore(t *testing.T) {
	tests := []struct {
		version rhpv4.ProtocolVersion
		want    float64
	}{
		{rhpv4utils.ProtocolVersion501, 1.0},
		{rhpv4.ProtocolVersion{5, 0, 2}, 1.0},
		{rhpv4utils.ProtocolVersion500, 0.9},
		{rhpv4.ProtocolVersion{4, 0, 0}, 0.8},
	}
	for _, tt := range tests {
		if got := versionScore(tt.version); got != tt.want {
			t.Errorf("versionScore(%v) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

func TestLatencyScore(t *testing.T) {
	scan := func(success bool, latency time.Duration) portalScan {
		return portalScan{Success: success, Latency: latency}
	}

	tests := []struct {
		name    string
		history []portalScan
		want    float64
	}{
		{"no scans", nil, 0},
		{"only failed scans", []portalScan{scan(false, time.Second)}, 0},
		{"below 10ms", []portalScan{scan(true, 5*time.Millisecond)}, 1},
		{"above 1s", []portalScan{scan(true, 2*time.Second)}, 0},
		{"500ms", []portalScan{scan(true, 500*time.Millisecond)}, 0.5},
		// Failed scans are excluded from the average.
		{"failures ignored", []portalScan{scan(true, 400*time.Millisecond), scan(false, 10*time.Second)}, 0.6},
	}
	for _, tt := range tests {
		if got := latencyScore(tt.history); !approx(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestBenchmarksScore(t *testing.T) {
	bench := func(success bool, up, down float64) hostdb.HostBenchmark {
		return hostdb.HostBenchmark{Success: success, UploadSpeed: up, DownloadSpeed: down}
	}

	tests := []struct {
		name       string
		benchmarks []hostdb.HostBenchmark
		want       float64
	}{
		{"no benchmarks", nil, 0},
		{"only failed benchmarks", []hostdb.HostBenchmark{bench(false, 1e9, 1e9)}, 0},
		{"fast host", []hostdb.HostBenchmark{bench(true, 5e7, 1e8)}, 1},
		{"slow host", []hostdb.HostBenchmark{bench(true, 1e6, 1e6)}, 0},
		{"half upload speed", []hostdb.HostBenchmark{bench(true, 2.5e7, 1e8)}, 0.5},
		// Failed benchmarks are excluded from the averages.
		{"failures ignored", []hostdb.HostBenchmark{bench(true, 5e7, 1e8), bench(false, 0, 0)}, 1},
	}
	for _, tt := range tests {
		if got := benchmarksScore(tt.benchmarks); !approx(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestContractsScore(t *testing.T) {
	if got := contractsScore(true); got != 1 {
		t.Errorf("accepting contracts: got %v, want 1", got)
	}
	if got := contractsScore(false); got != 0 {
		t.Errorf("not accepting contracts: got %v, want 0", got)
	}
}

func TestBytesToSectors(t *testing.T) {
	tests := []struct {
		bytes, want uint64
	}{
		{0, 0},
		{1, 1},
		{rhpv4.SectorSize, 1},
		{rhpv4.SectorSize + 1, 2},
		{10 * rhpv4.SectorSize, 10},
	}
	for _, tt := range tests {
		if got := bytesToSectors(tt.bytes); got != tt.want {
			t.Errorf("bytesToSectors(%d) = %d, want %d", tt.bytes, got, tt.want)
		}
	}
}

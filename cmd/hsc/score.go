package main

import (
	"math"
	"math/big"
	"time"

	"github.com/mike76-dev/hostscore/hostdb"
	"github.com/mike76-dev/hostscore/internal/build"
	rhpv2 "go.sia.tech/core/rhp/v2"
	rhpv3 "go.sia.tech/core/rhp/v3"
	"go.sia.tech/core/types"
)

// To calculate the score of each host, we need to assume the settings
// of an average renter.
var (
	hostPeriodBudget = types.Siacoins(1000)              // 1 KS
	dataPerHost      = uint64(1024 * 1024 * 1024 * 1024) // 1 TiB
	contractPeriod   = uint64(144 * 30)                  // 1 month
)

// calculateScore calculates the total host's score.
func calculateScore(host hostdb.HostDBEntry) scoreBreakdown {
	hostPeriodCost := hostPeriodCostForScore(host.Settings, host.PriceTable)
	sb := scoreBreakdown{
		PricesScore:       priceAdjustmentScore(hostPeriodCost),
		StorageScore:      storageRemainingScore(host.Settings),
		CollateralScore:   collateralScore(host.PriceTable),
		InteractionsScore: interactionScore(host.Interactions.HistoricSuccesses, host.Interactions.HistoricFailures),
		UptimeScore:       uptimeScore(host.Uptime, host.Downtime, host.ScanHistory),
		AgeScore:          ageScore(host.FirstSeen),
		VersionScore:      versionScore(host.Settings),
	}
	sb.TotalScore = sb.PricesScore *
		sb.StorageScore *
		sb.CollateralScore *
		sb.InteractionsScore *
		sb.UptimeScore *
		sb.AgeScore *
		sb.VersionScore
	return sb
}

// calculateGlobalScore calculates the average score over all nodes.
func calculateGlobalScore(host *portalHost) scoreBreakdown {
	hostPeriodCost := hostPeriodCostForScore(host.Settings, host.PriceTable)
	sb := scoreBreakdown{
		PricesScore:     priceAdjustmentScore(hostPeriodCost),
		StorageScore:    storageRemainingScore(host.Settings),
		CollateralScore: collateralScore(host.PriceTable),
		AgeScore:        ageScore(host.FirstSeen),
		VersionScore:    versionScore(host.Settings),
	}
	var us, is float64
	var count int
	for _, interactions := range host.Interactions {
		us += uptimeScore(interactions.Uptime, interactions.Downtime, interactions.ScanHistory)
		is += interactionScore(interactions.HistoricSuccesses, interactions.HistoricFailures)
		count++
	}
	if count > 0 {
		sb.UptimeScore = us / float64(count)
		sb.InteractionsScore = is / float64(count)
	}
	sb.TotalScore = sb.PricesScore *
		sb.StorageScore *
		sb.CollateralScore *
		sb.InteractionsScore *
		sb.UptimeScore *
		sb.AgeScore *
		sb.VersionScore
	return sb
}

// priceAdjustmentScore computes a score between 0 and 1 for a host given its
// price settings.
//   - 0.5 is returned if the host's costs exactly match the settings.
//   - If the host is cheaper than expected, a linear bonus is applied. The best
//     score of 1 is reached when the ratio between host cost and expectations is
//     10x.
//   - If the host is more expensive than expected, an exponential malus is applied.
//     A 2x ratio will already cause the score to drop to 0.16 and a 3x ratio causes
//     it to drop to 0.05.
func priceAdjustmentScore(hostCostPerPeriod types.Currency) float64 {
	ratio := new(big.Rat).SetFrac(hostCostPerPeriod.Big(), hostPeriodBudget.Big())
	fRatio, _ := ratio.Float64()
	switch ratio.Cmp(new(big.Rat).SetUint64(1)) {
	case 0:
		return 0.5 // ratio is exactly 1 -> score is 0.5
	case 1:
		// cost is greater than budget -> score is in range (0; 0.5)
		//
		return 1.5 / math.Pow(3, fRatio)
	case -1:
		// cost < budget -> score is (0.5; 1]
		s := 0.44 + 0.06*(1/fRatio)
		if s > 1.0 {
			s = 1.0
		}
		return s
	}
	panic("unreachable")
}

func storageRemainingScore(h rhpv2.HostSettings) float64 {
	// hostExpectedStorage is the amount of storage that we expect to be able to
	// store on this host.
	hostExpectedStorage := float64(h.RemainingStorage) * 0.25
	// The score for the host is the square of the amount of storage we
	// expected divided by the amount of storage we want. If we expect to be
	// able to store more data on the host than we need to allocate, the host
	// gets full score for storage.
	if hostExpectedStorage >= float64(dataPerHost) {
		return 1
	}
	// Otherwise, the score of the host is the fraction of the data we expect
	// raised to the storage penalty exponentiation.
	storageRatio := hostExpectedStorage / float64(dataPerHost)
	return math.Pow(storageRatio, 2.0)
}

func ageScore(knownSince time.Time) float64 {
	// Sanity check.
	if knownSince.IsZero() {
		return 0
	}

	const day = 24 * time.Hour
	weights := []struct {
		age    time.Duration
		factor float64
	}{
		{128 * day, 1.5},
		{64 * day, 2},
		{32 * day, 2},
		{16 * day, 2},
		{8 * day, 3},
		{4 * day, 3},
		{2 * day, 3},
		{1 * day, 3},
	}

	age := time.Since(knownSince)
	weight := 1.0
	for _, w := range weights {
		if age >= w.age {
			break
		}
		weight /= w.factor
	}

	return weight
}

func collateralScore(pt rhpv3.HostPriceTable) float64 {
	// Ignore hosts which have set their max collateral to 0.
	if pt.MaxCollateral.IsZero() || pt.CollateralCost.IsZero() {
		return 0
	}

	// Convenience variables.
	ratioNum := uint64(3)
	ratioDenom := uint64(2)

	// Compute the cost of storing.
	numSectors := bytesToSectors(dataPerHost)
	storageCost := pt.AppendSectorCost(contractPeriod).Storage.Mul64(numSectors)

	// Calculate the expected collateral for the host allocation.
	expectedCollateral := pt.CollateralCost.Mul64(dataPerHost).Mul64(contractPeriod)
	if expectedCollateral.Cmp(pt.MaxCollateral) > 0 {
		expectedCollateral = pt.MaxCollateral
	}

	// Avoid division by zero.
	if expectedCollateral.IsZero() {
		expectedCollateral = types.NewCurrency64(1)
	}

	// Determine a cutoff at 150% of the storage cost. Meaning that a host
	// should be willing to put in at least 1.5x the amount of money the renter
	// expects to spend on storage on that host.
	cutoff := storageCost.Mul64(ratioNum).Div64(ratioDenom)

	// The score is a linear function between 0 and 1 where the upper limit is
	// 4 times the cutoff. Beyond that, we don't care if a host puts in more
	// collateral.
	cutoffMultiplier := uint64(4)

	if expectedCollateral.Cmp(cutoff) < 0 {
		return math.SmallestNonzeroFloat64 // expectedCollateral <= cutoff -> score is basically 0
	} else if expectedCollateral.Cmp(cutoff.Mul64(cutoffMultiplier)) >= 0 {
		return 1 // expectedCollateral is 10x cutoff -> score is 1
	} else {
		// Perform linear interpolation for all other values.
		slope := new(big.Rat).SetFrac(new(big.Int).SetInt64(1), cutoff.Mul64(cutoffMultiplier).Big())
		intercept := new(big.Rat).Mul(slope, new(big.Rat).SetInt(cutoff.Big())).Neg(slope)
		score := new(big.Rat).SetInt(expectedCollateral.Big())
		score = score.Mul(score, slope)
		score = score.Add(score, intercept)
		fScore, _ := score.Float64()
		if fScore > 1 {
			return 1.0
		}
		return fScore
	}
}

func interactionScore(hs, hf float64) float64 {
	success, fail := 30.0, 1.0
	success += hs
	fail += hf
	return math.Pow(success/(success+fail), 10)
}

func uptimeScore(ut, dt time.Duration, history []hostdb.HostScan) float64 {
	lastScanSuccess := len(history) > 0 && history[0].Success
	uptime := ut
	downtime := dt
	totalScans := len(history)

	// Special cases.
	switch totalScans {
	case 0:
		return 0.25 // no scans yet
	case 1:
		if lastScanSuccess {
			return 0.75 // 1 successful scan
		} else {
			return 0.25 // 1 failed scan
		}
	}

	// Account for the interval between the most recent interaction and the
	// current time.
	if totalScans > 0 {
		finalInterval := time.Since(history[0].Timestamp)
		if lastScanSuccess {
			uptime += finalInterval
		} else {
			downtime += finalInterval
		}
	}
	ratio := float64(uptime) / float64(uptime+downtime)

	// Unconditionally forgive up to 2% downtime.
	if ratio >= 0.98 {
		ratio = 1
	}

	// Forgive downtime inversely proportional to the number of interactions;
	// e.g. if we have only interacted 4 times, and half of the interactions
	// failed, assume a ratio of 88% rather than 50%.
	ratio = math.Max(ratio, 1-(0.03*float64(totalScans)))

	// Calculate the penalty for poor uptime. Penalties increase extremely
	// quickly as uptime falls away from 95%.
	return math.Pow(ratio, 200*math.Min(1-ratio, 0.30))
}

func versionScore(settings rhpv2.HostSettings) float64 {
	versions := []struct {
		version string
		penalty float64
	}{
		{"1.6.0", 0.99},
		{"1.5.9", 0.00},
	}
	weight := 1.0
	for _, v := range versions {
		if build.VersionCmp(settings.Version, v.version) < 0 {
			weight *= v.penalty
		}
	}
	return weight
}

// contractPriceForScore returns the contract price of the host used for
// scoring. Since we don't know whether rhpv2 or rhpv3 are used, we return the
// bigger one for a pesimistic score.
func contractPriceForScore(settings rhpv2.HostSettings, pt rhpv3.HostPriceTable) types.Currency {
	cp := settings.ContractPrice
	if cp.Cmp(pt.ContractPrice) > 0 {
		cp = pt.ContractPrice
	}
	return cp
}

func bytesToSectors(bytes uint64) uint64 {
	numSectors := bytes / rhpv2.SectorSize
	if bytes%rhpv2.SectorSize != 0 {
		numSectors++
	}
	return numSectors
}

func sectorStorageCost(pt rhpv3.HostPriceTable, duration uint64) types.Currency {
	asc := pt.BaseCost().Add(pt.AppendSectorCost(duration))
	return asc.Storage
}

func sectorUploadCost(pt rhpv3.HostPriceTable, duration uint64) types.Currency {
	asc := pt.BaseCost().Add(pt.AppendSectorCost(duration))
	uploadSectorCostRHPv3, _ := asc.Total()
	return uploadSectorCostRHPv3
}

func uploadCostForScore(pt rhpv3.HostPriceTable, bytes uint64) types.Currency {
	uploadSectorCostRHPv3 := sectorUploadCost(pt, contractPeriod)
	numSectors := bytesToSectors(bytes)
	return uploadSectorCostRHPv3.Mul64(numSectors)
}

func downloadCostForScore(pt rhpv3.HostPriceTable, bytes uint64) types.Currency {
	rsc := pt.BaseCost().Add(pt.ReadSectorCost(rhpv2.SectorSize))
	downloadSectorCostRHPv3, _ := rsc.Total()
	numSectors := bytesToSectors(bytes)
	return downloadSectorCostRHPv3.Mul64(numSectors)
}

func storageCostForScore(pt rhpv3.HostPriceTable, bytes uint64) types.Currency {
	storeSectorCostRHPv3 := sectorStorageCost(pt, contractPeriod)
	numSectors := bytesToSectors(bytes)
	return storeSectorCostRHPv3.Mul64(numSectors)
}

func hostPeriodCostForScore(settings rhpv2.HostSettings, pt rhpv3.HostPriceTable) types.Currency {
	// Compute the individual costs.
	hostCollateral := rhpv2.ContractFormationCollateral(contractPeriod, dataPerHost, settings)
	hostContractPrice := contractPriceForScore(settings, pt)
	hostUploadCost := uploadCostForScore(pt, dataPerHost)
	hostDownloadCost := downloadCostForScore(pt, dataPerHost)
	hostStorageCost := storageCostForScore(pt, dataPerHost)
	siafundFee := hostCollateral.
		Add(hostContractPrice).
		Add(hostUploadCost).
		Add(hostDownloadCost).
		Add(hostStorageCost).
		Mul64(39).
		Div64(1000)

	// Add it all up. We multiply the contract price here since we might refresh
	// a contract multiple times.
	return hostContractPrice.Mul64(3).
		Add(hostUploadCost).
		Add(hostDownloadCost).
		Add(hostStorageCost).
		Add(siafundFee)
}

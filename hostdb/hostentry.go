package hostdb

import (
	"math"
)

const (
	interactionDecay       = 0.9995
	interactionDecayLimit  = 500
	interactionWeightLimit = 0.01
)

// updateHistoricInteractions updates a HostDBEntries's historic interactions if more
// than one block passed since the last update. This should be called every time
// before the recent interactions are updated. If passedTime is e.g. 10, this
// means that the recent interactions were updated 10 blocks ago but never
// since. So we need to apply the decay of 1 block before we append the recent
// interactions from 10 blocks ago and then apply the decay of 9 more blocks in
// which the recent interactions have been 0.
func (hdb *HostDB) updateHostHistoricInteractions(host *HostDBEntry) {
	var height uint64
	if host.Network == "zen" {
		height = hdb.sZen.tip.Height
	} else {
		height = hdb.s.tip.Height
	}
	// Check that the last historic update was not in the future.
	if host.Interactions.LastUpdate >= height {
		// The hostdb may be performing a rescan, or maybe no time has passed
		// since the last update, so there is nothing to do.
		return
	}
	passedTime := height - host.Interactions.LastUpdate

	// tmp float64 values for more accurate decay.
	hsi := host.Interactions.HistoricSuccesses
	hfi := host.Interactions.HistoricFailures

	// Apply the decay of a single block.
	decay := interactionDecay
	hsi *= decay
	hfi *= decay

	// Apply the recent interactions of that single block. Recent interactions
	// cannot represent more than interactionWeightLimit of historic
	// interactions, unless there are less than interactionDecayLimit
	// total interactions, and then the recent interactions cannot count for
	// more than interactionWeightLimit of the decay limit.
	rsi := float64(host.Interactions.RecentSuccesses)
	rfi := float64(host.Interactions.RecentFailures)
	if hsi+hfi > interactionDecayLimit {
		if rsi+rfi > interactionWeightLimit*(hsi+hfi) {
			adjustment := interactionWeightLimit * (hsi + hfi) / (rsi + rfi)
			rsi *= adjustment
			rfi *= adjustment
		}
	} else {
		if rsi+rfi > interactionWeightLimit*interactionDecayLimit {
			adjustment := interactionWeightLimit * interactionDecayLimit / (rsi + rfi)
			rsi *= adjustment
			rfi *= adjustment
		}
	}
	hsi += rsi
	hfi += rfi

	// Apply the decay of the rest of the blocks.
	if passedTime > 1 && hsi+hfi > interactionDecayLimit {
		decay := math.Pow(interactionDecay, float64(passedTime-1))
		hsi *= decay
		hfi *= decay
	}

	// Set new values.
	host.Interactions.HistoricSuccesses = hsi
	host.Interactions.HistoricFailures = hfi
	host.Interactions.RecentSuccesses = 0
	host.Interactions.RecentFailures = 0

	// Update the time of the last update.
	host.Interactions.LastUpdate = height
}

// IncrementSuccessfulInteractions increments the number of successful
// interactions with a given host.
func (hdb *HostDB) IncrementSuccessfulInteractions(host *HostDBEntry) error {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()

	// Update historic values if necessary.
	hdb.updateHostHistoricInteractions(host)

	// Increment the successful interactions.
	host.Interactions.RecentSuccesses++

	return nil
}

// IncrementFailedInteractions increments the number of failed interactions with
// a given host.
func (hdb *HostDB) IncrementFailedInteractions(host *HostDBEntry) error {
	hdb.mu.Lock()
	defer hdb.mu.Unlock()

	// If we are offline it probably wasn't the host's fault.
	if !hdb.online(host.Network) {
		return nil
	}

	// Update historic values if necessary.
	hdb.updateHostHistoricInteractions(host)

	// Increment the failed interactions.
	host.Interactions.RecentFailures++

	return nil
}

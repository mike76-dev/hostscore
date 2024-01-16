package hostdb

import (
	"errors"
	"math"

	"github.com/mike76-dev/hostscore/internal/utils"
	"go.sia.tech/core/types"
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
	// Check that the last historic update was not in the future.
	if host.Interactions.LastUpdate >= hdb.s.tip.Height {
		// The hostdb may be performing a rescan, or maybe no time has passed
		// since the last update, so there is nothing to do.
		return
	}
	passedTime := hdb.s.tip.Height - host.Interactions.LastUpdate

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
	host.Interactions.LastUpdate = hdb.s.tip.Height
}

// IncrementSuccessfulInteractions increments the number of successful
// interactions with a host for a given key.
func (hdb *HostDB) IncrementSuccessfulInteractions(key types.PublicKey) error {
	if err := hdb.tg.Add(); err != nil {
		return utils.AddContext(err, "error adding hostdb threadgroup")
	}
	defer hdb.tg.Done()

	hdb.mu.Lock()
	defer hdb.mu.Unlock()

	// Fetch the host.
	host, exists := hdb.s.findHost(key)
	if !exists {
		return errors.New("host not found in the database")
	}

	// Update historic values if necessary.
	hdb.updateHostHistoricInteractions(&host)

	// Increment the successful interactions.
	host.Interactions.RecentSuccesses++

	return hdb.s.updateHost(&host)
}

// IncrementFailedInteractions increments the number of failed interactions with
// a host for a given key.
func (hdb *HostDB) IncrementFailedInteractions(key types.PublicKey) error {
	if err := hdb.tg.Add(); err != nil {
		return utils.AddContext(err, "error adding hostdb threadgroup")
	}
	defer hdb.tg.Done()

	hdb.mu.Lock()
	defer hdb.mu.Unlock()

	// If we are offline it probably wasn't the host's fault.
	if !hdb.online() {
		return nil
	}

	// Fetch the host.
	host, exists := hdb.s.findHost(key)
	if !exists {
		return errors.New("host not found in the database")
	}

	// Update historic values if necessary.
	hdb.updateHostHistoricInteractions(&host)

	// Increment the failed interactions.
	host.Interactions.RecentFailures++

	return hdb.s.updateHost(&host)
}

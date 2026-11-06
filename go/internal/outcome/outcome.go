// Package outcome is how a write call that answers only with its final outcome waits for an effect the daemon finishes elsewhere (the
// tick, a delivery, a spawn): it looks until the effect ended, the limit passed or the caller hung up.
package outcome

import "time"

// Limit is how long such a call waits before it gives up what it scheduled and answers refused.
const Limit = 10 * time.Minute

// poll is how often Wait looks.
const poll = 100 * time.Millisecond

// Wait calls ended until it says the effect ended, then answers true. Past limit (0 is Limit), or once hungUp closes, it answers false:
// the caller then drops what it scheduled, so nothing it did not answer for runs later. An error from ended ends the wait with it.
func Wait(limit time.Duration, hungUp <-chan struct{}, ended func() (bool, error)) (bool, error) {
	if limit == 0 {
		limit = Limit
	}
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		if done, err := ended(); done || err != nil {
			return done, err
		}
		select {
		case <-deadline.C:
			return ended()
		case <-hungUp:
			return ended()
		case <-tick.C:
		}
	}
}

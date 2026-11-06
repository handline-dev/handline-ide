package proposalwrites

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	wait "github.com/handline/handline/go/internal/outcome"
)

// outcome is how a held change ended: applied, or refused with why.
type outcome struct {
	ok   bool
	said string
}

// The tick that carries held changes and the RPC methods each have their own Env, so what they share is kept here. heldMu lets one held
// change be carried or given up at a time; waiters is the calls waiting on each held proposal (an approve and an apply_now may both), by
// its file.
var (
	heldMu  sync.Mutex
	waitMu  sync.Mutex
	waiters = map[string][]chan outcome{}
)

// await is where the outcome of the held proposal pid arrives; done stops waiting for it.
func (e *Env) await(pid string) (got <-chan outcome, done func()) {
	ch, key := make(chan outcome, 1), e.path(pid)
	waitMu.Lock()
	waiters[key] = append(waiters[key], ch)
	waitMu.Unlock()
	return ch, func() {
		waitMu.Lock()
		defer waitMu.Unlock()
		waiters[key] = slices.DeleteFunc(waiters[key], func(c chan outcome) bool { return c == ch })
		if len(waiters[key]) == 0 {
			delete(waiters, key)
		}
	}
}

// settled hands each call waiting on pid how it ended.
func (e *Env) settled(pid string, ok bool, said string) {
	waitMu.Lock()
	defer waitMu.Unlock()
	for _, ch := range waiters[e.path(pid)] {
		select {
		case ch <- outcome{ok, said}:
		default:
		}
	}
}

// waitHeld waits up to the limit (0 is outcome.Limit) for the held proposal pid to end. Past it, or when the caller hangs up, a proposal
// still held is given up (withdrawn), so nothing is left waiting that the caller did not hear the end of; ended false then says so.
func (e *Env) waitHeld(pid string, got <-chan outcome, hungUp <-chan struct{}, limit time.Duration) (o outcome, ended bool) {
	arrived := func() (bool, error) {
		select {
		case o = <-got:
			return true, nil
		default:
			return false, nil
		}
	}
	if ok, _ := wait.Wait(limit, hungUp, arrived); ok {
		return o, true
	}
	heldMu.Lock()
	defer heldMu.Unlock()
	if ok, _ := arrived(); ok { // it ended just before the lock was taken
		return o, true
	}
	if prop, err := e.load(pid); err == nil && prop != nil && prop.str("state") == applying {
		waiting := strings.ReplaceAll(prop.str("waiting_on"), ",", ", ")
		prop.pop("waiting_on")
		prop.pop("waiting_since")
		prop.pop("apply_now")
		prop.pop("apply_now_esc")
		prop.set("state", "withdrawn")
		if err := e.save(prop); err != nil {
			e.logf("  proposal: %s: not withdrawn after the wait: %v", pid, err)
		}
		e.tell(prop, fmt.Sprintf("proposal %s was dropped: %s did not end the turn in time, so nothing was changed", pid, waiting))
	}
	return outcome{}, false
}

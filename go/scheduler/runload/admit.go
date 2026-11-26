package runload

import "time"

const (
	// MaxWait is the longest a heavy run waits for its turn: past it the run goes, whatever the cap, so nothing waits forever.
	MaxWait = 15 * time.Minute
	// AskEvery is how often a waiting hook asks again.
	AskEvery = 3 * time.Second
	// askFor is how long an ask stands without being asked again: a hook that went away gives up its place.
	askFor = 5 * AskEvery
	// grantFor is how long a turn that was given counts as a run while its start note is not there: the hook writes the note right
	// after the answer, so a turn with no note after this long is one whose run never began or has already ended.
	grantFor = 10 * time.Second
	// rereadEvery is the least time between two readings of the runs for an ask.
	rereadEvery = AskEvery
)

type ask struct {
	id, agent, shape string
	since, seen      time.Time
}

// Answer is what the hook is told before a heavy run.
type Answer struct {
	Go bool
	// Ahead is how many waiting runs are before this one, and Cap how many go at once, when Go is false.
	Ahead, Cap int
	// WaitedS is how long it waited, when it did.
	WaitedS int64
}

// forget drops the asks not asked again in askFor. The caller holds mu.
func (w *Watcher) forget(now time.Time) {
	kept := w.asks[:0]
	for _, a := range w.asks {
		if now.Sub(a.seen) <= askFor {
			kept = append(kept, a)
		}
	}
	for i := len(kept); i < len(w.asks); i++ {
		w.asks[i] = nil
	}
	w.asks = kept
}

// unask takes id out of the line and says since when it waited. The caller holds mu.
func (w *Watcher) unask(id string) (since time.Time) {
	for i, a := range w.asks {
		if a.id == id {
			w.asks = append(w.asks[:i], w.asks[i+1:]...)
			return a.since
		}
	}
	return time.Time{}
}

// Admit answers whether agent's Bash call id, a run of shape, may start now. A run goes at once unless a cap stands (CapFor) and
// its shape is heavy from its start; then it goes when fewer heavy runs than the cap are going and no earlier ask is still
// waiting, first come first served, or once it has waited MaxWait. The hook asks again every AskEvery while the answer is no.
// With no cap the answer comes from what is already known: nothing is read for it.
func (w *Watcher) Admit(agent, id, shape string) Answer {
	now := w.now()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.load()
	w.forget(now)
	limit := w.state.CapFor(w.cfg, w.cores)
	if limit == 0 || id == "" || !w.shapes.Heavy(shape) {
		w.unask(id)
		return Answer{Go: true}
	}
	if now.Sub(w.readAt) >= rereadEvery {
		w.readAt = now // this ask reads; one that comes meanwhile answers from what stands
		w.mu.Unlock()
		s := w.gather(now, false)
		w.mu.Lock()
		w.take(s)
		if limit = w.state.CapFor(w.cfg, w.cores); limit == 0 {
			w.unask(id)
			return Answer{Go: true}
		}
	}
	at := -1
	for i, a := range w.asks {
		if a.id == id {
			at = i
		}
	}
	if at < 0 {
		w.asks = append(w.asks, &ask{id: id, agent: agent, shape: shape, since: now})
		at = len(w.asks) - 1
	}
	a := w.asks[at]
	a.seen = now
	going := len(w.grants)
	for _, r := range w.runs {
		if r.Heavy && w.grants[r.ID].IsZero() {
			going++
		}
	}
	waited := now.Sub(a.since)
	if at < limit-going || waited >= MaxWait {
		w.unask(id)
		w.grants[id] = now
		if waited >= MaxWait {
			w.logf("run load: %s's %s goes after waiting %s, over the cap of %d", agent, shape, waited.Round(time.Second), limit)
		}
		return Answer{Go: true, WaitedS: int64(waited.Seconds())}
	}
	return Answer{Ahead: at, Cap: limit, WaitedS: int64(waited.Seconds())}
}

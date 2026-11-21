package turns

// View is an agent's turn as its tracker holds it, for a tracker on another machine: a runner's tracker watches its sessions and sends
// its View on each change, and the master's tracker holds it (SetView) so its readers see a runner's agent as one of its own.
type View struct {
	State      State  `json:"state"`
	At         int64  `json:"at"`
	Doing      *Doing `json:"doing,omitempty"`
	Background []Job  `json:"background,omitempty"`
	BgKnown    bool   `json:"bg_known"`
	Settled    bool   `json:"settled"`
	Live       bool   `json:"live"`  // its session process is watched there
	Ended      bool   `json:"ended"` // its session ended there
}

// ViewOf is agent's turn as this tracker holds it.
func (t *Tracker) ViewOf(agent string) View {
	t.mu.Lock()
	defer t.mu.Unlock()
	tr := t.agents[agent]
	if tr == nil {
		return View{}
	}
	v := View{State: tr.state, At: tr.at, BgKnown: tr.bgKnown, Settled: tr.settled, Live: tr.pid != 0 || tr.live, Ended: tr.ended,
		Background: append([]Job(nil), tr.background...)}
	if tr.state == Working {
		v.Doing = tr.doing
	}
	return v
}

// SetView takes agent's turn as the tracker on its machine holds it, in place of anything this one held, and tells OnChange's func.
func (t *Tracker) SetView(agent string, v View) {
	t.mu.Lock()
	tr := t.get(agent)
	tr.stopWatch()
	tr.stopPid()
	tr.set(v.State)
	tr.at, tr.doing, tr.background, tr.bgKnown, tr.settled = v.At, v.Doing, v.Background, v.BgKnown, v.Settled
	tr.live, tr.ended = v.Live, v.Ended
	changed := t.changed
	t.mu.Unlock()
	if changed != nil {
		changed(agent)
	}
}

// Forget drops agent's turn: its state is Unknown again, as after a restart (a runner gone offline).
func (t *Tracker) Forget(agent string) { t.SetView(agent, View{}) }

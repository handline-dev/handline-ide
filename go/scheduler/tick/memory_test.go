package tick

import (
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// fakeMemory is over the cap until a close acts on its reading; each agent may close for memory when it is idle, since its idle time.
type fakeMemory struct {
	over  bool
	idle  map[string]int64 // absent: may not close
	acted int
}

func (m *fakeMemory) Over(int64) (int64, int64, bool) { return 12 << 30, 8 << 30, m.over }
func (m *fakeMemory) MayClose(_ db.Q, agent, _ string, _ *db.AgentState, _ int64, _ Status) (bool, int64) {
	idle, ok := m.idle[agent]
	return ok, idle
}
func (m *fakeMemory) Acted() { m.acted++; m.over = false }

// Over the cap on the agents' memory the tick closes the session idle longest, one per reading; within it, none.
func TestOverTheMemoryCapTheLongestIdleSessionClosesOnePerReading(t *testing.T) {
	tk, w := newWorld(t)
	tk.Reconcile, tk.Sweeps, tk.Observe = &fakeReconciler{}, quietSweeps(), fakeObserve{}
	w.agents = []string{"a", "b", "c"}
	w.alive = map[string]bool{"a": true, "b": true, "c": true}
	w.closeOK = true
	mem := &fakeMemory{over: true, idle: map[string]int64{"a": 9_000, "b": 7_000}} // c is working: never a candidate
	tk.Memory = mem
	res, err := tk.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !w.did("close b") || w.did("close a") || w.did("close c") || mem.acted != 1 || !has(res.Actions, "b: session closed for memory") {
		t.Fatalf("the longest idle, alone: %v %v", w.calls, res.Actions)
	}
	w.calls = nil
	if _, err := tk.Run(); err != nil || w.did("close") {
		t.Fatalf("the same reading closes no second session: %v", w.calls)
	}
	// a refused close tries the next candidate and leaves the reading for the next tick; each refusal is said once
	mem.over, w.closeOK, w.closeWhy, w.calls = true, false, "typing elsewhere", nil
	tk.Run()
	tk.Run()
	said := 0
	for _, l := range w.logs {
		if strings.Contains(l, "not closed for memory - typing elsewhere") {
			said++
		}
	}
	if mem.acted != 1 || !w.did("close a") || said != 2 {
		t.Fatalf("refused: %d %v %v", mem.acted, w.calls, w.logs)
	}
	// an idle session closed past its window counts as a close: none closes for memory on the same reading
	w.shouldClose, w.closeOK, w.calls = true, true, nil
	res, _ = tk.Run()
	if mem.acted < 2 || has(res.Actions, "closed for memory") {
		t.Fatalf("closed for memory beside an idle close: %d %v", mem.acted, res.Actions)
	}
}

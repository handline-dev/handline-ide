package tick

import (
	"fmt"
	"sort"

	"github.com/handline/handline/go/data/db"
)

// Memory is the founder's cap on the agents' memory (agents_memory_max_gb): over it the tick closes the longest-idle session that may
// close, one per reading of the agents' memory.
type Memory interface {
	// Over is the agents' memory and the limit, in bytes, from the newest reading not yet acted on; ok is false when the cap is off, there
	// is no reading newer than the last close, or the agents are within the cap.
	Over(now int64) (used, limit int64, ok bool)
	// MayClose is whether the agent's session may close for memory (lifecycle's ShouldCloseForMemory) and since when it is idle.
	MayClose(q db.Q, agent, state string, row *db.AgentState, now int64, st Status) (bool, int64)
	// Acted says a session was closed, for memory or idle: the next close for memory waits for a reading taken after it.
	Acted()
}

type memoryCandidate struct {
	agent     string
	st        Status
	idleSince int64
}

// noteForMemory notes an agent whose session may close for memory; the tick closes one of them after visiting every agent.
func (t *Tick) noteForMemory(q db.Q, agent, state string, row *db.AgentState, now int64, st Status) {
	if ok, idle := t.Memory.MayClose(q, agent, state, row, now, st); ok {
		t.memCands = append(t.memCands, memoryCandidate{agent, st, idle})
	}
}

// closeForMemory closes the longest-idle candidate, trying the next when one refuses, once for this reading.
func (t *Tick) closeForMemory(actions *[]string) {
	if !t.memOver || len(t.memCands) == 0 {
		return
	}
	sort.SliceStable(t.memCands, func(i, j int) bool { return t.memCands[i].idleSince < t.memCands[j].idleSince })
	for _, c := range t.memCands {
		ok, why := t.Lifecycle.CloseAgent(t.Q, c.agent, c.st)
		if !ok {
			if t.saidClose["memory:"+c.agent] != why { // once per reason, as the idle close says its refusals
				t.saidClose["memory:"+c.agent] = why
				t.logf("  %s: not closed for memory - %s", c.agent, why)
			}
			continue
		}
		t.Memory.Acted()
		*actions = append(*actions, fmt.Sprintf("%s: session closed for memory", c.agent))
		t.logf("  %s: session closed, the longest idle: the agents use %.1f GB, over the %d GB set", c.agent, float64(t.memUsed)/(1<<30), t.memCap>>30)
		return
	}
}

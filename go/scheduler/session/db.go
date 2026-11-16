package session

import (
	"sort"

	"github.com/handline/handline/go/data/db"
)

// The states an agent is in (session/states.py).
const (
	Running  = "RUNNING"
	Handoff  = "HANDOFF"
	Stalled  = "STALLED"
	Idle     = "IDLE"
	Queued   = "QUEUED"
	Parked   = "PARKED"
	Degraded = "DEGRADED"
)

// AddEvent appends an event stamped now, its payload's keys in sorted order.
func (e *Env) AddEvent(agent, kind string, payload map[string]any) error {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	kv := make([]db.KV, len(keys))
	for i, k := range keys {
		kv[i] = db.KV{K: k, V: payload[k]}
	}
	_, err := db.AddEventAt(e.DB, e.now(), agent, kind, kv...)
	return err
}

// LastEventTS is when the agent last had an event of this kind, 0 when never.
func (e *Env) LastEventTS(agent, kind string) int64 {
	ts, _, _ := db.LastEventTS(e.DB, agent, kind)
	return ts
}

// Package rowstate is the words agent_state.state holds. It imports nothing, so the record path (hooks) and the model (states) name
// the same states.
package rowstate

const (
	Idle      = "IDLE"
	Queued    = "QUEUED"
	Running   = "RUNNING"
	Done      = "DONE"
	Handoff   = "HANDOFF"
	Degraded  = "DEGRADED"
	Parked    = "PARKED"
	Throttled = "THROTTLED"
	Stalled   = "STALLED"
)

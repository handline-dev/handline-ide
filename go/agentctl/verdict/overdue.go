package verdict

// Holder is who holds a task, as far as its time box cares.
type Holder int

const (
	AgentHolds   Holder = iota
	FounderHolds        // nothing an agent does moves the founder's wait, so it is never overdue
	GateHolds           // a gate closes the task at its own deadline
)

// Overdue is whether a task has run past its time box: it is doing, an agent holds it, it waits on no open blocker or child, and its
// deadline is at or before now. timeoutAt is 0 when no clock runs.
func Overdue(state string, holder Holder, waits bool, timeoutAt, now int64) bool {
	return state == "doing" && holder == AgentHolds && !waits && timeoutAt != 0 && timeoutAt <= now
}

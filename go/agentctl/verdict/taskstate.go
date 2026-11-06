package verdict

// TaskFacts is what a task's state is decided from. The db reads them for one task and the board for many; neither decides.
type TaskFacts struct {
	Discarded, Done bool // written by a discard or a done
	Begun           bool // begun by task_begin or task_self_start, not released since
	Sent            bool // its request was sent to its owner
	Held            bool // it is the task its owner holds now (the founder and the gates hold all of theirs)
	Blocked         bool // an open, not dormant, blocker
	Queued          bool // someone queued it
	PauseHolds      bool // new work is paused and its delivery has not reached the agent that acts on it
}

// Asked is whether the task's work has been asked for: sent to its owner, or begun.
func (f TaskFacts) Asked() bool { return f.Begun || f.Sent }

// TaskState is one of discarded, done, doing, queued, draft. Doing is asked for, held by its owner, unblocked and not held back by the
// pause; any other asked or queued open task is queued.
func TaskState(f TaskFacts) string {
	switch {
	case f.Discarded:
		return "discarded"
	case f.Done:
		return "done"
	case f.Asked() && f.Held && !f.Blocked && !f.PauseHolds:
		return "doing"
	case f.Queued || f.Asked():
		return "queued"
	}
	return "draft"
}

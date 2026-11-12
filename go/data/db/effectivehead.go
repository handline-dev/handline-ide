package db

// HeadTaskBlocked is delivery.head_task_blocked: is the head's own task on record as waited on, blocked by another open task or
// held up on an ask the agent itself sent that is linked to the same task. A head with no task is never this.
func HeadTaskBlocked(q Q, agent string, head *Request) (bool, error) {
	tasks, err := TasksForRequest(q, head.ID)
	if err != nil {
		return false, err
	}
	for _, t := range tasks {
		blockers, err := TaskBlockers(q, t.ID)
		if err != nil {
			return false, err
		}
		if len(blockers) > 0 {
			return true, nil
		}
		reqs, err := TaskRequests(q, t.ID)
		if err != nil {
			return false, err
		}
		for _, r := range reqs {
			if r.Requester == agent && r.ID != head.ID && r.State == "queued" {
				return true, nil
			}
		}
	}
	return false, nil
}

// EffectiveHead is HeadOfQueue past every head whose task is blocked on record, so the one-at-a-time rule falls on whatever the agent
// can actually act on. nil when nothing can be acted on.
func EffectiveHead(q Q, agent string) (*Request, error) {
	return firstUnblocked(q, agent, func(r *Request) bool { return r.Kind.String != "say" })
}

// EffectiveTaskHead is TaskHead past every task delivery whose task is blocked on record.
func EffectiveTaskHead(q Q, agent string) (*Request, error) {
	return firstUnblocked(q, agent, func(r *Request) bool { return r.Kind.String == "task" })
}

// firstUnblocked is the first queued request keep takes whose task is not blocked; as firstOf, one whose edge is unchecked stops the
// walk.
func firstUnblocked(q Q, agent string, keep func(*Request) bool) (*Request, error) {
	rows, err := QueuedFor(q, agent)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if !keep(r) {
			continue
		}
		if r.EdgeOK.Int64 == 0 {
			return nil, nil
		}
		if blocked, err := HeadTaskBlocked(q, agent, r); err != nil || !blocked {
			return r, err
		}
	}
	return nil, nil
}

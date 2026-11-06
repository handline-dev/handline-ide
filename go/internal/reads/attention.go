package reads

import (
	"fmt"
	"strings"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/scheduler/settings"
)

// RequestRow.attention (spec RequestAttention): the first of overdue, bounced, waiting and quiet that holds in a request's tree, read
// from its facts. Its text carries no elapsed time, so a row changes only when its facts do.

const (
	attnOverdue = "overdue"
	attnBounced = "bounced"
	attnWaiting = "waiting"
	attnQuiet   = "quiet"
)

// waitHold is what the supervisor's sweep recorded of why a task waits.
type waitHold struct {
	on    string
	since int64
}

// attention is what every request's attention is read from, read once per list.
type attention struct {
	e                       *Env
	clock                   int64
	waitS, quietS, sentBack int64
	holds                   map[int64]waitHold
	reopens                 map[int64]int64
}

func (e *Env) attentionFacts(clock int64) (*attention, error) {
	s := &settings.Settings{Home: e.Home}
	a := &attention{e: e, clock: clock, holds: map[int64]waitHold{}, reopens: map[int64]int64{},
		waitS: s.ConfigSeconds(settings.AttentionWaitKey, settings.AttentionWaitS), quietS: s.ConfigSeconds(settings.AttentionQuietKey, settings.AttentionQuietS),
		sentBack: s.ConfigSeconds(settings.AttentionSentBackKey, settings.AttentionSentBack)}
	holds, err := query(e.DB, "SELECT name, value FROM meta WHERE substr(name, 1, 9) = 'taskhold:'")
	if err != nil {
		return nil, err
	}
	for _, h := range holds {
		var tid int64
		if _, err := fmt.Sscanf(strings.TrimPrefix(h.str("name"), "taskhold:"), "%d", &tid); err != nil {
			continue
		}
		on, since := holdCause(decodeObject(h.str("value")))
		a.holds[tid] = waitHold{on: on, since: since}
	}
	// A send-back is a reopen (but a reopen of a discarded task is a restore) or a turn sent back (but not by a retiring agent).
	reopens, err := query(e.DB, "SELECT "+eventTask+" AS task, COUNT(*) AS n FROM events WHERE (kind = 'reopen_task' "+
		"AND COALESCE(CASE WHEN json_valid(payload) THEN json_extract(payload, '$.was') END, '') != 'discarded') OR (kind = 'task_turn_pop' "+
		"AND json_valid(payload) AND json_extract(payload, '$.outcome') = 'send_back' AND json_extract(payload, '$.note') != 'retired') GROUP BY task")
	if err != nil {
		return nil, err
	}
	for _, r := range reopens {
		a.reopens[r.i("task")] = r.i("n")
	}
	return a, nil
}

// of is request root's attention, nil when nothing calls for it. last is the tree's latest activity; cards, the tasks with a decision card
// out (they wait on the founder, which the row's status already says).
func (a *attention) of(root int64, tree []requestNode, last int64, cards map[int64]bool) (map[string]any, error) {
	say := func(kind string, task int64, text string) map[string]any {
		return map[string]any{"kind": kind, "text": text, "task": task}
	}
	var open []requestNode
	for _, n := range tree {
		if n.open {
			open = append(open, n)
		}
	}
	if len(open) == 0 {
		return nil, nil
	}
	clockRuns := false
	for _, n := range open {
		if n.due == 0 {
			continue
		}
		clockRuns = true // timeout_at is set only while the task is doing and not waiting
		if n.due > a.clock {
			continue
		}
		waits, err := db.TaskWaits(a.e.DB, n.id)
		if err != nil {
			return nil, err
		}
		if verdict.Overdue("doing", db.HolderOf(n.acting), waits, n.due, a.clock) {
			return say(attnOverdue, n.id, fmt.Sprintf("T%d past its time box", n.id)), nil
		}
	}
	var bounced *requestNode
	for i, n := range open {
		if a.reopens[n.id] >= a.sentBack && (bounced == nil || a.reopens[n.id] > a.reopens[bounced.id]) {
			bounced = &open[i]
		}
	}
	if bounced != nil {
		return say(attnBounced, bounced.id, fmt.Sprintf("T%d sent back %d times", bounced.id, a.reopens[bounced.id])), nil
	}
	for _, n := range open {
		h, held := a.holds[n.id]
		if n.blocked || cards[n.id] || !held || h.since == 0 || a.clock-h.since < a.waitS {
			continue
		}
		if h.on == "cap" || h.on == "wall" || h.on == "owner" {
			return say(attnWaiting, n.id, fmt.Sprintf("T%d waiting on %s", n.id, h.on)), nil
		}
	}
	// A task whose clock runs is being worked, though its turns leave no row the tree's activity counts.
	if !clockRuns && a.clock-last >= a.quietS {
		return say(attnQuiet, root, "no recent activity"), nil
	}
	return nil, nil
}

package tick

import (
	"fmt"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

// personUnstalled clears agent's stall, as agent.unstall does, when a person has written to it (a message, or a comment the daemon
// forwards) since it stalled: the person writing is the try again. Each request clears at most once, and stalling again moves the cursor
// past all written before, so three more failures stall it for good; an agent's message or comment never clears it.
func (t *Tick) personUnstalled(q db.Q, agent string, actions *[]string) bool {
	key := db.UnstallCursor(agent)
	seen := must(db.Cursor(q, key))
	var by *db.Request
	who := ""
	for _, r := range must(db.QueuedFor(q, agent)) {
		if r.ID <= seen || r.TypedAt.Valid || by != nil && r.ID < by.ID {
			continue
		}
		if p := personBehind(q, r); p != "" {
			by, who = r, p
		}
	}
	if by == nil {
		return false
	}
	check(db.ClearStall(q, agent, "cleared by a message from "+who))
	check(db.SetCursor(q, key, by.ID))
	must(db.AddEvent(q, agent, "stall_retried", db.KV{K: "by", V: who}, db.KV{K: "request", V: by.ID}))
	*actions = append(*actions, fmt.Sprintf("%s: STALLED -> QUEUED (a message from %s)", agent, who))
	return true
}

// personBehind is the person who wrote r: its requester, or the author of a comment it forwards; "" when no person did.
func personBehind(q db.Q, r *db.Request) string {
	if actor.IsPerson(r.Requester) {
		return r.Requester
	}
	for _, a := range must(db.CommentAuthors(q, r.ID)) {
		if actor.IsPerson(a) {
			return a
		}
	}
	return ""
}

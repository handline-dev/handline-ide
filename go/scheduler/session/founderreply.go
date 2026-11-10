package session

import (
	"database/sql"
	"fmt"

	"github.com/handline/handline/go/data/db"
)

// A founder's message is typed into the agent's terminal as a bare prompt, so an answer typed back in the terminal looks natural, and
// the founder never sees it. The Stop hook holds the agent once for each such message nothing answered.

// FounderNudgeKind is the event that marks a founder's message as nudged; its dedup key makes that once per message.
const FounderNudgeKind = "founder_nudged"

// FounderDeferKind marks a founder's message whose nudge a Stop paused for the session's own work put off, once per message: the
// message stays this turn's for the Stop after it, whatever that Stop's lists hold.
const FounderDeferKind = "founder_nudge_deferred"

// UnansweredFounder is the founder's typed message to agent that landed since agent's last recorded Stop or was deferred, was never nudged, and has no
// answer: no say or other request from agent to the founder since it landed, and no task made from it. 0 when none.
func (e *Env) UnansweredFounder(agent string) (int64, error) {
	var rid int64
	err := e.DB.QueryRow("SELECT r.id FROM requests r WHERE r.requester = ? AND r.target = ? AND r.kind = 'say' AND r.founder_auth = 1 "+
		"AND (r.landed_at >= (SELECT COALESCE(MAX(ts), 0) FROM events WHERE agent = ? AND kind IN ('Stop', 'StopFailure')) "+
		"OR EXISTS (SELECT 1 FROM events d WHERE d.dedup_key = ?||r.id)) "+
		"AND NOT EXISTS (SELECT 1 FROM task_requests l WHERE l.request_id = r.id) "+
		"AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.from_request = r.id) "+
		"AND NOT EXISTS (SELECT 1 FROM requests a WHERE a.requester = r.target AND a.target = ? AND a.ts >= r.landed_at) "+
		"AND NOT EXISTS (SELECT 1 FROM events e WHERE e.dedup_key = ?||r.id) "+
		"ORDER BY r.id DESC LIMIT 1", db.Human, agent, agent, founderDeferKey, db.Human, founderNudgeKey).Scan(&rid)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return rid, err
}

const founderNudgeKey, founderDeferKey = "founder_nudge:", "founder_nudge_deferred:"

// NoteFounderNudged marks the founder's message rid as nudged; false when it already was.
func (e *Env) NoteFounderNudged(agent string, rid int64) (bool, error) {
	return e.noteFounder(agent, FounderNudgeKind, founderNudgeKey, rid)
}

// NoteFounderDeferred marks the founder's message rid as deferred; false when it already was.
func (e *Env) NoteFounderDeferred(agent string, rid int64) (bool, error) {
	return e.noteFounder(agent, FounderDeferKind, founderDeferKey, rid)
}

// noteFounder adds the kind event for rid once, by its key; false when it already was.
func (e *Env) noteFounder(agent, kind, key string, rid int64) (bool, error) {
	id, err := db.AddEventDeduped(e.DB, agent, kind, fmt.Sprintf("%s%d", key, rid), db.KV{K: "request_id", V: rid})
	return id != 0, err
}

// FounderNudgeText is the feedback the Stop hook gives for holding the agent.
func FounderNudgeText(name string) string {
	return fmt.Sprintf("The founder's message %s has no answer: your terminal text is not shown to them. "+
		"Answer it with hl say, then stop.", name)
}

// FounderNudge is the Stop hook's nudge: the text that keeps the turn going, once per message, when a founder's message typed in this turn
// has no answer (UnansweredFounder), else "". A Stop the hook already held (stop_hook_active) and any error pass. A Stop that only pauses
// until its own background work or wakeup brings the session back (pausedForOwnWork) puts the nudge off once per message, so the Stop
// after it nudges whatever its lists hold: a recurring cron or a monitor is listed on every Stop.
func (e *Env) FounderNudge(agent string, raw map[string]any) (out string) {
	defer func() {
		if recover() != nil {
			out = ""
		}
	}()
	if active, _ := raw["stop_hook_active"].(bool); active {
		return ""
	}
	rid, err := e.UnansweredFounder(agent)
	if err != nil || rid == 0 {
		return ""
	}
	if pausedForOwnWork(raw) {
		if first, err := e.NoteFounderDeferred(agent, rid); err != nil || first {
			return ""
		}
	}
	if first, err := e.NoteFounderNudged(agent, rid); err != nil || !first {
		return ""
	}
	return FounderNudgeText(db.MsgName(e.DB, rid))
}

// pausedForOwnWork: the Stop input lists a background task still in flight or a session wakeup still scheduled, so the turn ends only
// until that brings the session back. Claude Code leaves both lists out where its task registry cannot be read.
func pausedForOwnWork(raw map[string]any) bool {
	tasks, _ := raw["background_tasks"].([]any)
	crons, _ := raw["session_crons"].([]any)
	return len(tasks) > 0 || len(crons) > 0
}

package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AgentState is one agent_state row.
type AgentState struct {
	Agent, State                                                        string
	WakeAt, ClosedAt, CurrentTask                                       sql.NullInt64
	Detail, Terminal, Worktree, Account, ResumeSession, Task            sql.NullString
	Handoff, AccountSource, LaunchParts                                 sql.NullString
	NotifiedSession, StallKind, ParkReason, StallArgs                   sql.NullString
	Updated, FailedAttempts, Continuations, ThrottleCount, WakeFailures int64
}

const stateCols = "agent, state, wake_at, detail, updated, failed_attempts, continuations, throttle_count, wake_failures, terminal, worktree, " +
	"account, closed_at, resume_session, task, handoff, account_source, launch_parts, notified_session, current_task, stall_kind, park_reason, stall_args"

type scanner interface{ Scan(...any) error }

func scanState(r scanner) (*AgentState, error) {
	s := &AgentState{}
	err := r.Scan(&s.Agent, &s.State, &s.WakeAt, &s.Detail, &s.Updated, &s.FailedAttempts, &s.Continuations, &s.ThrottleCount, &s.WakeFailures,
		&s.Terminal, &s.Worktree, &s.Account, &s.ClosedAt, &s.ResumeSession, &s.Task, &s.Handoff, &s.AccountSource, &s.LaunchParts,
		&s.NotifiedSession, &s.CurrentTask, &s.StallKind, &s.ParkReason, &s.StallArgs)
	return s, err
}

// GetAgentState is the agent's agent_state row, nil when it has none.
func GetAgentState(q Q, agent string) (*AgentState, error) {
	s, err := scanState(q.QueryRow("SELECT "+stateCols+" FROM agent_state WHERE agent = ?", agent))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// AllAgentStates is every agent_state row, by agent.
func AllAgentStates(q Q) ([]*AgentState, error) {
	return stateRows(q, "SELECT "+stateCols+" FROM agent_state ORDER BY agent")
}

// DueAgents is the agents whose wake_at has come, earliest first.
func DueAgents(q Q, now int64) ([]*AgentState, error) {
	return stateRows(q, "SELECT "+stateCols+" FROM agent_state WHERE wake_at IS NOT NULL AND wake_at <= ? ORDER BY wake_at", now)
}

func stateRows(q Q, query string, args ...any) ([]*AgentState, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AgentState
	for rows.Next() {
		s, err := scanState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int64, set bool) any {
	if !set {
		return nil
	}
	return n
}

// SetTask records the free-text prompt a spawned terminal starts on.
func SetTask(q Q, agent, task string) error {
	_, err := q.Exec("INSERT INTO agent_state(agent, state, updated, task) VALUES(?,?,?,?) ON CONFLICT(agent) DO UPDATE SET task=excluded.task, updated=excluded.updated",
		agent, "IDLE", Now(), nullStr(task))
	return err
}

// SetCurrentTask records the one board task the agent holds now. Giving it a task ends the idle spell its
// agent-level progress note was written in. Passing set false clears it.
func SetCurrentTask(q Q, agent string, tid int64, set bool) error {
	if set {
		if _, err := q.Exec("INSERT INTO meta(name, value, updated) VALUES(?,?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value, updated=excluded.updated",
			AgentProgressKey(agent), nil, Now()); err != nil {
			return err
		}
	}
	_, err := q.Exec("INSERT INTO agent_state(agent, state, updated, current_task) VALUES(?,?,?,?) ON CONFLICT(agent) DO UPDATE SET current_task=excluded.current_task, updated=excluded.updated",
		agent, "IDLE", Now(), nullInt(tid, set))
	return err
}

// CurrentTask is the board task the agent holds now; ok is false when it holds none.
func CurrentTask(q Q, agent string) (int64, bool, error) {
	var n sql.NullInt64
	err := q.QueryRow("SELECT current_task FROM agent_state WHERE agent = ?", agent).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return n.Int64, n.Valid, err
}

// SetAgentState writes the agent's state; wakeAt 0 and detail "" write NULL. It clears the stall kind and the park reason: SetStalled and
// SetParked are the ones that keep one.
func SetAgentState(q Q, agent, state string, wakeAt int64, detail string) error {
	return SetParked(q, agent, state, wakeAt, "", detail)
}

// SetParked is SetAgentState with why the daemon parked or queued the agent (a verdict.ParkReason word) beside its detail.
func SetParked(q Q, agent, state string, wakeAt int64, reason, detail string) error {
	_, err := q.Exec("INSERT INTO agent_state(agent, state, wake_at, detail, updated, park_reason) VALUES(?,?,?,?,?,?) ON CONFLICT(agent) DO UPDATE SET state=excluded.state, wake_at=excluded.wake_at, detail=excluded.detail, updated=excluded.updated, stall_kind=NULL, stall_args=NULL, park_reason=excluded.park_reason",
		agent, state, nullInt(wakeAt, wakeAt != 0), nullStr(detail), Now(), nullStr(reason))
	if err == nil && state == "STALLED" {
		err = stallSeen(q, agent)
	}
	return err
}

// UnstallCursor names the cursor past which a person's request to agent clears its stall: what was written before the stall does not.
func UnstallCursor(agent string) string { return "unstall_by:" + agent }

// stallSeen moves agent's UnstallCursor to the newest request, as it stalls.
func stallSeen(q Q, agent string) error {
	_, err := q.Exec("INSERT INTO cursors(name, row_id, updated) SELECT ?, COALESCE(MAX(id), 0), ? FROM requests WHERE true "+
		"ON CONFLICT(name) DO UPDATE SET row_id=excluded.row_id, updated=excluded.updated", UnstallCursor(agent), Now())
	return err
}

// SetStalled writes STALLED with the kind of the stall (a verdict.StuckKind word) beside its detail, and the facts the card shows (the
// folder, what the dialog asked, why) as args, so no screen cuts them out of the detail's words.
func SetStalled(q Q, agent, kind, detail string, args map[string]string) error {
	var js any
	if len(args) > 0 {
		b, err := json.Marshal(args)
		if err != nil {
			return err
		}
		js = string(b)
	}
	_, err := q.Exec("INSERT INTO agent_state(agent, state, wake_at, detail, updated, stall_kind, stall_args) VALUES(?,'STALLED',NULL,?,?,?,?) ON CONFLICT(agent) DO UPDATE SET state=excluded.state, wake_at=NULL, detail=excluded.detail, updated=excluded.updated, stall_kind=excluded.stall_kind, stall_args=excluded.stall_args, park_reason=NULL",
		agent, nullStr(detail), Now(), nullStr(kind), js)
	if err == nil {
		err = stallSeen(q, agent)
	}
	return err
}

// ClearStall puts an agent back in the queue with detail as its word, its failure counters at 0 and its dialog marks gone.
func ClearStall(q Q, agent, detail string) error {
	if err := SetAgentState(q, agent, "QUEUED", 0, detail); err != nil {
		return err
	}
	for _, col := range []string{"failed_attempts", "throttle_count", "wake_failures"} {
		if err := ResetCounter(q, agent, col); err != nil {
			return err
		}
	}
	for _, key := range []string{"dialog_since:", "dialog_title:"} {
		if err := ClearCursor(q, key+agent); err != nil {
			return err
		}
	}
	return nil
}

// StallFacts are the facts the stall was raised with, empty when it was raised without any (or before they were kept).
func (s *AgentState) StallFacts() map[string]string {
	out := map[string]string{}
	if s != nil && s.StallArgs.Valid {
		_ = json.Unmarshal([]byte(s.StallArgs.String), &out)
	}
	return out
}

// SetStateUnlessRunning writes the agent's state unless it is RUNNING, true only when this call wrote it.
func SetStateUnlessRunning(q Q, agent, state, detail string) (bool, error) {
	res, err := q.Exec("UPDATE agent_state SET state = ?, wake_at = NULL, detail = ?, updated = ?, stall_kind = NULL, stall_args = NULL, park_reason = NULL WHERE agent = ? AND state != 'RUNNING'",
		state, nullStr(detail), Now(), agent)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetTerminal records where an agent lives (its terminal and worktree); a new agent is IDLE.
func SetTerminal(q Q, agent, terminal, worktree string) error {
	_, err := q.Exec("INSERT INTO agent_state(agent, state, updated, terminal, worktree) VALUES(?,?,?,?,?) ON CONFLICT(agent) DO UPDATE SET terminal=excluded.terminal, worktree=excluded.worktree, updated=excluded.updated",
		agent, "IDLE", Now(), nullStr(terminal), nullStr(worktree))
	if err != nil || terminal == "" {
		return err
	}
	return RecordTerminal(q, terminal, agent)
}

// RecordTerminal remembers a terminal opened for agent ("": opened with no agent), beside the ones it had
// before. The same terminal named again for the same agent, still open, keeps when it was first opened: a launch names the terminal
// its spawn has just named.
func RecordTerminal(q Q, handle, agent string) error {
	_, err := q.Exec("INSERT INTO agent_terminals(handle, agent, opened_at) VALUES(?,?,?) ON CONFLICT(handle) DO UPDATE SET "+
		"opened_at=CASE WHEN agent IS excluded.agent AND closed_at IS NULL THEN opened_at ELSE excluded.opened_at END, "+
		"agent=excluded.agent, closed_at=NULL", handle, nullStr(agent), Now())
	return err
}

// OpenTerminal is a terminal opened for an agent and not yet seen closed.
type OpenTerminal struct {
	Handle, Agent string
	OpenedAt      int64
}

// OpenAgentTerminals is every terminal opened for an agent and not yet closed; one opened with no agent is not among them.
func OpenAgentTerminals(q Q) ([]OpenTerminal, error) {
	rows, err := q.Query("SELECT handle, agent, opened_at FROM agent_terminals WHERE agent IS NOT NULL AND closed_at IS NULL ORDER BY opened_at, handle")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OpenTerminal
	for rows.Next() {
		var t OpenTerminal
		if err := rows.Scan(&t.Handle, &t.Agent, &t.OpenedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CloseAgentTerminal records that the terminal is closed.
func CloseAgentTerminal(q Q, handle string) error {
	_, err := q.Exec("UPDATE agent_terminals SET closed_at = ? WHERE handle = ? AND closed_at IS NULL", Now(), handle)
	return err
}

// EnsureAgentState gives agent an IDLE row when it has none, so the count of held slots (read from agent_state) sees its first launch.
func EnsureAgentState(q Q, agent string) error {
	_, err := q.Exec("INSERT INTO agent_state(agent, state, updated) VALUES(?,?,?) ON CONFLICT(agent) DO NOTHING", agent, "IDLE", Now())
	return err
}

// SetAccount records the account the agent runs on; source is "recorded" or "assumed".
func SetAccount(q Q, agent, account, source string) error {
	_, err := q.Exec("INSERT INTO agent_state(agent, state, updated, account, account_source) VALUES(?,?,?,?,?) ON CONFLICT(agent) DO UPDATE SET account=excluded.account, account_source=excluded.account_source, updated=excluded.updated",
		agent, "IDLE", Now(), nullStr(account), source)
	return err
}

// SetLaunchParts keeps what the agent's session was launched with (launch_parts).
func SetLaunchParts(q Q, agent, parts string) error {
	_, err := q.Exec("INSERT INTO agent_state(agent, state, updated, launch_parts) VALUES(?,?,?,?) ON CONFLICT(agent) DO UPDATE SET launch_parts=excluded.launch_parts, updated=excluded.updated",
		agent, "IDLE", Now(), nullStr(parts))
	return err
}

// The names a launch record (launch_parts) gives an agent's role and standing context.
const (
	LaunchedRole = "role"
	LaunchedBase = "standing context"
)

// oldLaunchedNames are the names records written before the instructions moved into the database gave those parts.
var oldLaunchedNames = map[string]string{"ROLE.md": LaunchedRole, "base.md": LaunchedBase}

// LaunchPartsOf is a launch record by part, nil when s is not one. A part under its old name, and its kept text, read as today's.
func LaunchPartsOf(s string) map[string]json.RawMessage {
	var rec map[string]json.RawMessage
	if json.Unmarshal([]byte(s), &rec) != nil || rec == nil {
		return nil
	}
	rename := func(m map[string]json.RawMessage) bool {
		moved := false
		for old, now := range oldLaunchedNames {
			if v, ok := m[old]; ok {
				if _, has := m[now]; !has {
					m[now] = v
				}
				delete(m, old)
				moved = true
			}
		}
		return moved
	}
	rename(rec)
	var texts map[string]json.RawMessage
	if json.Unmarshal(rec["texts"], &texts) == nil && rename(texts) {
		rec["texts"], _ = json.Marshal(texts)
	}
	return rec
}

// ReplaceLaunchParts puts parts in place of was as the agent's launch record; a record that is no longer was is left as it is.
func ReplaceLaunchParts(q Q, agent, was, parts string) error {
	_, err := q.Exec("UPDATE agent_state SET launch_parts = ?, updated = ? WHERE agent = ? AND launch_parts = ?", parts, Now(), agent, was)
	return err
}

// SetClosed records that the session was closed, and this is how to get it back.
func SetClosed(q Q, agent, session string) error {
	_, err := q.Exec("UPDATE agent_state SET closed_at = ?, resume_session = ?, updated = ? WHERE agent = ?", Now(), nullStr(session), Now(), agent)
	return err
}

// SetResumeSession records the session a structured agent runs under: the id a restart or a wake resumes. A closed mark is not touched.
func SetResumeSession(q Q, agent, session string) error {
	_, err := q.Exec("UPDATE agent_state SET resume_session = ?, updated = ? WHERE agent = ?", nullStr(session), Now(), agent)
	return err
}

// ForgetResume leaves the agent closed still, with no session to resume.
func ForgetResume(q Q, agent string) error {
	_, err := q.Exec("UPDATE agent_state SET resume_session = NULL, updated = ? WHERE agent = ?", Now(), agent)
	return err
}

// ClearClosed clears the closed mark: a session runs again, a new process, told its queue afresh unless a tell was recorded after the close.
func ClearClosed(q Q, agent string) error {
	var closed sql.NullInt64
	err := q.QueryRow("SELECT closed_at FROM agent_state WHERE agent = ?", agent).Scan(&closed)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	told, ok, err := CursorUpdated(q, "notify:"+agent)
	if err != nil {
		return err
	}
	if !(closed.Valid && closed.Int64 != 0 && ok && told != 0 && told > closed.Int64) {
		if err := ForgetTold(q, agent); err != nil {
			return err
		}
	}
	_, err = q.Exec("UPDATE agent_state SET closed_at = NULL, resume_session = NULL, updated = ? WHERE agent = ?", Now(), agent)
	return err
}

var counters = map[string]bool{"failed_attempts": true, "continuations": true, "throttle_count": true, "wake_failures": true}

// BumpCounter adds by to one of the agent's four counters, named by column.
func BumpCounter(q Q, agent, column string, by int64) error {
	if !counters[column] {
		return fmt.Errorf("not a counter column: %s", column)
	}
	_, err := q.Exec(fmt.Sprintf("INSERT INTO agent_state(agent, state, updated, %s) VALUES(?,?,?,?) ON CONFLICT(agent) DO UPDATE SET %s = agent_state.%s + ?, updated = ?", column, column, column),
		agent, "IDLE", Now(), by, by, Now())
	return err
}

// ResetCounter sets one of the agent's four counters, named by column, to 0.
func ResetCounter(q Q, agent, column string) error {
	if !counters[column] {
		return fmt.Errorf("not a counter column: %s", column)
	}
	_, err := q.Exec(fmt.Sprintf("UPDATE agent_state SET %s = 0 WHERE agent = ?", column), agent)
	return err
}

// Terminal is the agent's terminal, "" when none is recorded.
func Terminal(q Q, agent string) (string, error) {
	return textOf(q, "SELECT terminal FROM agent_state WHERE agent = ?", agent)
}

// textOf is the one text value query reads for args, "" when it finds no row or a NULL.
func textOf(q Q, query string, args ...any) (string, error) {
	var t sql.NullString
	err := q.QueryRow(query, args...).Scan(&t)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return t.String, err
}

// AgentOfTerminal is the agent last recorded in this terminal.
func AgentOfTerminal(q Q, terminal string) (string, error) {
	var a string
	err := q.QueryRow("SELECT agent FROM agent_state WHERE terminal = ? ORDER BY updated DESC LIMIT 1", terminal).Scan(&a)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return a, err
}

// ActivityKinds are the events that move an agent's activity clock: its own turns and commands, never what Handline does to it.
var ActivityKinds = []string{"ask", "cancel", "commit", "raise_decision", "report", "resolve", "withdraw", "no_task",
	"task_new", "task_start", "task_note", "task_begin", "task_handoff",
	"UserPromptSubmit", "Stop", "StopFailure", "SubagentStop", "PostCompact", "PermissionRequest", "TeammateIdle"}

func marks(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			s += ","
		}
		s += "?"
	}
	return s
}

func maxInt(q Q, query string, args ...any) (int64, bool, error) {
	var n sql.NullInt64
	if err := q.QueryRow(query, args...).Scan(&n); err != nil {
		return 0, false, err
	}
	return n.Int64, n.Valid, nil
}

func kindArgs(prefix []any, kinds []string) []any {
	args := append([]any{}, prefix...)
	for _, k := range kinds {
		args = append(args, k)
	}
	return args
}

// LastActivity is when the agent last did anything we can see (agent_heard included).
func LastActivity(q Q, agent string) (int64, bool, error) {
	return maxInt(q, "SELECT MAX(t) FROM (SELECT MAX(ts) AS t FROM events WHERE agent = ? AND kind IN ("+marks(len(ActivityKinds))+
		") UNION ALL SELECT at FROM agent_heard WHERE agent = ?)", append(kindArgs([]any{agent}, ActivityKinds), agent)...)
}

// LastActivityBefore is the newest activity event before ts (agent_heard keeps no history).
func LastActivityBefore(q Q, agent string, ts int64) (int64, bool, error) {
	return maxInt(q, "SELECT MAX(ts) FROM events WHERE agent = ? AND ts < ? AND kind IN ("+marks(len(ActivityKinds))+")", kindArgs([]any{agent, ts}, ActivityKinds)...)
}

// LastActivityID is the id of the agent's latest event of one of kinds.
func LastActivityID(q Q, agent string, kinds []string) (int64, bool, error) {
	return maxInt(q, "SELECT MAX(id) FROM events WHERE agent = ? AND kind IN ("+marks(len(kinds))+")", kindArgs([]any{agent}, kinds)...)
}

// accountActivitySQL reads through events_account_ts (migration 33), the account's own events only.
const accountActivitySQL = "SELECT MAX(ts) FROM events WHERE account = ? AND kind IN ('UserPromptSubmit', 'Stop', 'StopFailure', 'PostCompact', 'SessionStart')"

// LastAccountActivity is when a session on the account last took a turn.
func LastAccountActivity(q Q, account string) (int64, bool, error) {
	return maxInt(q, accountActivitySQL, account)
}

// LastEventTS is when the agent's latest event of kind was.
func LastEventTS(q Q, agent, kind string) (int64, bool, error) {
	return maxInt(q, "SELECT MAX(ts) FROM events WHERE agent = ? AND kind = ?", agent, kind)
}

// LastEventOfTS is when the agent's latest event of any of kinds was.
func LastEventOfTS(q Q, agent string, kinds []string) (int64, bool, error) {
	args := []any{agent}
	for _, k := range kinds {
		args = append(args, k)
	}
	return maxInt(q, "SELECT MAX(ts) FROM events WHERE agent = ? AND kind IN (?"+strings.Repeat(", ?", len(kinds)-1)+")", args...)
}

// LaunchSession is the session of the agent's latest launch, as its own hooks reported it after that launch began (by event id, since a
// late hook keeps its own time); "" while that session has reported nothing, so a hook of the session before is never taken for it.
func LaunchSession(q Q, agent string) (string, error) {
	var id sql.NullString
	err := q.QueryRow("SELECT json_extract(payload, '$.session_id') FROM events WHERE agent = ?1 AND kind IN "+
		"('SessionStart', 'UserPromptSubmit', 'Stop', 'StopFailure', 'PostCompact') AND json_valid(payload) "+
		"AND json_extract(payload, '$.session_id') IS NOT NULL "+
		"AND id > COALESCE((SELECT MAX(id) FROM events WHERE agent = ?1 AND kind = 'launching'), 0) ORDER BY id DESC LIMIT 1", agent).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id.String, err
}

// NoWorktree is the event of a start that found no worktree to start in (its make or checkout failed): making it is over.
const NoWorktree = "no_worktree"

// MakingWorktree is the event of an agent's worktree being made before its start, which the view reads as making its worktree.
const MakingWorktree = "making_worktree"

// LaunchOver are the events that say a launch is no longer starting: a hook of its session (it is up), its end, or the wake that failed.
var LaunchOver = []string{"SessionStart", "UserPromptSubmit", "Stop", "StopFailure", "PostCompact", "SessionEnd", "wake_failed"}

// FirstEventOfTS is when the agent's first event of any of kinds at or after since was.
func FirstEventOfTS(q Q, agent string, kinds []string, since int64) (int64, bool, error) {
	return maxInt(q, "SELECT MIN(ts) FROM events WHERE agent = ? AND kind IN (?"+strings.Repeat(", ?", len(kinds)-1)+") AND ts >= ?",
		append(kindArgs([]any{agent}, kinds), since)...)
}

// LastSessionStartTS is when the agent's latest kept session start (hl launch, just before it becomes claude) was.
func LastSessionStartTS(q Q, agent string) (int64, bool, error) {
	return maxInt(q, "SELECT MAX(ts) FROM session_starts WHERE agent = ?", agent)
}

// LastEventID is the id of the agent's latest event of kind.
func LastEventID(q Q, agent, kind string) (int64, bool, error) {
	return maxInt(q, "SELECT MAX(id) FROM events WHERE agent = ? AND kind = ?", agent, kind)
}

// LastEvent is the id and payload of the agent's latest event of kind; ok is false when there is none. A payload that is
// not a JSON object reads as empty.
func LastEvent(q Q, agent, kind string) (id int64, payload map[string]any, ok bool, err error) {
	var p sql.NullString
	err = q.QueryRow("SELECT id, payload FROM events WHERE agent = ? AND kind = ? ORDER BY id DESC LIMIT 1", agent, kind).Scan(&id, &p)
	if err == sql.ErrNoRows {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, err
	}
	payload = map[string]any{}
	if p.String != "" {
		var m map[string]any
		if json.Unmarshal([]byte(p.String), &m) == nil && m != nil {
			payload = m
		}
	}
	return id, payload, true, nil
}

// MaxEventID is the newest event's id, 0 when there are none.
func MaxEventID(q Q) (int64, error) {
	n, _, err := maxInt(q, "SELECT MAX(id) FROM events")
	return n, err
}

// HasEventAfter is whether the agent has an event of kind newer than event id since.
func HasEventAfter(q Q, since int64, agent, kind string) (bool, error) {
	var n int
	err := q.QueryRow("SELECT COUNT(*) FROM events WHERE id > ? AND agent = ? AND kind = ?", since, agent, kind).Scan(&n)
	return n > 0, err
}

// AgentNode is the machine agent's session was last placed on (agent_state.node), "" for this one or none.
func AgentNode(q Q, agent string) (string, error) {
	return textOf(q, "SELECT node FROM agent_state WHERE agent = ?", agent)
}

// ClearNode forgets the machine agent's session was placed on.
func ClearNode(q Q, agent string) error {
	_, err := q.Exec("UPDATE agent_state SET node = NULL WHERE agent = ?", agent)
	return err
}

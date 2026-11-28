package db

import (
	"database/sql"
	"sort"
	"strings"
	"time"
)

// SubagentStart is the event a subagent's start is recorded as, by the daemon when the PreToolUse guard lets an Agent call through; its
// end is the session's own SubagentStop hook event.
const SubagentStart = "SubagentStart"

// SubagentMeta is what the Agent call said of the subagent: its description, its subagent_type, and whether it runs in the background.
type SubagentMeta struct {
	Description, Type string
	Background        bool
}

// RecordSubagentStart writes a subagent start of agent's session on acct ("" when unknown) at at.
func RecordSubagentStart(q Q, agent, acct, session string, at int64, m SubagentMeta) error {
	var a any
	if acct != "" {
		a = acct
	}
	_, err := q.Exec("INSERT INTO events(ts, agent, account, kind, payload) VALUES(?,?,?,?,json_object('session_id', ?, 'description', ?, "+
		"'subagent_type', ?, 'background', json(?)))", at, agent, a, SubagentStart, session, m.Description, m.Type, boolJSON(m.Background))
	return err
}

func boolJSON(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// OpenSubagent is a recorded subagent start.
type OpenSubagent struct {
	Agent, Account string
	At             int64
	SubagentMeta
}

// Label is what the subagent is, in at most 100 characters: the call's description, else its subagent_type, else "subagent".
func (s OpenSubagent) Label() string {
	l := strings.Join(strings.Fields(s.Description), " ")
	if l == "" {
		l = s.Type
	}
	if l == "" {
		l = "subagent"
	}
	if r := []rune(l); len(r) > 100 {
		l = string(r[:99]) + "…"
	}
	return l
}

// openSubagents walks agent's subagent events from from on: its starts that nothing has ended, while its row is live. A start
// runs until a SubagentStop of the same session (the oldest open one ends first), or until that session ends: its SessionEnd, a
// SessionStart of another session of the agent, or the agent's row closed.
func openSubagents(q Q, agent string, from int64) (open []OpenSubagent, err error) {
	rows, err := q.Query("SELECT agent, COALESCE(account, ''), kind, ts, COALESCE(json_extract(payload, '$.session_id'), ''), "+
		"COALESCE(json_extract(payload, '$.description'), ''), COALESCE(json_extract(payload, '$.subagent_type'), ''), "+
		"COALESCE(json_extract(payload, '$.background'), 0) FROM events "+
		"WHERE +agent = ? AND kind IN (?, 'SubagentStop', 'SessionStart', 'SessionEnd') AND ts >= ? AND json_valid(payload) ORDER BY id",
		agent, SubagentStart, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	live := map[string]map[string][]OpenSubagent{} // agent -> session -> its starts not yet stopped
	for rows.Next() {
		var s OpenSubagent
		var kind, session string
		if err := rows.Scan(&s.Agent, &s.Account, &kind, &s.At, &session, &s.Description, &s.Type, &s.Background); err != nil {
			return nil, err
		}
		if live[s.Agent] == nil {
			live[s.Agent] = map[string][]OpenSubagent{}
		}
		switch kind {
		case SubagentStart:
			live[s.Agent][session] = append(live[s.Agent][session], s)
		case "SubagentStop":
			if l := live[s.Agent][session]; len(l) > 0 {
				live[s.Agent][session] = l[1:]
			}
		case "SessionEnd":
			delete(live[s.Agent], session)
		case "SessionStart":
			for other := range live[s.Agent] {
				if other != session {
					delete(live[s.Agent], other)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for agent, sessions := range live {
		var closed sql.NullInt64
		if err := q.QueryRow("SELECT closed_at FROM agent_state WHERE agent = ?", agent).Scan(&closed); err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if closed.Int64 != 0 {
			continue
		}
		for _, l := range sessions {
			open = append(open, l...)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].At < open[j].At })
	return open, nil
}

// RunningSubagents are agent's subagents running at now, oldest first, read over the last day.
func RunningSubagents(q Q, agent string, now time.Time) ([]OpenSubagent, error) {
	return openSubagents(q, agent, now.Unix()-86400) // its own rows only: one agent's read of every agent's is paid per agent
}

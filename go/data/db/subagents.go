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

// SubagentCount is how many subagents run now and how many started since the day began.
type SubagentCount struct {
	Running int `json:"running"`
	Today   int `json:"today"`
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

// openSubagents walks the subagent events from from on: every start, and those of agents with a live row that nothing has ended. A start
// runs until a SubagentStop of the same session (the oldest open one ends first), or until that session ends: its SessionEnd, a
// SessionStart of another session of the agent, or the agent's row closed.
func openSubagents(q Q, from int64) (starts, open []OpenSubagent, err error) {
	rows, err := q.Query("SELECT agent, COALESCE(account, ''), kind, ts, COALESCE(json_extract(payload, '$.session_id'), ''), "+
		"COALESCE(json_extract(payload, '$.description'), ''), COALESCE(json_extract(payload, '$.subagent_type'), ''), "+
		"COALESCE(json_extract(payload, '$.background'), 0) FROM events "+
		"WHERE kind IN (?, 'SubagentStop', 'SessionStart', 'SessionEnd') AND ts >= ? AND json_valid(payload) ORDER BY id", SubagentStart, from)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	live := map[string]map[string][]OpenSubagent{} // agent -> session -> its starts not yet stopped
	for rows.Next() {
		var s OpenSubagent
		var kind, session string
		if err := rows.Scan(&s.Agent, &s.Account, &kind, &s.At, &session, &s.Description, &s.Type, &s.Background); err != nil {
			return nil, nil, err
		}
		if live[s.Agent] == nil {
			live[s.Agent] = map[string][]OpenSubagent{}
		}
		switch kind {
		case SubagentStart:
			starts = append(starts, s)
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
		return nil, nil, err
	}
	for agent, sessions := range live {
		var closed sql.NullInt64
		if err := q.QueryRow("SELECT closed_at FROM agent_state WHERE agent = ?", agent).Scan(&closed); err != nil && err != sql.ErrNoRows {
			return nil, nil, err
		}
		if closed.Int64 != 0 {
			continue
		}
		for _, l := range sessions {
			open = append(open, l...)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].At < open[j].At })
	return starts, open, nil
}

// SubagentCounts are the subagent counts per agent and per account (by the account each start was recorded on) at now (openSubagents).
// The last day is read, so a start older than that counts as ended.
func SubagentCounts(q Q, now time.Time) (agents, accounts map[string]SubagentCount, err error) {
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix()
	starts, open, err := openSubagents(q, min(midnight, now.Unix()-86400))
	if err != nil {
		return nil, nil, err
	}
	agents, accounts = map[string]SubagentCount{}, map[string]SubagentCount{}
	for _, s := range starts {
		if s.At >= midnight {
			a, c := agents[s.Agent], accounts[s.Account]
			a.Today, c.Today = a.Today+1, c.Today+1
			agents[s.Agent], accounts[s.Account] = a, c
		}
	}
	for _, s := range open {
		a, c := agents[s.Agent], accounts[s.Account]
		a.Running, c.Running = a.Running+1, c.Running+1
		agents[s.Agent], accounts[s.Account] = a, c
	}
	delete(accounts, "")
	return agents, accounts, nil
}

// RunningSubagents are agent's subagents running at now, oldest first, read over the last day as SubagentCounts reads them.
func RunningSubagents(q Q, agent string, now time.Time) ([]OpenSubagent, error) {
	_, open, err := openSubagents(q, now.Unix()-86400)
	var out []OpenSubagent
	for _, s := range open {
		if s.Agent == agent {
			out = append(out, s)
		}
	}
	return out, err
}

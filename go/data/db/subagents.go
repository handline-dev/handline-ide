package db

import (
	"database/sql"
	"time"
)

// SubagentStart is the event a subagent's start is recorded as, by the daemon when the PreToolUse guard lets an Agent call through; its
// end is the session's own SubagentStop hook event.
const SubagentStart = "SubagentStart"

// RecordSubagentStart writes a subagent start of agent's session on acct ("" when unknown) at at.
func RecordSubagentStart(q Q, agent, acct, session string, at int64) error {
	var a any
	if acct != "" {
		a = acct
	}
	_, err := q.Exec("INSERT INTO events(ts, agent, account, kind, payload) VALUES(?,?,?,?,json_object('session_id', ?))", at, agent, a, SubagentStart, session)
	return err
}

// SubagentCount is how many subagents run now and how many started since the day began.
type SubagentCount struct {
	Running int `json:"running"`
	Today   int `json:"today"`
}

// SubagentCounts are the subagent counts per agent and per account (by the account each start was recorded on) at now. A start runs
// until a SubagentStop of the same session, or until that session ends: its SessionEnd, a SessionStart of another session of the
// agent, or the agent's row closed. The last day is read, so a start older than that counts as ended.
func SubagentCounts(q Q, now time.Time) (agents, accounts map[string]SubagentCount, err error) {
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix()
	rows, err := q.Query("SELECT agent, COALESCE(account, ''), kind, ts, COALESCE(json_extract(payload, '$.session_id'), '') FROM events "+
		"WHERE kind IN (?, 'SubagentStop', 'SessionStart', 'SessionEnd') AND ts >= ? AND json_valid(payload) ORDER BY id",
		SubagentStart, min(midnight, now.Unix()-86400))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	agents, accounts = map[string]SubagentCount{}, map[string]SubagentCount{}
	open := map[string]map[string][]string{} // agent -> session -> the account of each start not yet stopped
	for rows.Next() {
		var agent, acct, kind, session string
		var ts int64
		if err := rows.Scan(&agent, &acct, &kind, &ts, &session); err != nil {
			return nil, nil, err
		}
		if open[agent] == nil {
			open[agent] = map[string][]string{}
		}
		switch kind {
		case SubagentStart:
			open[agent][session] = append(open[agent][session], acct)
			if ts >= midnight {
				a, c := agents[agent], accounts[acct]
				a.Today, c.Today = a.Today+1, c.Today+1
				agents[agent], accounts[acct] = a, c
			}
		case "SubagentStop":
			if s := open[agent][session]; len(s) > 0 {
				open[agent][session] = s[1:]
			}
		case "SessionEnd":
			delete(open[agent], session)
		case "SessionStart":
			for s := range open[agent] {
				if s != session {
					delete(open[agent], s)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	for agent, sessions := range open {
		var closed sql.NullInt64
		if err := q.QueryRow("SELECT closed_at FROM agent_state WHERE agent = ?", agent).Scan(&closed); err != nil && err != sql.ErrNoRows {
			return nil, nil, err
		}
		if closed.Int64 != 0 {
			continue
		}
		for _, accts := range sessions {
			for _, acct := range accts {
				a, c := agents[agent], accounts[acct]
				a.Running, c.Running = a.Running+1, c.Running+1
				agents[agent], accounts[acct] = a, c
			}
		}
	}
	delete(accounts, "")
	return agents, accounts, nil
}

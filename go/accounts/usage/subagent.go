package usage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/handline/handline/go/data/db"
)

// SubagentRefusal is why one more subagent may not start on acct, "" when it may: by p (Predict), one window would reach its block before
// its reset. A window p does not know refuses nothing.
func SubagentRefusal(acct string, st *AccountStatus, p Prediction, now float64) string {
	clock := func(at float64) string { return time.Unix(int64(at), 0).Format("15:04") }
	if p.ShortKnown && p.Short < 1 {
		hit := now + float64(max(0, ShortBlock-*st.ShortTrusted()))/(p.Burn+p.Extra)
		return fmt.Sprintf("account %s would hit its %s limit at %s, before its %s reset, with one more subagent: do it yourself, or hl task new",
			acct, WindowLabel(st.Short.Minutes), clock(hit), clock(float64(st.Short.Reset)))
	}
	if p.LongKnown && p.Long < 1 {
		left, _ := WeekEndsIn(st, now)
		return fmt.Sprintf("account %s would hit its weekly limit before it resets on %s, with one more subagent: do it yourself, or hl task new",
			acct, time.Unix(int64(now+left), 0).Format("Mon 15:04"))
	}
	return ""
}

// SubagentGuardKey is the config.yaml switch that lets the guard refuse a subagent; off by default, when every start is recorded and
// none refused.
const SubagentGuardKey = "subagent_pace_guard"

// NetworkAddressesKey chooses the hosts the network listener serves (connection.addresses); unset serves -listen as before.
const NetworkAddressesKey = "network_addresses"

// StartSubagent is the guard's question before agent's session starts a subagent at at: the refusal on the agent's account when the
// guard is on, or "" once the start is recorded (db.RecordSubagentStart).
func (e *Env) StartSubagent(q db.Q, agent, session string, at int64) (string, error) {
	var acct sql.NullString
	if err := q.QueryRow("SELECT account FROM agent_state WHERE agent = ?", agent).Scan(&acct); err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if acct.String != "" && e.config()[SubagentGuardKey] == true {
		if why := SubagentRefusal(acct.String, e.Read(acct.String), e.Predict(q, acct.String, agent, ""), e.now()); why != "" {
			return why, nil
		}
	}
	return "", db.RecordSubagentStart(q, agent, acct.String, session, at)
}

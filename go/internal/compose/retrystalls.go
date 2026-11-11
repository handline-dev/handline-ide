package compose

import (
	"database/sql"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

// RetryStartStalls runs once as the daemon opens: each agent stalled because its starts or wakes failed is cleared as agent.unstall
// clears it, so a fault the restart fixed is tried again. Three more failures stall it again, so this never loops.
func RetryStartStalls(d *sql.DB, logf func(string, ...any)) error {
	rows, err := db.AllAgentStates(d)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.State != states.Stalled {
			continue
		}
		kind := verdict.ParseStuckKind(row.StallKind.String)
		if kind != verdict.StuckStart && kind != verdict.StuckWake {
			continue
		}
		err := inTx(d, func(q db.Q) error {
			if err := db.ClearStall(q, row.Agent, "cleared as the daemon started, to try again"); err != nil {
				return err
			}
			_, err := db.AddEvent(q, row.Agent, "stall_retried", db.KV{K: "kind", V: string(kind)}, db.KV{K: "detail", V: row.Detail.String})
			return err
		})
		if err != nil {
			logf("%s: clearing its %s stall to try again: %v", row.Agent, kind, err)
			continue
		}
		logf("%s: %s stall cleared as the daemon started, to try again", row.Agent, kind)
	}
	return nil
}

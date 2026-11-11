package reads

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/cronspec"
)

// firedAt is when a schedule last fired: the newest schedule_fired event of the agent for its cron.
func (e *Env) firedAt(agent string, s cronspec.Schedule) int64 {
	rows, _ := query(e.DB, "SELECT ts, payload FROM events WHERE kind = 'schedule_fired' AND agent = ? ORDER BY id DESC LIMIT 200", agent)
	for _, r := range rows {
		var p map[string]any
		if json.Unmarshal([]byte(r.str("payload")), &p) == nil && p["cron"] == s.Cron {
			return r.i("ts")
		}
	}
	return 0
}

// schedulesList is schedules.list: each agent's schedules with the last time one fired and the next time it names, and the agents whose
// value is bad in errors, when there are any.
func (e *Env) schedulesList(c *rpc.Conn, params map[string]any) (any, error) {
	now := time.Now().Unix()
	out, bad := []any{}, []any{}
	for _, agent := range e.listAgents() {
		items, err := cronspec.Load(e.Home, agent)
		var b *cronspec.ScheduleError
		if errors.As(err, &b) {
			bad = append(bad, map[string]any{"target": agent, "error": b.Msg})
			continue
		}
		if err != nil {
			continue
		}
		for _, s := range items {
			last := e.firedAt(agent, s)
			cur, _ := queryOne(e.DB, "SELECT row_id FROM cursors WHERE name = ?", "schedule_at:"+agent+":"+cronspec.SID(s))
			handled := cur.i("row_id")
			from := max(last, handled)
			if last == 0 && handled == 0 {
				from = now
			}
			var next, lastRun any
			if n, ok := cronspec.NextCronAfter(s.Cron, from); ok {
				next = ts(n)
			}
			if last != 0 {
				lastRun = ts(last)
			}
			out = append(out, map[string]any{"id": cronspec.ID(agent, s), "target": agent, "cron": s.Cron, "message": s.Message,
				"next_run": next, "last_run": lastRun, "set_by": db.Human})
		}
	}
	res := map[string]any{"schedules": out}
	if len(bad) > 0 {
		res["errors"] = bad
	}
	return res, nil
}

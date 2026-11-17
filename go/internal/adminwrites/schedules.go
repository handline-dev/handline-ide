package adminwrites

import (
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/pyjson"
	"github.com/handline/handline/go/kit/yamlcompat"
	"github.com/handline/handline/go/scheduler/cronspec"
)

// WriteSchedules keeps the agent's schedules in its row through r; none removes them.
func WriteSchedules(r *team.Roster, agent string, items []cronspec.Schedule) error {
	var value any
	if len(items) > 0 {
		list := make([]any, len(items))
		for i, it := range items {
			list[i] = map[string]any{"cron": it.Cron, "message": it.Message}
		}
		value = pyjson.PyJSON(list, true, true)
	}
	return r.SetKeys(agent, []yamlcompat.Pair{{Key: "schedules", Val: value}})
}

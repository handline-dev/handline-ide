package compose

import (
	"database/sql"
	"math"
	"slices"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/taskboard/rulesfold"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// rulesFoldKey is the setting that turns the rules fold off ("off"); it runs by default.
const rulesFoldKey = "rules_fold"

// rulesFold is the daily fold of each person's comments into their rules, on the claude account with the most headroom; none when no
// account has any left, so it never takes what the agents need.
func rulesFold(cfg Config, q *sql.DB, uenv *usage.Env) *rulesfold.Daily {
	home := cfg.Home
	return &rulesfold.Daily{DB: q, Log: cfg.Log, Now: nowOf(cfg.Now),
		On: func() bool { return usage.LoadConfig(home)[rulesFoldKey] != "off" },
		Model: func() (rulesfold.Model, bool) {
			best, most := "", math.Inf(-1)
			for _, a := range paths.ListAccounts(home) {
				if team.AccountVendor(home, a) != team.DefaultVendor {
					continue
				}
				if h, ok := uenv.Headroom(a); ok && h > 0 && h > most {
					best, most = a, h
				}
			}
			if best == "" {
				return nil, false
			}
			dir, err := team.AccountConfigDir(home, best)
			if err != nil {
				return nil, false
			}
			return rulesfold.Claude(cfg.CLI, dir, cfg.Env), true
		},
		Kind: func(task int64) string { return taskArea(q, home, task) },
	}
}

// taskArea is the area of task's kind of work: what its newest Ready recorded, else the files its assignee's commits changed; "" when
// neither says.
func taskArea(q *sql.DB, home string, task int64) string {
	works, err := db.ReadyWorks(q, task)
	if err != nil {
		works = nil // the commits below say it as well
	}
	if len(works) == 0 {
		var assignee string // a task's owner column is who it is assigned to
		if q.QueryRow("SELECT COALESCE(owner, '') FROM tasks WHERE id = ?", task).Scan(&assignee) == nil && assignee != "" {
			works = taskwrites.TaskWorks(home, assignee, task)
		}
	}
	switch {
	case len(works) == 0:
		return ""
	case slices.Contains(works, db.WorkScreen):
		return db.AreaScreen
	}
	return db.AreaWork
}

package compose

import (
	"database/sql"
	"math"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/taskboard/rulesfold"
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
	}
}

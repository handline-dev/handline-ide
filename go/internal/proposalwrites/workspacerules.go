package proposalwrites

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/leases"
)

// workspaceRuleSet is workspace.rule.set: a kept rule on a workspace, or with none the machine default; a null value clears it.
func (e *Env) workspaceRuleSet(c *rpc.Conn, params map[string]any) (any, error) {
	ws, where := int64(team.MachineRules), "the machine default"
	if _, named := params["workspace"]; named {
		w, err := e.workspaceOf(params)
		if err != nil {
			return nil, err
		}
		ws, where = w.id, "workspace "+w.name
	}
	key, _ := params["key"].(string)
	raw, has := params["value"]
	if !has {
		return nil, rpc.BadRequestError("value is the rule's value, or null to clear it", "value", rpc.InvalidParams)
	}
	var value any
	if raw != nil {
		var err error
		if value, err = team.ParseRule(key, raw); err != nil {
			return nil, rpc.BadRequestError(err.Error(), "value", rpc.InvalidParams)
		}
		if err := e.ruleNamesKnown(key, value); err != nil {
			return nil, err
		}
	} else if !team.IsRuleKey(key) {
		_, err := team.ParseRule(key, nil)
		return nil, rpc.BadRequestError(err.Error(), "key", rpc.InvalidParams)
	}
	by := c.Caller.Actor()
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		if value == nil {
			if err := team.ClearRule(tx, ws, key); err != nil {
				return nil, err
			}
			return map[string]any{"accepted": true, "said": fmt.Sprintf("%s: %s cleared", where, key)}, nil
		}
		if err := team.SetRule(tx, ws, key, value, by, time.Now().Unix()); err != nil {
			return nil, err
		}
		return map[string]any{"accepted": true, "said": fmt.Sprintf("%s: %s set", where, key)}, nil
	})
}

// ruleNamesKnown refuses a lease kind there is none of and an account no login has.
func (e *Env) ruleNamesKnown(key string, value any) error {
	if kind, ok := strings.CutPrefix(key, team.RuleLeaseMax); ok && !slices.Contains(leases.Kinds, kind) {
		return rpc.BadRequestError(fmt.Sprintf("no lease kind %s (%s)", kind, strings.Join(leases.Kinds, ", ")), "key", rpc.InvalidParams)
	}
	if key == team.RuleAccounts {
		have := paths.ListAccounts(e.Home)
		for _, a := range value.([]string) {
			if !slices.Contains(have, a) {
				return rpc.BadRequestError(fmt.Sprintf("no account %s (%s)", a, strings.Join(have, ", ")), "value", rpc.InvalidParams)
			}
		}
	}
	return nil
}

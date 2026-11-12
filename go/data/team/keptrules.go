package team

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/handline/handline/go/data/db"
)

// The kept rules (docs/design/people-workspaces.md "Workspace rules"): rows of workspace_settings, set by the owner. A workspace's own
// value beats the machine default (workspace 0), which beats the built-in: no rule.
const (
	RuleAccounts       = "accounts"
	RuleAgentCap       = "agent_cap"
	RuleLeaseMax       = "lease_max." // + a lease kind
	RuleToolsDeny      = "tools_deny"
	RuleModeMax        = "mode_max"
	RuleReviewRequired = "review_required"
)

// MachineRules is the workspace number of the machine default.
const MachineRules = 0

// ModeOrder is Modes from weakest to strongest, the order mode_max caps by.
var ModeOrder = []string{"plan", "default", "acceptEdits", "auto"}

// ruleOrder is the order rules are listed and rendered in; lease_max.<kind> sorts by kind in its place.
var ruleOrder = []string{RuleAccounts, RuleAgentCap, RuleLeaseMax, RuleToolsDeny, RuleModeMax, RuleReviewRequired}

// atNextStart are the rules a running session keeps until its next start.
var atNextStart = []string{RuleToolsDeny, RuleModeMax}

// KeptRule is one rule in force for a workspace: its value (int64, []string, string or bool) and where it comes from.
type KeptRule struct {
	Key    string
	Value  any
	Source string // "workspace" or "machine"
	SetBy  string
	SetAt  int64
}

func ruleRank(key string) int {
	if strings.HasPrefix(key, RuleLeaseMax) {
		key = RuleLeaseMax
	}
	return slices.Index(ruleOrder, key)
}

// IsRuleKey is whether key names a kept rule; a lease_max key needs a kind after the dot.
func IsRuleKey(key string) bool {
	if strings.HasPrefix(key, RuleLeaseMax) {
		return len(key) > len(RuleLeaseMax)
	}
	return slices.Contains(ruleOrder, key)
}

// NotARule is why key names no kept rule.
func NotARule(key string) error {
	return fmt.Errorf("%s is not a kept rule (%s, lease_max.<kind>)", key, strings.Join(slices.DeleteFunc(slices.Clone(ruleOrder), func(k string) bool { return k == RuleLeaseMax }), ", "))
}

// ParseRule is v (as JSON decodes it) as key's value, or why it cannot be.
func ParseRule(key string, v any) (any, error) {
	switch {
	case !IsRuleKey(key):
		return nil, NotARule(key)
	case key == RuleAgentCap || strings.HasPrefix(key, RuleLeaseMax):
		n, ok := wholeNumber(v)
		if !ok || n < 0 {
			return nil, fmt.Errorf("%s is a whole number, 0 for no rule", key)
		}
		return n, nil
	case key == RuleAccounts || key == RuleToolsDeny:
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("%s is a list of names, empty for no rule", key)
		}
		out := []string{}
		for _, x := range list {
			s, ok := x.(string)
			if s = strings.TrimSpace(s); !ok || s == "" {
				return nil, fmt.Errorf("%s is a list of names, empty for no rule", key)
			}
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
		return out, nil
	case key == RuleModeMax:
		s, ok := v.(string)
		if !ok || s != "" && !slices.Contains(ModeOrder, s) {
			return nil, fmt.Errorf("%s is one of %s, or empty for no rule", key, strings.Join(ModeOrder, ", "))
		}
		return s, nil
	default: // review_required
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("%s is true or false", key)
		}
		return b, nil
	}
}

// wholeNumber is v as a whole number, however JSON was decoded.
func wholeNumber(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), n == float64(int64(n))
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case int64:
		return n, true
	case int:
		return int64(n), true
	}
	return 0, false
}

// RuleOn is whether value holds anything: 0, an empty list or name and false are no rule.
func RuleOn(value any) bool {
	switch v := value.(type) {
	case int64:
		return v != 0
	case []string:
		return len(v) > 0
	case string:
		return v != ""
	case bool:
		return v
	}
	return false
}

// SetRule keeps value (from ParseRule) as key's on workspace ws (MachineRules for the machine default), set by by at now.
func SetRule(q db.Q, ws int64, key string, value any, by string, now int64) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO workspace_settings(workspace, key, value, set_by, set_at) VALUES(?, ?, ?, ?, ?) "+
		"ON CONFLICT(workspace, key) DO UPDATE SET value = excluded.value, set_by = excluded.set_by, set_at = excluded.set_at",
		ws, key, string(raw), by, now)
	return err
}

// ClearRule takes key off ws, which then follows the level below.
func ClearRule(q db.Q, ws int64, key string) error {
	_, err := q.Exec("DELETE FROM workspace_settings WHERE workspace = ? AND key = ?", ws, key)
	return err
}

// ClearRules takes every rule off ws: the workspace is gone.
func ClearRules(q db.Q, ws int64) error {
	_, err := q.Exec("DELETE FROM workspace_settings WHERE workspace = ?", ws)
	return err
}

// KeptRules is every rule set for ws or on the machine, ws's beating the machine's, in rule order. A value set to no rule is in it too:
// a workspace's 0 still beats the machine's 3.
func KeptRules(q db.Q, ws int64) ([]KeptRule, error) {
	rows, err := q.Query("SELECT workspace, key, value, set_by, set_at FROM workspace_settings WHERE workspace IN (?, ?) "+
		"ORDER BY workspace = ?", MachineRules, ws, ws)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byKey := map[string]KeptRule{}
	for rows.Next() {
		var at int64
		var r KeptRule
		var raw string
		if err := rows.Scan(&at, &r.Key, &raw, &r.SetBy, &r.SetAt); err != nil {
			return nil, err
		}
		var v any
		if json.Unmarshal([]byte(raw), &v) != nil {
			continue
		}
		if r.Value, err = ParseRule(r.Key, v); err != nil {
			continue // a key or value this build does not know
		}
		r.Source = "machine"
		if at != MachineRules {
			r.Source = "workspace"
		}
		byKey[r.Key] = r // the workspace's row comes last
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]KeptRule, 0, len(byKey))
	for _, r := range byKey {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b KeptRule) int {
		if d := ruleRank(a.Key) - ruleRank(b.Key); d != 0 {
			return d
		}
		return strings.Compare(a.Key, b.Key)
	})
	return out, nil
}

// WorkspaceID is the id of the workspace named name; ok is false for none.
func WorkspaceID(q db.Q, name string) (int64, bool, error) {
	var id int64
	err := q.QueryRow("SELECT id FROM workspaces WHERE name = ?", name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// RulesBlock is the rules in force as the lines agents read under their team's text, "" with none on.
func RulesBlock(rules []KeptRule) string {
	var parts []string
	for _, r := range rules {
		if !RuleOn(r.Value) {
			continue
		}
		var v string
		switch x := r.Value.(type) {
		case []string:
			v = strings.Join(x, ", ")
		case bool:
			v = "on"
		default:
			v = fmt.Sprint(x)
		}
		line := r.Key + " " + v
		if slices.Contains(atNextStart, r.Key) {
			line += " (from the next session)"
		}
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Kept rules (set by the owner): " + strings.Join(parts, "; ") + "."
}

// KeptBlocks gives Rules each workspace's RulesBlock. A read that fails gives the last block read, so a failing read is no change to
// the rules agents are told.
type KeptBlocks struct {
	Q    db.Q
	mu   sync.Mutex
	last map[string]string
}

// Block is the RulesBlock of the workspace named name: "" for a team with no workspace row.
func (k *KeptBlocks) Block(name string) string {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.last == nil {
		k.last = map[string]string{}
	}
	id, ok, err := WorkspaceID(k.Q, name)
	var rules []KeptRule
	if err == nil && ok {
		rules, err = KeptRules(k.Q, id)
	}
	if err != nil {
		return k.last[name]
	}
	k.last[name] = RulesBlock(rules)
	return k.last[name]
}

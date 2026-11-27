package prompts

import (
	"fmt"
	"strings"

	"github.com/handline/handline/go/scheduler/access"
)

// SessionRule is a rule Claude Code applies to the running session when an allow carries it (updatedPermissions, destination session).
type SessionRule struct{ Tool, Content string }

// String is the rule as Claude Code writes it: Tool(content).
func (r SessionRule) String() string { return r.Tool + "(" + r.Content + ")" }

// ParseRule is the rule Tool(content) splits into; ok is false for one with no content.
func ParseRule(rule string) (r SessionRule, ok bool) {
	tool, content, found := strings.Cut(rule, "(")
	if !found || !strings.HasSuffix(content, ")") {
		return r, false
	}
	r = SessionRule{tool, strings.TrimSuffix(content, ")")}
	return r, r.Tool != "" && r.Content != ""
}

// exactRule is the rule for this exact Bash command, when Always allow may keep it (access.SafeRule); ok is false for none.
func (e *Env) exactRule(tool string, input map[string]any) (r SessionRule, ok bool) {
	command, _ := input["command"].(string)
	r = SessionRule{tool, command}
	return r, tool == "Bash" && access.SafeRule(e.Home, r.String())
}

// askedBefore is whether agent was asked rule before, on an earlier card.
func (e *Env) askedBefore(agent, rule string) (bool, error) {
	var n int
	err := e.DB.QueryRow("SELECT COUNT(*) FROM prompt_blocks WHERE agent = ? AND json_extract(detail, '$.source') IN (?, ?) AND "+
		"json_extract(detail, '$.rule') = ?", agent, HookSource, StreamSource, rule).Scan(&n)
	return n > 0, err
}

// keepRule keeps rule as the agent's always-allowed rule (its own, for a temp the agent it was hired from), which each fresh session of
// it and of its temps starts with, and says so on the agents topic.
func (e *Env) keepRule(agent, rule string) error {
	if !access.SafeRule(e.Home, rule) {
		return fmt.Errorf("%s is not a rule Always allow keeps", rule)
	}
	tx, err := e.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	owner, err := access.RuleOwner(tx, agent)
	if err != nil {
		return err
	}
	if err := access.AddRule(e.Home, owner, rule, e.now()); err != nil {
		return err
	}
	now, err := access.Changed(tx, e.Home, owner)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if e.Emit != nil {
		_ = e.Emit("agents", "access.changed", now)
	}
	return nil
}

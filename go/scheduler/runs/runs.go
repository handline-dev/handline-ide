// Package runs keeps the shared runs: a named set of commands that agents take in turns through hl use, at most AtOnce at a time,
// across this Mac or within one workspace. Any agent sets one for this Mac or its own workspace, applied at once, but changes or
// removes none the founder set; the founder sets any.
// Each is a row of workspace_settings under the key run.<name>: workspace 0 is This Mac. No run, nothing waits.
package runs

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
)

// Mac is the scope of a run every workspace on this Mac takes turns at, and its workspace number in workspace_settings.
const (
	Mac   = "mac"
	MacWS = 0
)

// keyPrefix is a run's key in workspace_settings, before its name.
const keyPrefix = "run."

// MaxCmds is the most commands one run covers.
const MaxCmds = 20

// Presets are the ready-made patterns a run may cover: the commands Handline already knows as a test or build run, a simulator or a
// browser (session.Covers).
var Presets = []string{"@build-test", "@simulator", "@browser"}

// reserved are the words hl use takes for itself or for a lease kind.
var reserved = []string{"add", "remove", "list", "heavy", "simulator", "browser", "port"}

// tooBroad are the single words a run may not cover: nearly every command starts with one.
var tooBroad = []string{"bash", "sh", "zsh", "git", "hl", "cat", "ls", "cd", "echo", "env", "sudo", "exec", "time", "timeout", "nice",
	"command", "xargs", "python", "python3", "node", "go", "npm", "pnpm", "npx", "yarn", "make", "open", "xcrun", "grep", "rg", "find",
	"sed", "awk", "rm", "cp", "mv", "mkdir", "touch", "head", "tail", "wc", "sort", "jq", "curl", "docker", "swift", "cargo", "gh", "test"}

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Run is one shared run.
type Run struct {
	Name      string
	Workspace int64 // MacWS for This Mac
	Cmds      []string
	AtOnce    int
	Why       string
	SetBy     string
	SetAt     int64
}

// Scope is "mac" or the workspace's key (w3).
func (r Run) Scope() string {
	if r.Workspace == MacWS {
		return Mac
	}
	return actor.Key(actor.Workspace, r.Workspace)
}

// Key is the run's lease kind: its scope and name, mac:xcode or w3:xcode.
func (r Run) Key() string { return r.Scope() + ":" + r.Name }

// Check is why r cannot be kept, "" when it can. It trims r's commands in place.
func Check(r *Run) (why, field string) {
	switch {
	case !nameRE.MatchString(r.Name):
		return "name is a lowercase word of letters, digits and dashes, at most 32", "name"
	case slices.Contains(reserved, r.Name):
		return fmt.Sprintf("%s is a word hl use keeps for itself (%s)", r.Name, strings.Join(reserved, ", ")), "name"
	case r.AtOnce < 1:
		return "at_once is a whole number from 1", "at_once"
	case len(r.Cmds) == 0 || len(r.Cmds) > MaxCmds:
		return fmt.Sprintf("cmds is 1 to %d commands", MaxCmds), "cmds"
	}
	var cmds []string
	for _, c := range r.Cmds {
		c = strings.Join(strings.Fields(c), " ")
		switch {
		case c == "":
			return "a command is a word, a command and its first words, a script path, or one of " + strings.Join(Presets, ", "), "cmds"
		case strings.HasPrefix(c, "@") && !slices.Contains(Presets, c):
			return fmt.Sprintf("%s is no preset (%s)", c, strings.Join(Presets, ", ")), "cmds"
		case slices.Contains(tooBroad, c):
			return fmt.Sprintf("%s alone would cover nearly every command; name what it runs too (%s test) or a script", c, c), "cmds"
		}
		if !slices.Contains(cmds, c) {
			cmds = append(cmds, c)
		}
	}
	r.Cmds = cmds
	return "", ""
}

// value is what a run keeps in its row's value.
type value struct {
	Cmds   []string `json:"cmds"`
	AtOnce int      `json:"at_once"`
	Why    string   `json:"why"`
}

// All is every run, in the order asks take them: by name, then This Mac's before a workspace's.
func All(q db.Q) ([]Run, error) {
	rows, err := q.Query("SELECT workspace, key, value, set_by, set_at FROM workspace_settings WHERE key LIKE 'run.%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		var key, raw string
		if err := rows.Scan(&r.Workspace, &key, &raw, &r.SetBy, &r.SetAt); err != nil {
			return nil, err
		}
		var v value
		if json.Unmarshal([]byte(raw), &v) != nil {
			continue
		}
		r.Name, r.Cmds, r.AtOnce, r.Why = strings.TrimPrefix(key, keyPrefix), v.Cmds, v.AtOnce, v.Why
		if why, _ := Check(&r); why != "" {
			continue // a row this build cannot read
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	Sort(out)
	return out, nil
}

// Sort puts runs in the order asks take them: by name, then by key.
func Sort(rs []Run) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].Name != rs[j].Name {
			return rs[i].Name < rs[j].Name
		}
		return rs[i].Key() < rs[j].Key()
	})
}

// For is the runs an agent of workspace ws (0 for none) takes: This Mac's and its workspace's.
func For(all []Run, ws int64) []Run {
	var out []Run
	for _, r := range all {
		if r.Workspace == MacWS || (ws != MacWS && r.Workspace == ws) {
			out = append(out, r)
		}
	}
	return out
}

// Clash is why r's name cannot be used in its scope, "" when it can: a This Mac run's name is used by no workspace's, and a
// workspace's by no This Mac run, so hl use <name> names one run for every agent.
func Clash(all []Run, r Run) string {
	for _, o := range all {
		switch {
		case o.Name != r.Name || o.Workspace == r.Workspace:
		case r.Workspace == MacWS:
			return fmt.Sprintf("workspace %s has a run named %s; pick another name", o.Scope(), r.Name)
		case o.Workspace == MacWS:
			return fmt.Sprintf("This Mac has a run named %s; pick another name, or change that one", r.Name)
		}
	}
	return ""
}

// Set keeps r, in place of the run of its name in its scope.
func Set(q db.Q, r Run) error {
	raw, err := json.Marshal(value{Cmds: r.Cmds, AtOnce: r.AtOnce, Why: r.Why})
	if err != nil {
		return err
	}
	_, err = q.Exec("INSERT INTO workspace_settings(workspace, key, value, set_by, set_at) VALUES(?, ?, ?, ?, ?) "+
		"ON CONFLICT(workspace, key) DO UPDATE SET value = excluded.value, set_by = excluded.set_by, set_at = excluded.set_at",
		r.Workspace, keyPrefix+r.Name, string(raw), r.SetBy, r.SetAt)
	return err
}

// Remove drops the run name of workspace ws; false when there was none.
func Remove(q db.Q, ws int64, name string) (bool, error) {
	res, err := q.Exec("DELETE FROM workspace_settings WHERE workspace = ? AND key = ?", ws, keyPrefix+name)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// WorkspaceOf is the workspace number of agent (its key), 0 when it is no agent or has none.
func WorkspaceOf(q db.Q, agent string) int64 {
	k, id, ok := actor.Parse(agent)
	if !ok || k != actor.Agent {
		return 0
	}
	var ws sql.NullInt64
	if err := q.QueryRow("SELECT workspace FROM agents WHERE id = ?", id).Scan(&ws); err != nil {
		return 0
	}
	return ws.Int64
}

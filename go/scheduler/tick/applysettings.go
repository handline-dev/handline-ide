package tick

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/pyjson"
)

// A session whose launch settings (model, effort, mcp config, browser, permissions, ROLE.md, base.md, the team line) have changed is
// restarted once at an idle moment, as session.restart does (resumed, unless agent.set asked for fresh), never mid-turn or over a dialog.
// A change of the team rules alone restarts no one: the session is told the changed lines once, as a notice for its next turn. Each tick
// compares the parts the launch recorded with what the next launch would read, so a change made mid-turn is applied after the turn.
const (
	// SettingsWho is who the restart says asked for it when no person's change is kept (db.SettingsByMeta).
	SettingsWho = "the daemon"
	// SettingsApplied is the meta key prefix that keeps what a restart was for.
	SettingsApplied = "settings_applied:"
)

// Part is one named setting a launch reads, in launch order.
type Part struct {
	Name  string
	Value any
}

// SettingsPorts are the foreign calls applying settings makes.
type SettingsPorts interface {
	// FingerprintParts is launch.fingerprint_parts: the settings the next launch would use, in launch order.
	FingerprintParts(agent string) ([]Part, error)
	// CurrentAccount is agents.current_account.
	CurrentAccount(q db.Q, agent string) string
	// RulesNote is the note of a session's changed team rules and the record to keep once it is told; KeepRulesTold keeps it.
	RulesNote(agent string, row *db.AgentState) (note, record string, err error)
	KeepRulesTold(q db.Q, agent, was, record string) error
	// Tell queues a notice to the agent for its next turn.
	Tell(q db.Q, agent, text string) error
}

// RulesPart is the one setting a running session is told of rather than restarted for: its team rules (lifecycle's part name).
const RulesPart = "team rules"

// ApplySettings restarts sessions launched with settings that have changed.
type ApplySettings struct{ Ports SettingsPorts }

// Changed is the named settings the running session was launched without, in launch order; nothing for a session that recorded none (a
// launch from before they were kept).
func (a *ApplySettings) Changed(agent string, row *db.AgentState) ([]string, error) {
	var old map[string]json.RawMessage
	if err := json.Unmarshal([]byte(orNull(row.LaunchParts.String)), &old); err != nil || old == nil {
		return nil, nil
	}
	parts, err := a.Ports.FingerprintParts(agent)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, p := range parts {
		was, ok := old[p.Name]
		if !ok {
			continue
		}
		now, err := json.Marshal(p.Value)
		if err != nil {
			return nil, err
		}
		if !sameJSON(was, now) {
			names = append(names, p.Name)
		}
	}
	return names, nil
}

func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

// Sweep applies changed settings to every agent; one agent's failure is not the tick's.
func (a *ApplySettings) Sweep(t *Tick, now int64) error {
	for _, agent := range t.Host.Agents() {
		if err := guard(func() { a.apply(t, agent, now) }); err != nil {
			t.logf("  %s: applying changed settings failed: %v", agent, err)
		}
	}
	return nil
}

func (a *ApplySettings) apply(t *Tick, agent string, now int64) {
	row := must(db.GetAgentState(t.Q, agent))
	if row == nil || closedAt(row) || !t.Host.SessionAlive(agent) {
		return
	}
	busy := row.State == states.Running
	if !busy && row.State != states.Idle && row.State != states.Queued {
		return
	}
	names := must(a.Changed(agent, row))
	if len(names) == 1 && names[0] == RulesPart {
		note, record := must2(a.Ports.RulesNote(agent, row))
		if note != "" { // told before it is kept: a failed tell is tried again next tick, a failed keep only tells twice
			check(a.Ports.Tell(t.Q, agent, note))
			t.logf("  %s: told of its changed team rules, not restarted", agent)
		}
		if record != "" {
			check(a.Ports.KeepRulesTold(t.Q, agent, row.LaunchParts.String, record))
		}
		names = nil
	}
	if len(names) == 0 {
		check(db.SetSettingsOwed(t.Q, agent, nil, 0))
		return
	}
	// What this restart is for, kept: a launch that still disagrees with the settings after it (it has happened with things a launch
	// cannot reproduce) is not restarted again for the same change.
	parts := must(a.Ports.FingerprintParts(agent))
	obj := map[string]any{}
	for _, p := range parts {
		obj[p.Name] = p.Value
	}
	text := pyjson.PyJSON(obj, true, true)
	if applied, _ := must2(db.Meta(t.Q, SettingsApplied+agent)); applied == text {
		check(db.SetSettingsOwed(t.Q, agent, nil, 0))
		return
	}
	if busy || !t.Delivery.SessionPresent(agent) || !t.Host.SessionIdle(agent) {
		check(db.SetSettingsOwed(t.Q, agent, names, now)) // the agent view says the restart waits for the turn to end
		return                                            // mid-turn or waiting on a permission: the next tick asks again
	}
	st := t.Usage.ReadForAgent(agent, a.Ports.CurrentAccount(t.Q, agent))
	if st.SessionID == "" {
		return
	}
	by, _ := must2(db.Meta(t.Q, db.SettingsByMeta(agent)))
	if by == "" {
		by = SettingsWho
	}
	mode, _ := must2(db.Meta(t.Q, db.SettingsRestartMeta(agent)))
	fresh := mode == "fresh"
	ok, said := t.Lifecycle.RestartAgent(t.Q, agent, st, by, names, "settings_applied", fresh)
	if ok {
		check(db.SetMeta(t.Q, SettingsApplied+agent, text))
		check(db.SetSettingsOwed(t.Q, agent, nil, 0))
		check(db.SetMeta(t.Q, db.SettingsByMeta(agent), ""))
		check(db.SetMeta(t.Q, db.SettingsRestartMeta(agent), ""))
		how := ""
		if fresh {
			how = ", as a new conversation"
		}
		t.logf("  %s: restarted to apply changed settings (%s)%s", agent, strings.Join(names, ", "), how)
	} else {
		t.logf("  %s: changed settings (%s) not applied yet - %s", agent, strings.Join(names, ", "), said)
	}
}

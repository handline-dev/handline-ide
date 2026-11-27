package tick

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/jsontext"
)

// A running session is never restarted for changed prompt parts (ROLE.md, base.md, team instructions): claude --resume keeps the system
// prompt its conversation began with, so a resume would cost the turn and the cache and apply nothing. They are owed until its next
// fresh start (a cold-cache wake, a handoff when context fills). Changed team rules are told once, in a message.
// A launch flag a person changed (agent.set, an approved proposal, a kept grant; who is kept in db.SettingsByMeta) is applied at an idle
// moment as a handoff note and a fresh start; one nobody changed is owed like the prompt parts. SettingsApplied is the meta key
// prefix that keeps what a restart was for.
const SettingsApplied = "settings_applied:"

// Part is one named setting a launch reads, in launch order.
type Part struct {
	Name  string
	Value any
}

// SettingsPorts are the foreign calls applying settings makes.
type SettingsPorts interface {
	// FingerprintParts is the settings the next launch would use, in launch order, as the running session is held to them
	// (lifecycle's PartsSince: a part whose text it launched with is unchanged keeps its recorded hash).
	FingerprintParts(agent string) ([]Part, error)
	// CurrentAccount is the account recorded for the agent.
	CurrentAccount(q db.Q, agent string) string
	// RulesNote is the note of a session's changed team rules and the record to keep once it is told.
	RulesNote(agent string, row *db.AgentState) (note, record string, err error)
	// TellRules sends the note as a message and puts record in place of was, both or neither, so a note is sent once.
	TellRules(agent, note, was, record string) error
}

// RulesPart is the one setting a running session is told of rather than restarted for: its team rules (lifecycle's part name).
const RulesPart = "team rules"

// PromptParts are lifecycle.PromptParts: what only a fresh start gives a session.
var PromptParts = []string{"ROLE.md", "base.md", "team instructions"}

// ApplySettings tells, owes or applies what changed since a session was launched.
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
	if i := slices.Index(names, RulesPart); i >= 0 {
		note, record := must2(a.Ports.RulesNote(agent, row))
		if note != "" || record != "" {
			check(a.Ports.TellRules(agent, note, row.LaunchParts.String, record))
		}
		if note != "" {
			t.logf("  %s: told of its changed team rules, not restarted", agent)
		}
		names = slices.Delete(names, i, i+1)
	}
	isFlag := func(n string) bool { return !slices.Contains(PromptParts, n) && n != RulesPart }
	by, _ := must2(db.Meta(t.Q, db.SettingsByMeta(agent)))
	if !slices.ContainsFunc(names, isFlag) {
		if by != "" { // a writer keeps who with what it writes, in one write: a change of no flag is theirs no longer
			check(db.SetMeta(t.Q, db.SettingsByMeta(agent), ""))
		}
		check(db.SetSettingsOwed(t.Q, agent, names, now, false)) // until the session's next fresh start, which reads them
		return
	}
	if by == "" { // nobody's change: a Handline update, or a workspace hold, which applies from the next session
		check(db.SetSettingsOwed(t.Q, agent, names, now, false))
		return
	}
	// What this restart is for, kept: a launch that still disagrees with the flags after it (it has happened with things a launch cannot
	// reproduce) is not restarted again for the same change. The flags alone: a prompt part edited since is no new reason to restart.
	parts := must(a.Ports.FingerprintParts(agent))
	obj := map[string]any{}
	for _, p := range parts {
		if isFlag(p.Name) {
			obj[p.Name] = p.Value
		}
	}
	text := jsontext.Spaced(obj, true, true)
	if applied, _ := must2(db.Meta(t.Q, SettingsApplied+agent)); applied == text {
		check(db.SetSettingsOwed(t.Q, agent, names, now, false)) // a launch cannot give it: the next fresh start tries again
		return
	}
	check(db.SetSettingsOwed(t.Q, agent, names, now, true)) // the agent view says the restart waits; the restart clears it
	if busy || !t.Delivery.SessionPresent(agent) || !t.Host.SessionIdle(agent) {
		return // mid-turn or waiting on a permission: the next tick asks again
	}
	st := t.Usage.ReadForAgent(agent, a.Ports.CurrentAccount(t.Q, agent))
	if st.SessionID == "" {
		return
	}
	// A new conversation starts from the handoff: the agent is asked for its note first, as before any other fresh start, and the
	// restart waits for it (for at most handoff.AskWaitS). The next tick tries again.
	if t.Handoff.NoteHold(t.Q, agent, now) {
		t.logf("  %s: changed settings (%s) wait for its handoff note before a new conversation", agent, strings.Join(names, ", "))
		return
	}
	ok, said := t.Lifecycle.RestartAgent(t.Q, agent, st, by, names, "settings_applied", true)
	if ok {
		check(db.SetMeta(t.Q, SettingsApplied+agent, text))
		check(db.SetSettingsOwed(t.Q, agent, nil, 0, false))
		check(db.SetMeta(t.Q, db.SettingsByMeta(agent), ""))
		t.logf("  %s: restarted to apply changed settings (%s), as a new conversation", agent, strings.Join(names, ", "))
	} else {
		t.logf("  %s: changed settings (%s) not applied yet - %s", agent, strings.Join(names, ", "), said)
	}
}

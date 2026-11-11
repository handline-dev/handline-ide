package tick

import (
	"errors"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
)

type settingsPorts struct {
	parts   []Part
	account string
	note    string
	told    *[]string
	kept    *[]string
	tellErr error
}

func (s settingsPorts) FingerprintParts(string) ([]Part, error) { return s.parts, nil }
func (s settingsPorts) CurrentAccount(db.Q, string) string      { return s.account }
func (s settingsPorts) RulesNote(string, *db.AgentState) (string, string, error) {
	return s.note, "record", nil
}
func (s settingsPorts) TellRules(agent, note, _, record string) error {
	if s.tellErr != nil {
		return s.tellErr
	}
	if note != "" {
		*s.told = append(*s.told, agent+": "+note)
	}
	*s.kept = append(*s.kept, agent+": "+record)
	return nil
}

// settled is a world whose agent "a" is open and idle, launched with the model "sonnet" and now configured for "opus".
func settled(t *testing.T) (*Tick, *world, *ApplySettings) {
	tk, w := newWorld(t)
	setState(t, tk, "a", states.Idle, 0)
	if _, err := tk.Q.Exec(`UPDATE agent_state SET launch_parts = '{"model": "sonnet", "effort": 3}' WHERE agent = 'a'`); err != nil {
		t.Fatal(err)
	}
	w.idle, w.restartOK = true, true
	w.status = Status{SessionID: "s1"}
	return tk, w, &ApplySettings{Ports: settingsPorts{parts: []Part{{"model", "opus"}, {"effort", float64(3)}}, account: "acct"}}
}

func TestAChangedSettingRestartsTheIdleSessionOnceAndRemembersWhatItWasFor(t *testing.T) {
	tk, w, a := settled(t)
	a.Sweep(tk, 10_000)
	if !w.did("restart a by the daemon for model (settings_applied)") || !w.logged("restarted to apply changed settings (model)") {
		t.Fatalf("%v %v", w.calls, w.logs)
	}
	if v, _, _ := db.Meta(tk.Q, SettingsApplied+"a"); !strings.Contains(v, `"model"`) || !strings.Contains(v, "opus") {
		t.Fatalf("what the restart was for was not kept: %q", v)
	}
	// A launch that still disagrees afterwards is not restarted again for the same change.
	w.calls = nil
	a.Sweep(tk, 10_030)
	if w.did("restart") {
		t.Fatalf("restarted twice for one change: %v", w.calls)
	}
	// A different change is restarted for.
	a.Ports = settingsPorts{parts: []Part{{"model", "opus"}, {"effort", float64(2)}}, account: "acct"}
	a.Sweep(tk, 10_060)
	if !w.did("restart a by the daemon for model,effort (settings_applied)") && !w.did("restart a by the daemon for effort (settings_applied)") {
		t.Fatalf("%v", w.calls)
	}
}

// A change made mid-turn is kept as owed, for the agent view to say the restart waits for the turn; the restart clears it.
func TestAChangeMadeMidTurnIsOwedUntilItsRestart(t *testing.T) {
	tk, w, a := settled(t)
	setState(t, tk, "a", states.Running, 0)
	a.Sweep(tk, 10_000)
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o == nil || o.Since != 10_000 || strings.Join(o.Settings, ",") != "model" || w.did("restart") {
		t.Fatalf("%+v %v", o, w.calls)
	}
	setState(t, tk, "a", states.Idle, 0)
	a.Sweep(tk, 10_030)
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o != nil || !w.did("restart a") {
		t.Fatalf("%+v %v", o, w.calls)
	}
}

// A person's change is restarted in their name, once: the next change with no one kept is the daemon's.
func TestAPersonsChangeIsRestartedInTheirName(t *testing.T) {
	tk, w, a := settled(t)
	if err := db.SetMeta(tk.Q, db.SettingsByMeta("a"), db.Human); err != nil {
		t.Fatal(err)
	}
	a.Sweep(tk, 10_000)
	if !w.did("restart a by human for model (settings_applied)") {
		t.Fatalf("%v", w.calls)
	}
	if v, _, _ := db.Meta(tk.Q, db.SettingsByMeta("a")); v != "" {
		t.Fatalf("who asked outlived the restart: %q", v)
	}
}

// agent.set asking for a fresh restart makes the next one a new conversation, once: the mode goes with the restart it was for.
func TestARestartAskedFreshStartsANewConversationOnce(t *testing.T) {
	tk, w, a := settled(t)
	if err := db.SetMeta(tk.Q, db.SettingsRestartMeta("a"), "fresh"); err != nil {
		t.Fatal(err)
	}
	a.Sweep(tk, 10_000)
	if !w.did("restart fresh a by the daemon for model (settings_applied)") || !w.logged("restarted to apply changed settings (model), as a new conversation") {
		t.Fatalf("%v %v", w.calls, w.logs)
	}
	if v, _, _ := db.Meta(tk.Q, db.SettingsRestartMeta("a")); v != "" {
		t.Fatalf("the mode outlived its restart: %q", v)
	}
	w.calls = nil
	a.Ports = settingsPorts{parts: []Part{{"model", "opus"}, {"effort", float64(2)}}, account: "acct"}
	a.Sweep(tk, 10_060)
	if w.did("restart fresh") || !w.did("restart a") {
		t.Fatalf("the next change resumes: %v", w.calls)
	}
}

func TestAChangedSettingWaitsForAnIdleMomentAndAnOpenRunnableSession(t *testing.T) {
	cases := map[string]func(tk *Tick, w *world){
		"mid-turn or an open prompt": func(tk *Tick, w *world) { w.idle = false },
		"no session to restart":      func(tk *Tick, w *world) { w.alive["a"] = false },
		"session not present":        func(tk *Tick, w *world) { w.present = false },
		"running":                    func(tk *Tick, w *world) { setState(t, tk, "a", states.Running, 0) },
		"parked":                     func(tk *Tick, w *world) { setState(t, tk, "a", states.Parked, 20_000) },
		"closed":                     func(tk *Tick, w *world) { db.SetClosed(tk.Q, "a", "s1") },
		"no session id to resume":    func(tk *Tick, w *world) { w.status.SessionID = "" },
	}
	for name, mutate := range cases {
		tk, w, a := settled(t)
		mutate(tk, w)
		a.Sweep(tk, 10_000)
		if w.did("restart") {
			t.Errorf("%s: restarted", name)
		}
	}
	// A launch that recorded no parts (from before they were kept) is not restarted.
	tk, w, a := settled(t)
	tk.Q.Exec(`UPDATE agent_state SET launch_parts = NULL WHERE agent = 'a'`)
	a.Sweep(tk, 10_000)
	if w.did("restart") {
		t.Error("a launch with no recorded parts was restarted")
	}
}

func TestARestartThatDidNotHappenIsLoggedAndTriedAgainNextTick(t *testing.T) {
	tk, w, a := settled(t)
	w.restartOK, w.restartSaid = false, "mid-turn"
	a.Sweep(tk, 10_000)
	if !w.logged("changed settings (model) not applied yet - mid-turn") {
		t.Fatalf("%v", w.logs)
	}
	if v, _, _ := db.Meta(tk.Q, SettingsApplied+"a"); v != "" {
		t.Fatalf("a restart that failed was remembered as applied: %q", v)
	}
	w.restartOK = true
	a.Sweep(tk, 10_030)
	if v, _, _ := db.Meta(tk.Q, SettingsApplied+"a"); v == "" {
		t.Fatal("the next tick did not apply it")
	}
}

func TestOneAgentsFailureApplyingSettingsDoesNotStopTheSweep(t *testing.T) {
	tk, w, a := settled(t)
	w.agents = []string{"b", "a"} // b has no row at all
	a.Ports = failingFor{settingsPorts: a.Ports.(settingsPorts), bad: "b"}
	a.Sweep(tk, 10_000)
	if !w.did("restart a") {
		t.Fatalf("%v %v", w.calls, w.logs)
	}
}

type failingFor struct {
	settingsPorts
	bad string
}

func (f failingFor) FingerprintParts(agent string) ([]Part, error) {
	if agent == f.bad {
		panic("agent.yaml unreadable")
	}
	return f.settingsPorts.FingerprintParts(agent)
}

// A change of the team rules alone is told to the session at its next turn, not restarted for; with anything else it restarts as before.
func TestATeamRulesChangeAloneIsToldNotRestarted(t *testing.T) {
	tk, w, a := settled(t)
	if _, err := tk.Q.Exec(`UPDATE agent_state SET launch_parts = '{"model": "opus", "team rules": "r1"}' WHERE agent = 'a'`); err != nil {
		t.Fatal(err)
	}
	var told, kept []string
	// A tell that fails keeps nothing, so the next tick tells again.
	a.Ports = settingsPorts{parts: []Part{{"model", "opus"}, {RulesPart, "r2"}}, note: "Added:\n+ be brief", told: &told, kept: &kept,
		tellErr: errors.New("no room")}
	a.Sweep(tk, 9_990)
	if len(kept) != 0 {
		t.Fatalf("kept as told when the tell failed: %v", kept)
	}
	a.Ports = settingsPorts{parts: []Part{{"model", "opus"}, {RulesPart, "r2"}}, note: "Added:\n+ be brief", told: &told, kept: &kept}
	a.Sweep(tk, 10_000)
	if w.did("restart") || len(told) != 1 || told[0] != "a: Added:\n+ be brief" || len(kept) != 1 {
		t.Fatalf("%v %v %v", w.calls, told, kept)
	}
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o != nil {
		t.Fatalf("a told change is owed: %+v", o)
	}
	a.Ports = settingsPorts{parts: []Part{{"model", "sonnet"}, {RulesPart, "r2"}}, told: &told, kept: &kept}
	a.Sweep(tk, 10_030)
	if !w.did("restart a by the daemon for model,team rules (settings_applied)") || len(told) != 1 {
		t.Fatalf("%v %v", w.calls, told)
	}
}

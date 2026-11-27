package tick

import (
	"errors"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/actor"
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
	if _, err := tk.Q.Exec(`UPDATE agent_state SET launch_parts = '{"model": "sonnet", "effort": 3, "build": "b1"}' WHERE agent = 'a'`); err != nil {
		t.Fatal(err)
	}
	w.idle, w.restartOK = true, true
	w.status = Status{SessionID: "s1"}
	return tk, w, &ApplySettings{Ports: settingsPorts{parts: []Part{{"model", "opus"}, {"effort", float64(3)}}, account: "acct"}}
}

// byPerson keeps that a person made agent a's newest settings change, as agent.set and an approved proposal do.
func byPerson(t *testing.T, tk *Tick) {
	t.Helper()
	if err := db.SetMeta(tk.Q, db.SettingsByMeta("a"), actor.OwnerKey); err != nil {
		t.Fatal(err)
	}
}

// A person's change of no launch flag (a prompt part, or a setting no launch reads) leaves no name behind for a later Handline change.
func TestWhoChangedIsDroppedWhenNoFlagChanged(t *testing.T) {
	tk, w, a := settled(t)
	if _, err := tk.Q.Exec(`UPDATE agent_state SET launch_parts = '{"model": "opus", "role": "r1", "build": "b1"}' WHERE agent = 'a'`); err != nil {
		t.Fatal(err)
	}
	a.Ports = settingsPorts{parts: []Part{{"model", "opus"}, {"role", "r2"}}, account: "acct"}
	byPerson(t, tk)
	a.Sweep(tk, 10_000)
	if v, _, _ := db.Meta(tk.Q, db.SettingsByMeta("a")); v != "" || w.did("restart") {
		t.Fatalf("who changed outlived a change of no flag: %q %v", v, w.calls)
	}
	// a Handline update moving a flag afterwards is owed, not restarted in the person's name
	a.Ports = settingsPorts{parts: []Part{{"model", "sonnet"}, {"role", "r2"}}, account: "acct"}
	a.Sweep(tk, 10_030)
	if w.did("restart") {
		t.Fatalf("restarted in a stale name: %v", w.calls)
	}
}

// A launch flag nobody changed (a Handline update, a workspace hold) restarts nothing: it is owed until the next fresh start.
func TestAFlagNobodyChangedIsOwedNotApplied(t *testing.T) {
	tk, w, a := settled(t)
	a.Sweep(tk, 10_000)
	if w.did("restart") {
		t.Fatalf("restarted for a change no person made: %v", w.calls)
	}
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o == nil || strings.Join(o.Settings, ",") != "model" || o.Since != 10_000 {
		t.Fatalf("not owed: %+v", o)
	}
}

// A prompt part (the role, the standing context, the team instructions) never restarts a session, even a person's change: a resume
// would not give it, and a fresh start costs the cache. It is owed until the next fresh start. The launch record here names the role
// as records did before the rename, which still reads.
func TestAPromptPartChangeIsOwedEvenWhenAPersonMadeIt(t *testing.T) {
	tk, w, a := settled(t)
	if _, err := tk.Q.Exec(`UPDATE agent_state SET launch_parts = '{"model": "opus", "ROLE.md": "r1", "team instructions": "t1"}' WHERE agent = 'a'`); err != nil {
		t.Fatal(err)
	}
	a.Ports = settingsPorts{parts: []Part{{"model", "opus"}, {"role", "r2"}, {"team instructions", "t1"}}, account: "acct"}
	byPerson(t, tk)
	a.Sweep(tk, 10_000)
	if w.did("restart") || w.hold {
		t.Fatalf("restarted for a prompt part: %v", w.calls)
	}
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o == nil || strings.Join(o.Settings, ",") != "role" {
		t.Fatalf("not owed: %+v", o)
	}
	// once a fresh launch carries it, nothing is owed
	if _, err := tk.Q.Exec(`UPDATE agent_state SET launch_parts = '{"model": "opus", "role": "r2", "team instructions": "t1"}' WHERE agent = 'a'`); err != nil {
		t.Fatal(err)
	}
	a.Sweep(tk, 10_030)
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o != nil {
		t.Fatalf("still owed after the fresh launch: %+v", o)
	}
}

// A person's flag change is applied at an idle moment as a handoff note and a new conversation, in their name, once; what it was for
// is kept, so a launch that still disagrees is not restarted again for the same change.
func TestAPersonsFlagChangeStartsFreshFromTheHandoffNoteOnce(t *testing.T) {
	tk, w, a := settled(t)
	byPerson(t, tk)
	w.hold = true
	a.Sweep(tk, 10_000)
	if w.did("restart") || !w.logged("wait for its handoff note") {
		t.Fatalf("restarted before the note: %v %v", w.calls, w.logs)
	}
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o == nil {
		t.Fatal("the held restart is not shown as owed")
	}
	w.hold = false
	a.Sweep(tk, 10_030)
	if !w.did("restart fresh a by "+actor.OwnerKey+" for model (settings_applied)") || !w.logged("as a new conversation") {
		t.Fatalf("%v %v", w.calls, w.logs)
	}
	if v, _, _ := db.Meta(tk.Q, db.SettingsByMeta("a")); v != "" {
		t.Fatalf("who asked outlived the restart: %q", v)
	}
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o != nil {
		t.Fatalf("owed after its restart: %+v", o)
	}
	w.calls = nil
	byPerson(t, tk)
	a.Sweep(tk, 10_060)
	if w.did("restart") {
		t.Fatalf("restarted twice for one change: %v", w.calls)
	}
	// a prompt part edited while the same flag still disagrees is no new reason to restart
	a.Ports = settingsPorts{parts: []Part{{"model", "opus"}, {"effort", float64(3)}, {"role", "r2"}}, account: "acct"}
	byPerson(t, tk)
	a.Sweep(tk, 10_090)
	if w.did("restart") {
		t.Fatalf("restarted for a prompt part beside a flag a launch cannot give: %v", w.calls)
	}
}

// A person's change made mid-turn is owed, for the agent view to say the restart waits for the turn; the restart clears it.
func TestAPersonsChangeMadeMidTurnIsOwedUntilItsRestart(t *testing.T) {
	tk, w, a := settled(t)
	byPerson(t, tk)
	setState(t, tk, "a", states.Running, 0)
	a.Sweep(tk, 10_000)
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o == nil || o.Since != 10_000 || strings.Join(o.Settings, ",") != "model" || w.did("restart") {
		t.Fatalf("%+v %v", o, w.calls)
	}
	setState(t, tk, "a", states.Idle, 0)
	a.Sweep(tk, 10_030)
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); o != nil || !w.did("restart fresh a") {
		t.Fatalf("%+v %v", o, w.calls)
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
		byPerson(t, tk)
		mutate(tk, w)
		a.Sweep(tk, 10_000)
		if w.did("restart") {
			t.Errorf("%s: restarted", name)
		}
	}
	// A launch that recorded no parts (from before they were kept) is not restarted.
	tk, w, a := settled(t)
	byPerson(t, tk)
	tk.Q.Exec(`UPDATE agent_state SET launch_parts = NULL WHERE agent = 'a'`)
	a.Sweep(tk, 10_000)
	if w.did("restart") {
		t.Error("a launch with no recorded parts was restarted")
	}
}

func TestARestartThatDidNotHappenIsLoggedAndTriedAgainNextTick(t *testing.T) {
	tk, w, a := settled(t)
	byPerson(t, tk)
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
	byPerson(t, tk)
	w.agents = []string{"b", "a"} // b has no row at all
	a.Ports = failingFor{settingsPorts: a.Ports.(settingsPorts), bad: "b"}
	a.Sweep(tk, 10_000)
	if !w.did("restart fresh a") {
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

// A change of the team rules is told to the session at its next turn, never restarted for, also beside another change.
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
	// with a flag that changed too, the rules are still told, and the flag is owed
	a.Ports = settingsPorts{parts: []Part{{"model", "sonnet"}, {RulesPart, "r3"}}, note: "Added:\n+ be kind", told: &told, kept: &kept}
	a.Sweep(tk, 10_030)
	if o, _ := db.GetSettingsOwed(tk.Q, "a"); w.did("restart") || len(told) != 2 || o == nil || strings.Join(o.Settings, ",") != "model" {
		t.Fatalf("%v %v %+v", w.calls, told, o)
	}
}

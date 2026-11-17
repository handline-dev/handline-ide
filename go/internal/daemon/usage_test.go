package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/storetest"
)

// usageEvents are the account.changed rows the accounts topic holds.
func usageEvents(t *testing.T, h *stream.Hub) []map[string]any {
	t.Helper()
	rows, err := h.Log.Since(0, []string{"accounts"}, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, e := range rows {
		var a map[string]any
		if e.Type != "account.changed" || json.Unmarshal(e.Data, &a) != nil {
			t.Fatalf("%s %s", e.Type, e.Data)
		}
		out = append(out, a)
	}
	return out
}

func TestAMoveInAnAccountsUsageIsAnAccountChangedWithTheUsageFields(t *testing.T) {
	t.Setenv("HANDLINE_TEST_ROOT", t.TempDir())
	home := t.TempDir()
	if err := os.MkdirAll(teamtest.AccountDir(t, home, "main"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	d := &Daemon{db: db, cfg: Config{Home: home, Log: func(f string, a ...any) { t.Logf(f, a...) }}, hub: stream.NewHub()}
	d.hub.Log = &stream.Log{DB: db}
	status := func(fiveHour int) {
		t.Helper()
		s := `{"collected_at": 1000, "windows": [` +
			`{"minutes": 10080, "used_percent": 40, "resets_at": 4000000000, "observed_at": 1000},` +
			`{"minutes": 300, "used_percent": ` + strconv.Itoa(fiveHour) + `, "resets_at": 4000000000, "observed_at": 1000}]}`
		if err := os.WriteFile(paths.StatusFile(home, teamtest.Account(t, home, "main")), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	status(10)
	d.sendUsageChanges()
	if got := usageEvents(t, d.hub); len(got) != 0 {
		t.Fatalf("the first look told the topic %v; the snapshot already shows it", got)
	}
	d.sendUsageChanges()
	if got := usageEvents(t, d.hub); len(got) != 0 {
		t.Fatalf("nothing moved, yet the topic was told %v", got)
	}
	status(55)
	d.sendUsageChanges()
	got := usageEvents(t, d.hub)
	if len(got) != 1 {
		t.Fatalf("one move, %d events: %v", len(got), got)
	}
	a := got[0]
	five, _ := a["five_hour"].(map[string]any)
	if a["name"] != "main" || five["used_pct"] != float64(55) {
		t.Fatalf("%v", a)
	}
	for _, k := range []string{"plan", "windows", "assignable", "assign_reason", "not_picked", "agents_assigned", "agents_running"} {
		if _, ok := a[k]; !ok {
			t.Errorf("the event has no %s: %v", k, a)
		}
	}
	if _, ok := a["email"]; ok {
		t.Errorf("the event, which every role gets, has the email: %v", a)
	}
	d.sendUsageChanges()
	if got := usageEvents(t, d.hub); len(got) != 1 {
		t.Fatalf("told again with nothing moved: %v", got)
	}
}

func TestTheClockAloneMovesNoUsageKey(t *testing.T) {
	a := map[string]any{"name": "main", "age_s": 3.0, "factors": map[string]any{"x": 1.0},
		"pace": map[string]any{"state": "behind", "line_pct": 40.1, "used_pct": 30, "rate_pct_h": 0.5}}
	b := map[string]any{"name": "main", "age_s": 9.0, "factors": map[string]any{"x": 2.0},
		"pace": map[string]any{"state": "behind", "line_pct": 40.3, "used_pct": 30, "rate_pct_h": 0.6}}
	if usageKey(a) != usageKey(b) {
		t.Fatalf("%s != %s", usageKey(a), usageKey(b))
	}
	b["pace"].(map[string]any)["state"] = "ahead"
	if usageKey(a) == usageKey(b) {
		t.Fatal("a pace state that moved is not a change")
	}
}

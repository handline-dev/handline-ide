package compose

import (
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/storetest"
)

// As the daemon starts, a start or wake stall is cleared as agent.unstall clears it, with one event each; every other kind stays.
func TestTheDaemonStartClearsStartAndWakeStallsOnly(t *testing.T) {
	d, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	kinds := map[string]verdict.StuckKind{"s": verdict.StuckStart, "w": verdict.StuckWake, "l": verdict.StuckLogin,
		"t": verdict.StuckTrust, "dl": verdict.StuckDialog, "sd": verdict.StuckSendDialog}
	for agent, kind := range kinds {
		if err := db.SetStalled(d, agent, string(kind), "stalled: "+string(kind), nil); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Exec("UPDATE agent_state SET failed_attempts = 3, wake_failures = 3 WHERE agent = ?", agent); err != nil {
			t.Fatal(err)
		}
		if err := db.SetCursor(d, "dialog_since:"+agent, 1); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 { // a second start finds nothing left to clear
		if err := RetryStartStalls(d, t.Logf); err != nil {
			t.Fatal(err)
		}
	}
	for agent, kind := range kinds {
		row, err := db.GetAgentState(d, agent)
		if err != nil {
			t.Fatal(err)
		}
		_, payload, evented, err := db.LastEvent(d, agent, "stall_retried")
		if err != nil {
			t.Fatal(err)
		}
		mark, err := db.Cursor(d, "dialog_since:"+agent)
		if err != nil {
			t.Fatal(err)
		}
		marked := mark != 0
		if kind == verdict.StuckStart || kind == verdict.StuckWake {
			if row.State != states.Queued || row.StallKind.Valid || row.FailedAttempts != 0 || row.WakeFailures != 0 || marked {
				t.Errorf("%s: not cleared: %s %q failed %d wake %d mark %v", agent, row.State, row.StallKind.String, row.FailedAttempts, row.WakeFailures, marked)
			}
			if !evented || payload["kind"] != string(kind) || payload["detail"] != "stalled: "+string(kind) {
				t.Errorf("%s: event %v %v", agent, evented, payload)
			}
			continue
		}
		if row.State != states.Stalled || row.StallKind.String != string(kind) || row.FailedAttempts != 3 || !marked || evented {
			t.Errorf("%s: touched: %s %q failed %d mark %v event %v", agent, row.State, row.StallKind.String, row.FailedAttempts, marked, evented)
		}
	}
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'stall_retried'").Scan(&n); err != nil || n != 2 {
		t.Fatalf("stall_retried events: %d %v", n, err)
	}
}

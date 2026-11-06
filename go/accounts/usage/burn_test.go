package usage

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
)

func burnEnv(t *testing.T) (*Env, db.Q, *int64) {
	t.Helper()
	now := clock
	e := &Env{Home: t.TempDir(), Now: func() float64 { return float64(now) }, PlanKey: func(string) string { return "pro" }}
	e.AgentConfig = func(agent string) (team.Config, error) {
		if agent == "gone" {
			return nil, os.ErrNotExist
		}
		return team.Config{"model": map[string]string{"x": "opus", "y": "opus", "z": "sonnet"}[agent], "effort": "high"}, nil
	}
	return e, openDB(t), &now
}

// liveStatus is a reading of the shortest window taken at `at`, as the poll leaves it.
func liveStatus(at int64, pct int, reset int64) *AccountStatus {
	p := pct
	return &AccountStatus{Account: "a", Exists: true, CollectedAt: &at, AsOf: ptrF(float64(at)),
		Short: AcctWindow{Minutes: 300, Pct: &p, Reset: reset, At: &at, Readable: true},
		Long:  AcctWindow{Minutes: 10080, Readable: true}}
}

func ptrF(f float64) *float64 { return &f }

func TestRecordSampleWritesWhatTheStatusHeldAndPruneKeepsTwoWeeks(t *testing.T) {
	e, q, now := burnEnv(t)
	st := liveStatus(clock-30, 12, 777)
	st.Long = AcctWindow{Readable: false}
	if err := e.RecordSample(q, "a", st, 0, false); err != nil {
		t.Fatal(err)
	}
	if err := e.RecordSample(q, "a", &AccountStatus{Account: "b", Error: "unreadable", Short: AcctWindow{Readable: true}, Long: AcctWindow{Readable: true}}, *now-15*86400, false); err != nil {
		t.Fatal(err)
	}
	rows, err := readSamples(q, 0)
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %v", rows, err)
	}
	old, fresh := rows[0], rows[1]
	if ws, isList := old["windows"].([]any); old["error"] != "unreadable" || !isList || len(ws) != 0 || old["collected_at"] != nil {
		t.Fatalf("a status with no windows and an error: %v", old)
	}
	if fresh["ts"] != json.Number(itoa(int(clock))) || fresh["exists_"] != json.Number("1") {
		t.Fatalf("stamped now: %v", fresh)
	}
	es := entries(fresh)
	if len(es) != 2 || es[1]["used_percent"] != json.Number("12") || es[1]["resets_at"] != json.Number("777") || es[0]["readable"] != false ||
		es[0]["used_percent"] != nil {
		t.Fatalf("every figure as the status has it, the unreadable window (minutes 0) sorting first: %v", es)
	}
	n, err := e.PruneSamples(q)
	if err != nil || n != 1 {
		t.Fatalf("pruned %d %v", n, err)
	}
}

// A sample says whether its account could be used when it was taken.
func TestRecordSampleSaysWhetherTheAccountWasLoggedOut(t *testing.T) {
	e, q, _ := burnEnv(t)
	for _, out := range []bool{false, true} {
		if err := e.RecordSample(q, "a", liveStatus(clock-30, 12, 777), 0, out); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := readSamples(q, 0)
	if err != nil || len(rows) != 2 || rows[0]["logged_out"] != json.Number("0") || rows[1]["logged_out"] != json.Number("1") {
		t.Fatalf("in, then out: %v %v", rows, err)
	}
}

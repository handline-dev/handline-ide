package usagepoll

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/storetest"
)

const now = 100000

type fakeReader struct {
	asked []string
	give  map[string]*usage.Reading
}

func (f *fakeReader) Read(_ context.Context, acct string) (*usage.Reading, bool) {
	f.asked = append(f.asked, acct)
	return f.give[acct], false
}

func pc(n int) *int { return &n }

func reading(acct string, p5, p7 int, reset5 int64) *usage.Reading {
	return &usage.Reading{Account: acct, At: now, Windows: []usage.ReadingWindow{
		{Minutes: 300, Pct: pc(p5), Reset: reset5}, {Minutes: 10080, Pct: pc(p7), Reset: now + 50000}}}
}

type rig struct {
	home  string
	db    db.Q
	m     *fakeReader
	p     *Poller
	rec   []string
	recOK bool
}

func newRig(t *testing.T, accts ...string) *rig {
	t.Helper()
	home := t.TempDir()
	for _, a := range accts {
		if err := os.MkdirAll(paths.AccountDir(home, a), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	d, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	r := &rig{home: home, db: d, m: &fakeReader{give: map[string]*usage.Reading{}}}
	env := &usage.Env{Home: home, Now: func() float64 { return now }, Metered: func(string) bool { return true }}
	r.p = &Poller{DB: d, Env: env, Home: home, Meter: r.m, LoggedIn: func(string) bool { return true },
		Now: func() int64 { return now }, Recover: func(a string) (bool, string) { r.rec = append(r.rec, a); return r.recOK, a + " recovered" }}
	return r
}

func (r *rig) seed(t *testing.T, acct string, at int64, reset5 int64) {
	t.Helper()
	if !usage.Record(r.home, usage.Reading{Account: acct, At: at, Windows: []usage.ReadingWindow{
		{Minutes: 300, Pct: pc(10), Reset: reset5}, {Minutes: 10080, Pct: pc(10), Reset: now + 50000}}}) {
		t.Fatal("seed not written")
	}
}

func TestAnAccountWithNoReadingIsAskedAndRecorded(t *testing.T) {
	r := newRig(t, "a")
	r.m.give["a"] = reading("a", 35, 39, now+3600)
	r.p.Tick(context.Background())
	if len(r.m.asked) != 1 {
		t.Fatalf("asked %v", r.m.asked)
	}
	st := r.p.Env.Read("a")
	if st.Short.Pct == nil || *st.Short.Pct != 35 {
		t.Errorf("recorded %+v", st.Short)
	}
	if c, _ := db.Cursor(r.db, "usage-poll:a"); c != now+states.UsagePollS {
		t.Errorf("cursor %d", c)
	}
}

func TestAFreshAccountWithNoTurnSinceIsLeftAlone(t *testing.T) {
	r := newRig(t, "a")
	r.seed(t, "a", now-10, now+3600)
	r.p.Tick(context.Background())
	if len(r.m.asked) != 0 {
		t.Errorf("asked %v", r.m.asked)
	}
}

func TestATurnSinceTheReadingMakesItDue(t *testing.T) {
	r := newRig(t, "a")
	r.seed(t, "a", now-10, now+3600)
	prev := db.Now
	db.Now = func() int64 { return now - 5 }
	t.Cleanup(func() { db.Now = prev })
	if _, err := db.AddEventFor(r.db, "x", "a", "Stop"); err != nil {
		t.Fatal(err)
	}
	r.m.give["a"] = reading("a", 12, 10, now+3600)
	r.p.Tick(context.Background())
	if len(r.m.asked) != 1 {
		t.Errorf("asked %v", r.m.asked)
	}
}

func TestOldestReadingFirstAndOneSuccessPerTick(t *testing.T) {
	r := newRig(t, "a", "b")
	r.seed(t, "a", now-1000, now+3600)
	r.seed(t, "b", now-2000, now+3600)
	r.m.give["a"], r.m.give["b"] = reading("a", 1, 1, now+3600), reading("b", 1, 1, now+3600)
	r.p.Tick(context.Background())
	if len(r.m.asked) != 1 || r.m.asked[0] != "b" {
		t.Errorf("asked %v, want only the oldest, b", r.m.asked)
	}
}

func TestAFailingAccountBacksOffAndDoesNotTakeTheTick(t *testing.T) {
	r := newRig(t, "a", "b")
	r.seed(t, "a", now-5000, now+3600) // older: asked first, fails
	r.seed(t, "b", now-1000, now+3600)
	r.m.give["b"] = reading("b", 2, 2, now+3600)
	r.p.Tick(context.Background())
	if len(r.m.asked) != 2 || r.m.asked[1] != "b" {
		t.Fatalf("asked %v", r.m.asked)
	}
	if c, _ := db.Cursor(r.db, "usage-poll:a"); c != now+states.UsagePollS {
		t.Errorf("first failure backs off %d", c-now)
	}
	// Second failure doubles; the streak caps.
	r.p.backoff("a", now)
	if c, _ := db.Cursor(r.db, "usage-poll:a"); c != now+2*states.UsagePollS {
		t.Errorf("second failure backs off %d", c-now)
	}
	for i := 0; i < 10; i++ {
		r.p.backoff("a", now)
	}
	if c, _ := db.Cursor(r.db, "usage-poll:a"); c != now+states.UsageFailBackoffCapS {
		t.Errorf("cap: %d", c-now)
	}
	r.m.asked = nil
	r.p.Tick(context.Background())
	for _, a := range r.m.asked {
		if a == "a" {
			t.Error("a was asked inside its backoff")
		}
	}
}

func TestADeadTokenIsRecoveredAndTakesTheTick(t *testing.T) {
	r := newRig(t, "a", "b")
	r.recOK = true
	r.p.Tick(context.Background())
	if len(r.rec) != 1 || len(r.m.asked) != 1 {
		t.Errorf("recovered %v, asked %v", r.rec, r.m.asked)
	}
}

func TestAnEndedWindowEarnsOneRealTurnPerReset(t *testing.T) {
	r := newRig(t, "a")
	r.seed(t, "a", now-100, now-10) // 5h window ended 10 s ago
	r.recOK = true
	r.m.give["a"] = reading("a", 0, 10, now+18000)
	r.p.Tick(context.Background())
	if len(r.rec) != 1 {
		t.Fatalf("recovered %v", r.rec)
	}
	if v, _, _ := db.Meta(r.db, "reset-ping:a:5h"); v != "99990" {
		t.Errorf("reset-ping = %q", v)
	}
	db.ClearCursor(r.db, "usage-poll:a")
	r.p.Tick(context.Background())
	if len(r.rec) != 1 {
		t.Errorf("a second turn for the same reset: %v", r.rec)
	}
}

func TestAWindowStillReportedExhaustedHasNotEnded(t *testing.T) {
	r := newRig(t, "a")
	r.seed(t, "a", now-100, now-10)
	st := r.p.Env.Read("a")
	if _, _, ok := EndedWindow(st, reading("a", 100, 10, now+60), now); ok {
		t.Error("exhausted and not reset must not be called ended")
	}
	if _, _, ok := EndedWindow(st, reading("a", 100, 10, 0), now); ok {
		t.Error("exhausted with no reset must not be called ended")
	}
	if l, _, ok := EndedWindow(st, reading("a", 3, 10, now+60), now); !ok || l != "5h" {
		t.Errorf("ended = %q %v", l, ok)
	}
}

func TestNotLoggedInIsLeftForTheLoginCheck(t *testing.T) {
	r := newRig(t, "a")
	r.p.LoggedIn = func(string) bool { return false }
	r.p.Tick(context.Background())
	if len(r.m.asked)+len(r.rec) != 0 {
		t.Error("asked a logged-out account")
	}
	if c, _ := db.Cursor(r.db, "usage-poll:a"); c != now+states.LoginCheckCooldownS {
		t.Errorf("cursor %d", c)
	}
}

func askedNow(t *testing.T, r *rig, acct string) bool {
	t.Helper()
	_, ok, err := db.Meta(r.db, readAskKey(acct))
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestAnAskedAccountIsReadFirstPastItsCadenceAndBackoff(t *testing.T) {
	r := newRig(t, "a", "b")
	r.seed(t, "a", now-10, now+3600) // fresh, nothing due
	r.seed(t, "b", now-2000, now+3600)
	_ = db.SetCursor(r.db, "usage-poll:a", now+states.UsageFailBackoffCapS)
	r.m.give["a"], r.m.give["b"] = reading("a", 40, 1, now+3600), reading("b", 1, 1, now+3600)
	if err := AskRead(r.db, "a", now-1); err != nil {
		t.Fatal(err)
	}
	r.p.Tick(context.Background())
	if len(r.m.asked) != 1 || r.m.asked[0] != "a" {
		t.Fatalf("asked %v, want only the asked a", r.m.asked)
	}
	if st := r.p.Env.Read("a"); st.Short.Pct == nil || *st.Short.Pct != 40 {
		t.Errorf("recorded %+v", st.Short)
	}
	if askedNow(t, r, "a") {
		t.Error("the ask outlived its read")
	}
	r.m.asked = nil
	r.p.Tick(context.Background())
	if len(r.m.asked) != 1 || r.m.asked[0] != "b" {
		t.Errorf("next tick asked %v, want the polled b", r.m.asked)
	}
}

func TestAskingTwiceBeforeTheReadIsOneRead(t *testing.T) {
	r := newRig(t, "a")
	r.seed(t, "a", now-10, now+3600)
	r.m.give["a"] = reading("a", 5, 1, now+3600)
	_ = AskRead(r.db, "a", now-2)
	_ = AskRead(r.db, "a", now) // during the read: the same ask
	r.p.Tick(context.Background())
	r.p.Tick(context.Background())
	if len(r.m.asked) != 1 {
		t.Errorf("asked %v, want one read", r.m.asked)
	}
}

func TestAnAskedReadOfABackedOffAccountSpendsNoTurn(t *testing.T) {
	r := newRig(t, "a")
	r.seed(t, "a", now-10, now+3600)
	_ = db.SetCursor(r.db, "usage-poll:a", now+states.UsagePollS)
	_ = AskRead(r.db, "a", now)
	r.p.Tick(context.Background())
	if len(r.m.asked) != 1 || len(r.rec) != 0 {
		t.Fatalf("asked %v, recovered %v: a backed-off account was read once and spent no turn", r.m.asked, r.rec)
	}
	if askedNow(t, r, "a") {
		t.Error("the failed read left the ask")
	}
}

func TestAnAskedReadCutShortByAStopKeepsTheAsk(t *testing.T) {
	r := newRig(t, "a")
	r.seed(t, "a", now-10, now+3600)
	_ = AskRead(r.db, "a", now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.p.pollAccount(ctx, "a", now)
	if !askedNow(t, r, "a") {
		t.Error("a read cut short by a stop dropped the ask")
	}
}

func TestAFailedAskedReadIsBackedOffAndAnswered(t *testing.T) {
	r := newRig(t, "a")
	r.seed(t, "a", now-10, now+3600)
	_ = AskRead(r.db, "a", now)
	r.p.Tick(context.Background())
	if len(r.m.asked) != 1 || len(r.rec) != 1 {
		t.Fatalf("asked %v, recovered %v", r.m.asked, r.rec)
	}
	if c, _ := db.Cursor(r.db, "usage-poll:a"); c != now+states.UsagePollS {
		t.Errorf("backoff %d", c-now)
	}
	if askedNow(t, r, "a") {
		t.Error("a failed read left the ask")
	}
}

func TestAnAskedAccountNotLoggedInIsNotRead(t *testing.T) {
	r := newRig(t, "a")
	r.p.LoggedIn = func(string) bool { return false }
	_ = AskRead(r.db, "a", now)
	r.p.Tick(context.Background())
	if len(r.m.asked) != 0 || askedNow(t, r, "a") {
		t.Errorf("asked %v, ask kept %v", r.m.asked, askedNow(t, r, "a"))
	}
}

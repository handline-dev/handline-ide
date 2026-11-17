package lifecycle

import (
	"strings"
	"testing"
)

type fakeRecover struct {
	enabled, renews, in bool
	week                int
	weekKnown, turnOK   bool
	reading             Reading
	auth                bool
	spent               int
	errType             string // what a failed turn names
	stalled             string // the error the account was stalled on
}

func (f *fakeRecover) UsageRecoverEnabled(string) bool { return f.enabled }
func (f *fakeRecover) RenewsByTurn(string) (bool, string, error) {
	return f.renews, "codex", nil
}
func (f *fakeRecover) LoggedIn(string) bool                   { return f.in }
func (f *fakeRecover) LongTrusted(string) (int, string, bool) { return f.week, "weekly", f.weekKnown }
func (f *fakeRecover) LongBlock() int                         { return 95 }
func (f *fakeRecover) RunModel(string, string, string) (bool, string, string) {
	f.spent++
	if f.turnOK {
		return true, "hi", ""
	}
	return false, "boom", f.errType
}
func (f *fakeRecover) Stall(_, errType, _ string) error { f.stalled = errType; return nil }
func (f *fakeRecover) ReadUsage(string) (Reading, bool) { return f.reading, f.auth }
func (f *fakeRecover) Record(Reading)                   {}
func (f *fakeRecover) Describe(Reading) string          { return "5h 3%" }

func TestRecoverRefusesBeforeSpendingAndMarksAnAuthRefusal(t *testing.T) {
	f := &fakeRecover{enabled: true, renews: true, in: true, weekKnown: true, week: 97, turnOK: true}
	r := &Recoverer{Home: t.TempDir(), Ports: f}
	if ok, why := r.Recover("a"); ok || !strings.Contains(why, "at the wall") || f.spent != 0 {
		t.Fatalf("%v %q %d", ok, why, f.spent)
	}
	f.week = 10
	f.auth = true
	if ok, why := r.Recover("a"); ok || !strings.Contains(why, "marked needs re-login") || f.spent != 1 {
		t.Fatalf("%v %q", ok, why)
	}
	if ok, why := r.Recover("a"); ok || why != "a needs re-login - not retrying" || f.spent != 1 {
		t.Fatalf("%v %q", ok, why)
	}
	r.ClearRelogin("a")
	f.reading = 1
	if ok, why := r.Recover("a"); !ok || why != "a: recovered - 5h 3%" {
		t.Fatalf("%v %q", ok, why)
	}
}

func TestARecoveryTurnRefusedForTheAccountStallsIt(t *testing.T) {
	f := &fakeRecover{enabled: true, renews: true, in: true, errType: "oauth_org_not_allowed"}
	r := &Recoverer{Home: t.TempDir(), Ports: f}
	if ok, why := r.Recover("a"); ok || f.stalled != "oauth_org_not_allowed" || !strings.Contains(why, "stalled until a login or a wake") {
		t.Fatalf("%v %q stalled %q", ok, why, f.stalled)
	}
	f = &fakeRecover{enabled: true, renews: true, in: true, errType: "server_error"}
	r = &Recoverer{Home: t.TempDir(), Ports: f}
	if ok, why := r.Recover("a"); ok || f.stalled != "" || !strings.Contains(why, "recovery turn failed: boom") {
		t.Fatalf("a passing error stalls nothing: %v %q stalled %q", ok, why, f.stalled)
	}
}

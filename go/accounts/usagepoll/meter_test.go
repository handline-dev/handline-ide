package usagepoll

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/accounts/claudecli"
	"github.com/handline/handline/go/accounts/livetest"
	"github.com/handline/handline/go/data/team/teamtest"
)

type fakeCLI struct {
	resp   map[string]any
	err    error
	sent   string
	result map[string]any
	before []map[string]any // frames the CLI writes before the result, in order
	h      claudecli.Handlers
	closed bool
}

func (f *fakeCLI) GetUsage(context.Context) (map[string]any, error) { return f.resp, f.err }
func (f *fakeCLI) Close() error                                     { f.closed = true; return nil }
func (f *fakeCLI) Send(text string) error {
	f.sent = text
	if f.result != nil {
		go func() {
			for _, ev := range f.before {
				f.h.OnEvent(ev)
			}
			f.h.OnEvent(f.result)
		}()
	}
	return nil
}

func meterWith(t *testing.T, f *fakeCLI, openErr error) (*Meter, claudecli.Launch) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(teamtest.AccountDir(t, home, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	var got claudecli.Launch
	return &Meter{Home: home, Claude: "claude", Now: func() time.Time { return time.Unix(now, 0) },
		Open: func(_ context.Context, l claudecli.Launch, h claudecli.Handlers) (CLI, error) {
			got = l
			f.h = h
			return f, openErr
		}}, got
}

func answer(five, seven any, reset string) map[string]any {
	return map[string]any{"rate_limits_available": true, "rate_limits": map[string]any{
		"five_hour": map[string]any{"utilization": five, "resets_at": reset},
		"seven_day": map[string]any{"utilization": seven, "resets_at": nil}}}
}

func TestReadTurnsTheAnswerIntoAReading(t *testing.T) {
	f := &fakeCLI{resp: answer(34.6, 39.0, "2026-10-09T05:00:00.5+00:00")}
	m, _ := meterWith(t, f, nil)
	r, authFailed := m.Read(context.Background(), "a")
	if r == nil || authFailed {
		t.Fatalf("read %v %v", r, authFailed)
	}
	if *r.Windows[0].Pct != 35 || *r.Windows[1].Pct != 39 || r.Windows[0].Minutes != 300 || r.Windows[1].Minutes != 10080 {
		t.Errorf("windows %+v %+v", r.Windows[0], r.Windows[1])
	}
	if r.Windows[0].Reset != time.Date(2026, 10, 9, 5, 0, 0, 0, time.UTC).Unix() || r.Windows[1].Reset != 0 {
		t.Errorf("resets %d %d", r.Windows[0].Reset, r.Windows[1].Reset)
	}
	if r.At != now || !f.closed {
		t.Errorf("at %d, closed %v", r.At, f.closed)
	}
}

func TestTheProcessGetsNoAccountSettings(t *testing.T) {
	m, _ := meterWith(t, &fakeCLI{resp: answer(1.0, 1.0, "")}, nil)
	l, cleanup, err := m.launch("a")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if len(l.Args) != 2 || l.Args[0] != "--setting-sources" || l.Args[1] != "project" {
		t.Errorf("launch %+v", l)
	}
	if filepath.Base(l.ConfigDir) != ".claude" {
		t.Errorf("config dir %q", l.ConfigDir)
	}
}

func TestNoFigureIsNoReading(t *testing.T) {
	for name, resp := range map[string]map[string]any{
		"unavailable": {"rate_limits_available": false, "rate_limits": nil},
		"null":        {"rate_limits_available": true, "rate_limits": nil},
		"empty":       {"rate_limits_available": true, "rate_limits": map[string]any{}},
		"badreset":    answer(10.0, 10.0, "tomorrow"),
	} {
		m, _ := meterWith(t, &fakeCLI{resp: resp}, nil)
		if r, _ := m.Read(context.Background(), "a"); r != nil {
			t.Errorf("%s: got a reading %+v", name, r)
		}
	}
}

func TestARefusedLoginIsToldApartFromATimeout(t *testing.T) {
	m, _ := meterWith(t, &fakeCLI{err: errors.New("usage request failed: 401 Unauthorized")}, nil)
	if r, auth := m.Read(context.Background(), "a"); r != nil || !auth {
		t.Errorf("401: %v %v", r, auth)
	}
	m, _ = meterWith(t, &fakeCLI{err: errors.New("deadline exceeded")}, nil)
	if r, auth := m.Read(context.Background(), "a"); r != nil || auth {
		t.Errorf("timeout: %v %v", r, auth)
	}
}

func TestRunModelIsOneTurnAndReadsTheResultFrame(t *testing.T) {
	f := &fakeCLI{result: map[string]any{"type": "result", "result": "hello"}}
	m, _ := meterWith(t, f, nil)
	if ok, out, e := m.RunModel(context.Background(), "a", "hi", "haiku"); !ok || out != "hello" || e != "" || f.sent != "hi" {
		t.Errorf("run %v %q %q sent %q", ok, out, e, f.sent)
	}
	f = &fakeCLI{result: map[string]any{"type": "result", "is_error": true, "result": "Not logged in"}}
	m, _ = meterWith(t, f, nil)
	if ok, out, e := m.RunModel(context.Background(), "a", "hi", "haiku"); ok || out != "Not logged in" || e != "" {
		t.Errorf("run %v %q %q", ok, out, e)
	}
}

// TestRunModelNamesTheErrorTheTurnEndedOn: a subscription refused for Claude Code (403 oauth_not_allowed_for_organization, a payment
// past due) comes back as the assistant frame's error, which recovery stalls the account on.
func TestRunModelNamesTheErrorTheTurnEndedOn(t *testing.T) {
	f := &fakeCLI{before: []map[string]any{{"type": "assistant", "error": "oauth_org_not_allowed", "api_error_status": 403.0}},
		result: map[string]any{"type": "result", "is_error": true, "result": "Claude Code can't use your Claude subscription"}}
	m, _ := meterWith(t, f, nil)
	if ok, out, e := m.RunModel(context.Background(), "a", "hi", "haiku"); ok || e != "oauth_org_not_allowed" || !strings.Contains(out, "subscription") {
		t.Errorf("run %v %q %q", ok, out, e)
	}
}

// TestLiveMeter asks a real CLI for the picked account's usage through the Meter, from a scratch home whose account dir is the real
// one's (livetest sets HANDLINE_LIVE_ACCOUNTS). HANDLINE_LIVE=1 go test -run TestLiveMeter -v ./accounts/usagepoll
func TestLiveMeter(t *testing.T) {
	a := livetest.Pick(t)
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Fatal(err)
	}
	m := &Meter{Home: t.TempDir(), Claude: path}
	c, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r, auth := m.Read(c, a.Name)
	if r == nil {
		t.Fatalf("no reading (authFailed %v)", auth)
	}
	t.Logf("reading: %s resets %d / %d", Describe(r), r.Windows[0].Reset, r.Windows[1].Reset)
}

// TestLiveLoggedOutDirectory is the dead-login check that needs no credential: a config dir the CLI has no login in. The Meter must
// return no reading (and say what the CLI said), not hang or read as a quiet account.
func TestLiveLoggedOutDirectory(t *testing.T) {
	livetest.Require(t)
	m := liveMeter(t, t.TempDir())
	c, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r, auth := m.Read(c, "live")
	t.Logf("reading %v, authFailed %v", r, auth)
	if r != nil {
		t.Error("a logged-out directory gave a reading")
	}
}

func liveMeter(t *testing.T, cfg string) *Meter {
	t.Helper()
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.MkdirAll(teamtest.AccountDir(t, home, "live"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cfg, filepath.Join(teamtest.AccountDir(t, home, "live"), ".claude")); err != nil {
		t.Fatal(err)
	}
	return &Meter{Home: home, Claude: path}
}

// extra_usage as the live answer has it: every field present, the amounts in cents and null while it is off. Kept by allowlist, in
// dollars; no object is unknown, never zero.
func TestReadKeepsExtraUsageInMajorUnits(t *testing.T) {
	extra := func(on bool, limit, used, places any) map[string]any {
		return map[string]any{"is_enabled": on, "monthly_limit": limit, "used_credits": used, "utilization": nil, "currency": "USD",
			"spend_limit_reached": false, "user_disabled": !on, "credits_ever_enabled": true, "decimal_places": places, "daily": nil, "weekly": nil}
	}
	for name, c := range map[string]struct {
		extra any
		want  string
	}{
		"off, as every live account was": {extra(false, nil, nil, nil), "map[cap:<nil> currency:USD on:false spent:<nil>]"},
		"on with spend":                  {extra(true, 5000.0, 1450.0, 2.0), "map[cap:50 currency:USD on:true spent:14.5]"},
		"on, no cap, three places":       {extra(true, nil, 1500.0, 3.0), "map[cap:<nil> currency:USD on:true spent:1.5]"},
		"no extra_usage object":          {nil, "<nil>"},
	} {
		resp := answer(34.6, 39.0, "")
		if c.extra != nil {
			resp["rate_limits"].(map[string]any)["extra_usage"] = c.extra
		}
		f := &fakeCLI{resp: resp}
		m, _ := meterWith(t, f, nil)
		r, _ := m.Read(context.Background(), "a")
		if r == nil {
			t.Fatalf("%s: no reading", name)
		}
		if got := fmt.Sprint(r.Extra.Map()); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

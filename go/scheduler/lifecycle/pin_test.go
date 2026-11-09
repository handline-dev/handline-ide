package lifecycle

import (
	"os"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
)

// pinVendors is fakeVendors with a vendor per account.
type pinVendors struct {
	fakeVendors
	of map[string]string
}

func (v pinVendors) AccountVendor(acct string) string { return v.of[acct] }

// pinRig is ann on two claude accounts, a and b, where the pick would take b; pin is what ann's agent.yaml says after vendor and model.
func pinRig(t *testing.T, pin string) *launchRig {
	r := newLaunchRig(t)
	for _, a := range []string{"a", "b"} {
		if err := os.MkdirAll(paths.AccountDir(r.l.Home, a), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	yaml := "vendor: claude\nmodel: m1\neffort: high\n" + pin
	if err := os.WriteFile(r.l.Roster.AgentFile("ann"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	r.l.Ports.Vendors = pinVendors{of: map[string]string{"a": "claude", "b": "claude"}}
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, pick: "b"}
	return r
}

func TestAPinnedAgentRunsOnlyOnItsAccountAndNothingMovesIt(t *testing.T) {
	r := pinRig(t, "account: a\n")
	if got := r.l.FindRoom("ann"); got != "a" {
		t.Fatalf("the pin, not the pick: %q", got)
	}
	if got := r.l.ChooseAccount("ann", false, "b"); got != "" {
		t.Fatalf("a destination other than the pin: %q", got)
	}
	if got := r.l.ChooseAccount("ann", false, "a"); got != "a" {
		t.Fatalf("the pin as a destination: %q", got)
	}
	r.l.Ports.CurrentAccount = func(db.Q, string) string { return "b" }
	if got := r.l.ChooseAccount("ann", true, ""); got != "a" {
		t.Fatalf("a resume recorded on b goes to the pin: %q", got)
	}
	if why := r.l.RoomOn(r.l.Q, "ann", nil, "b"); !strings.Contains(why, "pinned to") {
		t.Fatalf("a person's move elsewhere: %q", why)
	}
	if dest, why := r.l.RoomElsewhere(r.l.Q, "ann", nil, nil); dest != "" || why != "it is pinned to a" {
		t.Fatalf("the mover: %q %q", dest, why)
	}
	if got := pinRig(t, "").l.FindRoom("ann"); got != "b" {
		t.Fatalf("an unpinned agent goes where the pick says: %q", got)
	}
}

func TestAPinnedAgentWhoseAccountCannotTakeItWaitsWithThatAccountsReason(t *testing.T) {
	r := pinRig(t, "account: a\n")
	pct := 96
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, pick: "b", by: map[string]AccountStatus{"a": {Exists: true, ShortTrusted: &pct}}}
	if got := r.l.FindRoom("ann"); got != "" {
		t.Fatalf("a metered pin is held to its room, and b is no fallback: %q", got)
	}
	if why := r.l.NoRoom("ann"); !strings.HasPrefix(why, "it is pinned to a, which cannot take it: 5h usage at the wall") {
		t.Fatalf("%q", why)
	}
}

func TestARefusedLaunchIsRecorded(t *testing.T) {
	r := pinRig(t, "account: nowhere\n")
	if _, err := r.l.Prepare(&Site{Cwd: t.TempDir()}, "ann", nil, false, "", "", "", ""); err == nil {
		t.Fatal("it was not refused")
	}
	if _, ok, _ := db.LastEventID(r.l.Q, "ann", LaunchRefusedKind); !ok {
		t.Fatal("the refusal left no event")
	}
}

func TestAPinToAnotherVendorsAccountIsRefusedNamingBoth(t *testing.T) {
	r := pinRig(t, "account: a\n")
	r.l.Ports.Vendors = pinVendors{of: map[string]string{"a": "opencode", "b": "claude"}}
	if got := r.l.FindRoom("ann"); got != "" {
		t.Fatalf("%q", got)
	}
	if why := r.l.NoRoom("ann"); why != "it is pinned to a, a opencode account, but it runs claude" {
		t.Fatalf("%q", why)
	}
}

func TestAnUnmeteredPinTakesItsAgentWithNoRoomTestButStillNeedsALogin(t *testing.T) {
	r := pinRig(t, "account: a\n")
	pct := 99 // a reading at the wall would refuse a metered account; an unmetered one has no room to test
	r.l.Ports.Usage = fakeUsage{st: AccountStatus{Exists: true}, pick: "b", by: map[string]AccountStatus{"a": {Unmetered: true, ShortTrusted: &pct}}}
	if got := r.l.FindRoom("ann"); got != "a" {
		t.Fatalf("%q", got)
	}
	r.l.Ports.Accounts = fakeAccounts{false}
	if got, why := r.l.FindRoom("ann"), r.l.NoRoom("ann"); got != "" || why != "it is pinned to a, which is not logged in or may not start" {
		t.Fatalf("%q %q", got, why)
	}
}

func TestAPinWithSpacesIsTheAccountItNames(t *testing.T) {
	if got := pinRig(t, "account: '  a '\n").l.PinOf("ann"); got != "a" {
		t.Fatalf("%q", got)
	}
}

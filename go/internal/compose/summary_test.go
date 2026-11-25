package compose

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
)

// The handoff note is asked on an account a launch here may use: one found needing a login is passed over even when it alone has room.
func TestTheHandoffNoteNeverPicksAnAccountThatNeedsALogin(t *testing.T) {
	p, _, _, _ := wired(t) // a1 logged in
	home := p.home
	alice := teamtest.Key(t, home, "alice")
	dir := filepath.Join(teamtest.AccountDir(t, home, "omega"), ".claude")
	cred := filepath.Join(dir, ".credentials.json")
	old := time.Now().Add(-time.Hour) // written before the refusal: no login since
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cred, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(cred, old, old); err != nil {
		t.Fatal(err)
	}
	atPct(t, home, "a1", 100)
	atPct(t, home, "omega", 0) // nothing runs on it, so it looks empty
	if _, err := (team.Accounts{Home: home}).StallAccount("omega", "authentication_failed", "OAuth session expired", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.summary.Run(context.Background(), alice, "note"); err == nil || err.Error() != "no account has room" {
		t.Fatalf("%v, want no account has room", err)
	}
}

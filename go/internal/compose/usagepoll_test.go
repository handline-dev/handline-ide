package compose

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/storetest"
)

// A recovery turn refused for the account stalls it once, with the login_expired event the founder's notification is made from.
func TestARecoveryStallRecordsTheEventOnce(t *testing.T) {
	home := t.TempDir()
	d, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	accounts := team.Accounts{Home: home}
	r := recoverPorts{home: home, accounts: accounts, q: d, now: func() time.Time { return time.Unix(1_800_000_000, 0) }}
	for range 2 {
		if err := r.Stall("beta", "oauth_org_not_allowed", "payment past due"); err != nil {
			t.Fatal(err)
		}
	}
	if s := accounts.Stalled("beta"); s == nil || s.Error != "oauth_org_not_allowed" || s.At != 1_800_000_000 {
		t.Fatalf("stall %+v", s)
	}
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'login_expired' AND account = 'beta' AND agent = ''").Scan(&n); err != nil || n != 1 {
		t.Fatalf("login_expired events %d %v", n, err)
	}
}

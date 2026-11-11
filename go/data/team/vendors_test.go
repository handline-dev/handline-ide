package team

import (
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/kit/paths"
)

// A live run's scratch home takes its accounts' config dirs from HANDLINE_LIVE_ACCOUNTS, where the keychain keeps their logins.
func TestAccountConfigDirIsTheLiveAccountsOne(t *testing.T) {
	scratch, real := t.TempDir(), t.TempDir()
	t.Setenv("HANDLINE_TEST_ROOT", t.TempDir())
	t.Setenv("HANDLINE_"+LiveAccounts, "")
	got, err := AccountConfigDir(scratch, "theta")
	if err != nil || got != filepath.Join(paths.AccountDir(scratch, "theta"), ".claude") {
		t.Fatalf("unset: %q %v", got, err)
	}
	t.Setenv("HANDLINE_"+LiveAccounts, real)
	want := filepath.Join(accountDir(t, real, "theta"), ".claude")
	got, err = AccountConfigDir(scratch, "theta")
	if err != nil || got != want {
		t.Fatalf("set: %q %v", got, err)
	}
	if _, err := AccountConfigDir(scratch, "iota"); err == nil {
		t.Fatal("an account the real home has not")
	}
}

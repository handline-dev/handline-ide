package team

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/kit/paths"
)

// sharer is a CLI whose login is one file, auth.json, in its config dir (codex's shape); "unsure" there is a login it cannot tell.
type sharer struct{ name string }

func (s sharer) Name() string                     { return s.name }
func (sharer) ConfigDirName() string              { return ".sharer" }
func (sharer) ConfigDirEnv() string               { return "SHARER_HOME" }
func (sharer) LoginArgv() []string                { return []string{"true"} }
func (sharer) LogoutArgv() []string               { return nil }
func (sharer) WebLoginArgv() ([]string, bool)     { return nil, false }
func (sharer) CredentialModified(string) *float64 { return nil }
func (sharer) CredentialFile() string             { return "auth.json" }
func (sharer) CredentialPresent(dir string) (bool, bool) {
	if _, err := os.Stat(filepath.Join(dir, "unsure")); err == nil {
		return false, false
	}
	_, err := os.Stat(filepath.Join(dir, "auth.json"))
	return err == nil, true
}

// keyed is a CLI whose login is tied to its folder (claude's shape): found, never adopted. Its CredentialFile, of another signature,
// hides sharer's, so it is no agent.Sharable.
type keyed struct{ sharer }

func (keyed) CredentialFile() {}

func discoverHome(t *testing.T) (home, user string) {
	t.Helper()
	home, user = newHome(t), t.TempDir()
	was, wasList := agent.UserHome, discoverable
	agent.UserHome = func() string { return user }
	discoverable = []struct{ vendor, dir string }{{"keyedcli", ".keyed"}, {"sharecli", ".share"}, {"nocli", ".nocli"}}
	agent.Register(keyed{sharer{"keyedcli"}})
	agent.Register(sharer{"sharecli"})
	t.Cleanup(func() { agent.UserHome, discoverable = was, wasList })
	return home, user
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	d := filepath.Join(parts...)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDiscoverFindsTheFoldersThereAreAndTellsTheirLoginsFromTheVendorsCheck(t *testing.T) {
	_, user := discoverHome(t)
	if got := Discover(); len(got) != 0 {
		t.Fatalf("no folder, none found: %+v", got)
	}
	mkdir(t, user, ".keyed")
	share := mkdir(t, user, ".share")
	mkdir(t, user, ".nocli")
	os.WriteFile(filepath.Join(user, ".keyed", "auth.json"), nil, 0o600)
	got := Discover()
	if len(got) != 3 || got[0].Vendor != "keyedcli" || got[0].Source != "~/.keyed" || got[0].Adoptable || got[0].LoggedIn == nil || !*got[0].LoggedIn {
		t.Fatalf("found, logged in, not adoptable: %+v", got[0])
	}
	if got[1].Source != "~/.share" || !got[1].Adoptable || got[1].LoggedIn == nil || *got[1].LoggedIn {
		t.Fatalf("adoptable and logged out: %+v", got[1])
	}
	if got[2].Vendor != "nocli" || got[2].Adoptable || got[2].LoggedIn != nil {
		t.Fatalf("a vendor with no driver yet: found, not told: %+v", got[2])
	}
	os.WriteFile(filepath.Join(share, "unsure"), nil, 0o600)
	if got := Discover(); got[1].LoggedIn != nil {
		t.Fatalf("a check that cannot tell is null: %+v", got[1])
	}
}

func TestAnAdoptedAccountLinksOnlyTheCredentialFileAndKeepsNothingOnARefusal(t *testing.T) {
	home, user := discoverHome(t)
	a := Accounts{Home: home}
	share := mkdir(t, user, ".share")
	mkdir(t, user, ".keyed")
	mkdir(t, user, ".nocli")
	os.WriteFile(filepath.Join(user, ".keyed", "auth.json"), nil, 0o600)
	for _, c := range []struct {
		vendor, source string
		want           error
	}{
		{"keyedcli", "~/.keyed", ErrNotAdoptable},
		{"othercli", "~/.share", ErrNotAdoptable},
		{"nocli", "~/.elsewhere", ErrNoSource},
		{"nocli", "~/.nocli", ErrSourceLoggedOut},
		{"sharecli", "~/.elsewhere", ErrNoSource},
		{"sharecli", "~/.share", ErrSourceLoggedOut},
	} {
		if _, err := AdoptLink(c.vendor, c.source); !errors.Is(err, c.want) {
			t.Errorf("%s %s: %v, want %v", c.vendor, c.source, err, c.want)
		}
	}
	os.WriteFile(filepath.Join(share, "auth.json"), []byte("{}"), 0o600)
	os.WriteFile(filepath.Join(share, "hooks.json"), []byte("{}"), 0o600)
	link, err := AdoptLink("sharecli", "~/.share")
	if err != nil || link != (Link{Name: "auth.json", Target: filepath.Join(share, "auth.json")}) {
		t.Fatalf("%+v %v", link, err)
	}
	// A link that cannot be made takes the account back.
	if _, err := a.Add("cx", "sharecli", Link{Name: "no/auth.json", Target: link.Target}); err == nil {
		t.Fatal("a link into no folder was made")
	}
	if slices.Contains(paths.ListAccounts(home), "cx") {
		t.Fatal("a failed link kept the account")
	}
	cdir, err := a.Add("cx", "sharecli", link)
	if err != nil {
		t.Fatal(err)
	}
	if to, err := os.Readlink(filepath.Join(cdir, "auth.json")); err != nil || to != link.Target {
		t.Fatalf("the credential is linked: %q %v", to, err)
	}
	if entries, _ := os.ReadDir(cdir); len(entries) != 1 {
		t.Fatalf("only the credential: %v", entries)
	}
	if AccountVendor(home, "cx") != "sharecli" || !a.LoggedIn("cx") {
		t.Fatal("an account of the vendor, logged in as the found login")
	}
}

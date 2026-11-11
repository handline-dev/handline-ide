package team

import (
	"os"
	"path/filepath"
	"testing"
)

// loginHome is a home with accounts main and other, recorded as main@example.com and Other@Example.com.
func loginHome(t *testing.T) Accounts {
	t.Helper()
	t.Setenv("HANDLINE_TEST_ROOT", t.TempDir())
	t.Setenv("HANDLINE_"+LiveAccounts, "")
	a := Accounts{Home: t.TempDir()}
	for acct, email := range map[string]string{"main": "main@example.com", "other": "Other@Example.com"} {
		writeFile(t, a.identityFile(acct), `{"id": "x", "email": "`+email+`"}`)
	}
	return a
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func loggedInAs(t *testing.T, a Accounts, acct, oauth string) {
	t.Helper()
	dir, err := AccountConfigDir(a.Home, acct)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".claude.json"), `{"oauthAccount":`+oauth+`}`)
}

// After CredentialChanged, the stamp is read again: neither a cached one nor one a read begun before it brought back is used.
func TestCredentialChangedDropsTheCachedStampAndAnyReadUnderWay(t *testing.T) {
	a := loginHome(t)
	stamp, during := 1.0, func() {}
	old := readCredentialModified
	t.Cleanup(func() { readCredentialModified = old })
	readCredentialModified = func(string, string) *float64 { v := stamp; during(); return &v }

	if got := *a.credentialModified("main"); got != 1 {
		t.Fatalf("first read %v", got)
	}
	stamp = 2
	if got := *a.credentialModified("main"); got != 1 {
		t.Fatalf("a read within the TTL is the cached %v, got %v", 1.0, got)
	}
	a.CredentialChanged("main")
	if got := *a.credentialModified("main"); got != 2 {
		t.Fatalf("after CredentialChanged %v, want 2", got)
	}

	a.CredentialChanged("main")
	stamp, during = 3, func() { a.CredentialChanged("main") } // the login ends while this read runs
	_ = a.credentialModified("main")
	stamp, during = 4, func() {}
	if got := *a.credentialModified("main"); got != 4 {
		t.Fatalf("a read begun before CredentialChanged was cached: %v", got)
	}
}

func TestWrongLogin(t *testing.T) {
	for name, tc := range map[string]struct{ vendor, oauth, want string }{
		"the recorded one, in other case": {"", `{"emailAddress":"MAIN@example.com"}`, ""},
		"another account's, in other case": {"", `{"emailAddress":"other@example.com"}`,
			"logged in to a different Anthropic account than the one this account was set up with: the same Anthropic account as other"},
		"no email in the config":   {"", `{"userRateLimitTier":"x"}`, ""},
		"not a claude account":     {"opencode", `{"emailAddress":"new@example.com"}`, ""},
		"an unknown Anthropic one": {"", `{"emailAddress":"new@example.com"}`, "logged in to a different Anthropic account than the one this account was set up with"},
	} {
		t.Run(name, func(t *testing.T) {
			a := loginHome(t)
			if tc.vendor != "" {
				writeFile(t, filepath.Join(a.Home, "accounts", "main", "vendor"), tc.vendor)
			}
			loggedInAs(t, a, "main", tc.oauth)
			if got := a.WrongLogin("main"); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if got := a.RecordedEmail("main"); got != "main@example.com" {
				t.Fatalf("the record became %q", got)
			}
		})
	}
}

// With no email recorded, the login's is, beside what the record already holds.
func TestWrongLoginRecordsTheEmailAndKeepsTheRest(t *testing.T) {
	for record, want := range map[string]string{
		`{"id": 12345678901234567890}`: `{"email":"new@example.com","id":12345678901234567890}`,
		`null`:                         `{"email":"new@example.com"}`,
	} {
		a := loginHome(t)
		writeFile(t, a.identityFile("main"), record)
		loggedInAs(t, a, "main", `{"emailAddress":"new@example.com"}`)
		if got := a.WrongLogin("main"); got != "" {
			t.Fatalf("%s: got %q", record, got)
		}
		if b, _ := os.ReadFile(a.identityFile("main")); string(b) != want {
			t.Fatalf("%s: identity.json %s", record, b)
		}
	}
}

// Email is the recorded one, else the logged-in one, recorded then; a non-claude account has none from its config.
func TestEmail(t *testing.T) {
	a := loginHome(t)
	loggedInAs(t, a, "main", `{"emailAddress":"new@example.com"}`)
	if got := a.Email("main"); got != "main@example.com" {
		t.Fatalf("recorded: got %q", got)
	}
	writeFile(t, a.identityFile("main"), `{"id": "x"}`)
	if got := a.Email("main"); got != "new@example.com" {
		t.Fatalf("from the config: got %q", got)
	}
	if b, _ := os.ReadFile(a.identityFile("main")); string(b) != `{"email":"new@example.com","id":"x"}` {
		t.Fatalf("not recorded: identity.json %s", b)
	}
	writeFile(t, a.identityFile("main"), `{}`)
	writeFile(t, filepath.Join(a.Home, "accounts", "main", "vendor"), "opencode")
	if got := a.Email("main"); got != "" {
		t.Fatalf("not a claude account: got %q", got)
	}
}

// An empty user tier falls to the organization's.
func TestRateLimitTier(t *testing.T) {
	a := loginHome(t)
	dir, _ := AccountConfigDir(a.Home, "main")
	for oauth, want := range map[string]string{
		`{"userRateLimitTier":"u","organizationRateLimitTier":"o"}`: "u",
		`{"userRateLimitTier":"","organizationRateLimitTier":"o"}`:  "o",
		`{}`: "",
	} {
		loggedInAs(t, a, "main", oauth)
		if got := RateLimitTier(dir); got != want {
			t.Errorf("%s: %q, want %q", oauth, got, want)
		}
	}
}

package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
)

// fakeCodex puts a codex on PATH whose `login status` answers from $CODEX_HOME/says ("in", "out", anything else a failure), on stderr
// as codex does.
func fakeCodex(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
[ "$1 $2" = "login status" ] || exit 9
case "$(cat "$CODEX_HOME/says")" in
in) echo "Logged in using ChatGPT" >&2 ;;
out) echo "Not logged in" >&2; exit 1 ;;
*) echo "error: config.toml is broken" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialPresentAsksCodexLoginStatusUnderTheAccountsHome(t *testing.T) {
	fakeCodex(t)
	for _, tc := range []struct {
		name, says     string
		auth           bool
		present, known bool
	}{
		{"no auth.json, codex not asked", "in", false, false, true},
		{"logged in", "in", true, true, true},
		{"logged out", "out", true, false, true},
		{"a status that fails cannot tell", "broken", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "says"), tc.says)
			if tc.auth {
				write(t, filepath.Join(dir, "auth.json"), "{}")
			}
			if p, k := (Account{}).CredentialPresent(dir); p != tc.present || k != tc.known {
				t.Fatalf("got %v %v", p, k)
			}
		})
	}
}

func TestWithNoCodexInstalledALoginCannotBeTold(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	write(t, filepath.Join(dir, "auth.json"), "{}")
	if p, k := (Account{}).CredentialPresent(dir); p || k {
		t.Fatalf("got %v %v", p, k)
	}
}

func TestTheWebLoginIsADeviceLogin(t *testing.T) {
	argv, ok := Account{}.WebLoginArgv()
	if !ok || len(argv) != 3 || argv[2] != "--device-auth" {
		t.Fatalf("%v %v", argv, ok)
	}
	if d, ok := agent.Account(Account{}).(agent.DeviceCodeLogin); !ok || !d.DeviceCodeLogin() {
		t.Fatal("not a device login")
	}
}

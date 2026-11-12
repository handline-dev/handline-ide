package codex

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
)

func TestInstallWritesEveryWantedHookAndMissingSeesItWhole(t *testing.T) {
	h := Hooks{agent.Host{Home: t.TempDir()}}
	if !h.Missing() {
		t.Fatal("no file is missing")
	}
	if err := h.Install("/bin/hl"); err != nil {
		t.Fatal(err)
	}
	if h.Missing() {
		t.Fatal("installed")
	}
	b, _ := os.ReadFile(h.Target())
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if got := f.Hooks["PreToolUse"][0].Hooks[0]; got != (command{Type: "command", Command: "/bin/hl hook PreToolUse", Timeout: 25*60 + 60}) {
		t.Fatalf("%+v", got)
	}
	if got := f.Hooks["Stop"][0].Hooks[0]; got.Timeout != 10 || got.Command != "/bin/hl hook Stop" {
		t.Fatalf("%+v", got)
	}
	// An entry with another timeout, or an event left out, is missing.
	for _, edit := range []func(string) string{
		func(s string) string { return strings.Replace(s, "1560", "600", 1) },
		func(s string) string { return strings.Replace(s, `"Interrupt"`, `"Other"`, 1) },
	} {
		if err := os.WriteFile(h.Target(), []byte(edit(string(b))), 0o644); err != nil {
			t.Fatal(err)
		}
		if !h.Missing() {
			t.Fatalf("edited: %s", edit(string(b)))
		}
	}
}

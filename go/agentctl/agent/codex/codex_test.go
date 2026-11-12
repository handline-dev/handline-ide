package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/agentctl/agent"
)

// codex registers its account facts and its CLI.
func TestCodexRegistersItsAccountAndTool(t *testing.T) {
	if a, ok := agent.Lookup(Vendor); !ok || a.ConfigDirEnv() != "CODEX_HOME" {
		t.Fatalf("account facts: %v %v", a, ok)
	}
	found := false
	for _, tl := range agent.Tools() {
		found = found || tl == agent.Tool{Vendor: Vendor, Command: "codex", Fix: "npm install -g @openai/codex"}
	}
	if !found {
		t.Fatalf("tools: %v", agent.Tools())
	}
}

// The credential's time is auth.json's, none without it.
func TestCredentialModifiedIsAuthJSONs(t *testing.T) {
	dir := t.TempDir()
	if got := (Account{}).CredentialModified(dir); got != nil {
		t.Fatalf("no auth.json: %v", *got)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := (Account{}).CredentialModified(dir); got == nil || *got <= 0 {
		t.Fatalf("auth.json: %v", got)
	}
}

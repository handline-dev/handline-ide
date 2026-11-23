package runner

import (
	"os"
	"testing"
)

// The heartbeat says the app its shell wrote; the answer's app_update is kept for the shell, and an answer without one clears it.
func TestTheAppAroundAnEngineGoesUpAndItsUpdateComesBackForTheShell(t *testing.T) {
	home := t.TempDir()
	r := &Runner{cfg: Config{Home: home, Log: t.Logf}}
	if appOf(home) != nil {
		t.Fatal("an app with no app.json")
	}
	if err := os.WriteFile(AppFile(home), []byte(`{"version":"0.1.16","ready":"0.1.17"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if a := appOf(home); a == nil || *a != (App{Version: "0.1.16", Ready: "0.1.17"}) {
		t.Fatalf("app: %+v", a)
	}
	r.keepAppUpdate(map[string]any{"version": "0.1.17", "auto": true})
	if b, err := os.ReadFile(AppUpdateFile(home)); err != nil || string(b) != `{"version":"0.1.17","auto":true}` {
		t.Fatalf("kept: %s %v", b, err)
	}
	r.keepAppUpdate(nil)
	if _, err := os.Stat(AppUpdateFile(home)); !os.IsNotExist(err) {
		t.Fatalf("an answer with no app_update left the kept one: %v", err)
	}
	r.keepAppUpdate(nil) // none to clear: no error logged, nothing made
}

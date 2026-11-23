package runner

// The Mac app around a cluster engine (docs/story4-design.md slice 26): app.json from its shell goes up in the heartbeat, and the
// answer's app_update comes back to the shell in app-update.json.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// App is the Mac app around this runner, as its shell wrote it.
type App struct {
	Version string `json:"version"`
	Ready   string `json:"ready,omitempty"` // the newer version its update channel has downloaded and checked, "" for none
}

// AppUpdate is the version the master has the app install, and whether it was Auto (the shell then waits until no window is open)
// rather than its row's Update.
type AppUpdate struct {
	Version string `json:"version"`
	Auto    bool   `json:"auto"`
}

// AppFile is where the shell writes App; AppUpdateFile is where the runner keeps the master's AppUpdate for it.
func AppFile(home string) string       { return filepath.Join(home, "app.json") }
func AppUpdateFile(home string) string { return filepath.Join(home, "app-update.json") }

// appOf is the app around the runner of home, nil when there is none.
func appOf(home string) *App {
	var a App
	b, err := os.ReadFile(AppFile(home))
	if err != nil || json.Unmarshal(b, &a) != nil || a.Version == "" {
		return nil
	}
	return &a
}

// keepAppUpdate keeps the answer's app_update for the shell, or removes the one kept when the answer names none: the file says what the
// latest answer said, so a version installed meanwhile or an Update no longer asked is not acted on.
func (r *Runner) keepAppUpdate(raw any) {
	file := AppUpdateFile(r.cfg.Home)
	var u AppUpdate
	if b, err := json.Marshal(raw); raw == nil || err != nil || json.Unmarshal(b, &u) != nil || u.Version == "" {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			r.cfg.Log("runner: the app update was not cleared: %v", err)
		}
		return
	}
	b, _ := json.Marshal(u)
	if was, err := os.ReadFile(file); err == nil && bytes.Equal(was, b) {
		return
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		r.cfg.Log("runner: the app update was not kept: %v", err)
		return
	}
	if err := os.Rename(tmp, file); err != nil {
		r.cfg.Log("runner: the app update was not kept: %v", err)
	}
}

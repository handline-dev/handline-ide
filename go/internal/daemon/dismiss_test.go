package daemon

import (
	"fmt"
	"testing"

	"github.com/handline/handline/go/data/db"
)

// warning.dismiss, served by the daemon, hides the warning from requests.pending: its warnings are the reads' own.
func TestTheDaemonDismissesAWarningItHas(t *testing.T) {
	d, c := startDoors(t, false)
	if err := db.SetLoginNeed(d.db, "alpha", db.LoginNeed{Since: 100}); err != nil {
		t.Fatal(err)
	}
	has := func() bool {
		rows := call(t, c, "requests.pending", "{}")["result"].(map[string]any)["answer"].([]any)
		for _, r := range rows {
			if r := r.(map[string]any); r["source"] == "machine" && r["id"] == "login:alpha" {
				return true
			}
		}
		return false
	}
	if !has() {
		t.Fatal("no warning to dismiss")
	}
	if r := call(t, c, "warning.dismiss", `{"id":"login:alpha"}`); r["result"] == nil {
		t.Fatal(fmt.Sprint(r))
	}
	if has() {
		t.Fatal("still listed after its dismissal")
	}
}

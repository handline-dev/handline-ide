package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/team/teamtest"
)

func TestStatusReadsTheWindows(t *testing.T) {
	home := t.TempDir()
	write := func(acct, body string) {
		dir := teamtest.AccountDir(t, home, acct)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "status.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("new", `{"windows":[{"minutes":10080,"used_percent":41.2,"resets_at":9},{"minutes":300,"used_percent":12,"carried":true}]}`)
	write("bad", `[1]`)
	write("old-shape", `{"pct_5h":29,"reset_5h":5}`)
	n := ReadStatus(home, "new")
	if n.Short.Minutes != 300 || *n.Short.Pct != 12 || !n.Short.Carried || *n.Long.Pct != 42 || *n.Long.Reset != 9 {
		t.Fatalf("new shape: %+v", n)
	}
	if o := ReadStatus(home, "old-shape"); o.Short.Pct != nil || o.Long.Pct != nil {
		t.Fatalf("a record with no windows is no reading: %+v", o)
	}
	if ReadStatus(home, "bad").Error == "" || ReadStatus(home, "none").Short.Pct != nil {
		t.Fatal("a bad file is an error and a missing one no reading")
	}
}

func TestLabelsAndEnd(t *testing.T) {
	c := Config{"label_a": " ", "sub_end_a": "2026-10-22", "month_cycle_b": true}
	if c.LabelOf("a") != "a" {
		t.Fatal("a blank label is the name")
	}
	if got := c.EndDate("a"); got != "2026-10-22" {
		t.Fatalf("a typed end counts without the switch: %q", got)
	}
	if got := c.EndDate("b"); got != "" {
		t.Fatalf("no profile, no end: %q", got)
	}
	if got := (Config{"month_cycle_a": false, "sub_end_a": "2026-10-22"}).EndDate("a"); got != "" {
		t.Fatalf("off keeps the date but has no end: %q", got)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

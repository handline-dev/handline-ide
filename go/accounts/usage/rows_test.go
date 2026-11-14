package usage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/storetest"
)

func TestAccountSettingsComeFromTheRowAndAnUnreadableRowIsNotDefaults(t *testing.T) {
	home := t.TempDir()
	d, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("paused_main: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cfg := LoadConfig(home); cfg.Paused("main") {
		t.Fatalf("config.yaml says nothing of an account: %v", cfg)
	}
	if err := SetAccount(d, "main", map[string]any{"paused": true, "label": "Main"}); err != nil {
		t.Fatal(err)
	}
	if cfg, ok := LoadConfigOrNil(home); !ok || !cfg.Paused("main") {
		t.Fatalf("the row's pause: %v %v", cfg, ok)
	}
	if got := LoadConfig(home).LabelOf("main"); got != "Main" {
		t.Fatalf("the row's label: %q", got)
	}
	if _, err := d.Exec("DROP TABLE accounts"); err != nil {
		t.Fatal(err)
	}
	if cfg, ok := LoadConfigOrNil(home); ok {
		t.Fatalf("rows that cannot be read read as every account at its defaults: %v", cfg)
	}
	e := &Env{Home: home}
	if !LoadConfig(home).RowsUnreadable() || e.PausedWhy("other") == "" {
		t.Fatal("no account is picked on settings that could not be read")
	}
}

func TestSetAccountWritesNothingForABadColumnOrValue(t *testing.T) {
	d := storetest.Open(t)
	if err := SetAccount(d, "main", map[string]any{"owner": true}); err == nil || !strings.Contains(err.Error(), `no account setting "owner"`) {
		t.Fatalf("a column that is not a setting: %v", err)
	}
	for _, set := range []map[string]any{
		{"paused": true, "owner": "p2"},
		{"paused": true, "active_cap": 4},
		{"paused": true, "month_cycle": "yes"},
		{"paused": true, "label": 7},
	} {
		if err := SetAccount(d, "main", set); err == nil {
			t.Fatalf("%v written", set)
		}
	}
	if rows, err := ReadAccountRows(d); err != nil || len(rows) != 0 {
		t.Fatalf("a refused write left %v %v", rows, err)
	}
}

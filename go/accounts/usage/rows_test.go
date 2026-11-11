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
	os.WriteFile(filepath.Join(home, "config.yaml"), []byte("paused_main: true\nactive_cap: 3\n"), 0o644)
	if cfg := LoadConfig(home); cfg.Paused("main") || cfg.DefaultCap() != 3 {
		t.Fatalf("config.yaml says nothing of an account: %v", cfg)
	}
	if err := SetAccount(d, "main", map[string]any{"paused": true, "active_cap": float64(4)}); err != nil {
		t.Fatal(err)
	}
	if cfg, ok := LoadConfigOrNil(home); !ok || !cfg.Paused("main") {
		t.Fatalf("the row's pause: %v %v", cfg, ok)
	}
	if n, _ := LoadConfig(home).OwnCap("main"); n != 4 {
		t.Fatalf("a whole number from JSON is a cap: %d", n)
	}
	if _, err := d.Exec("DROP TABLE accounts"); err != nil {
		t.Fatal(err)
	}
	if cfg, ok := LoadConfigOrNil(home); ok {
		t.Fatalf("rows that cannot be read read as every account at its defaults: %v", cfg)
	}
}

func TestSetAccountWritesNothingForABadColumnOrValue(t *testing.T) {
	d := storetest.Open(t)
	if err := SetAccount(d, "main", map[string]any{"owner": true}); err == nil || !strings.Contains(err.Error(), `no account setting "owner"`) {
		t.Fatalf("a column that is not a setting: %v", err)
	}
	for _, set := range []map[string]any{
		{"paused": true, "owner": "p2"},
		{"paused": true, "active_cap": 4.5},
		{"paused": true, "active_cap": "4"},
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

package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func setConfig(t *testing.T, home, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The memory cap is off until set; set, a reading over it is acted on once, and the next reading is looked at again.
func TestTheMemoryCapIsReadFromTheSettingAndEachReadingOnce(t *testing.T) {
	home := t.TempDir()
	at, rss := int64(100), map[string]int64{"a1": 5 << 30, "a2": 4 << 30}
	m := &tickMemory{home: home, read: func() (int64, map[string]int64) { return at, rss }}
	if _, _, over := m.Over(0); over {
		t.Fatal("off by default")
	}
	setConfig(t, home, "agents_memory_max_gb: 8\n")
	used, limit, over := m.Over(0)
	if !over || used != 9<<30 || limit != 8<<30 {
		t.Fatalf("over: %d %d %v", used, limit, over)
	}
	m.Over(100)
	m.Acted()
	at = 115
	if _, _, over := m.Over(120); over {
		t.Fatal("a reading taken within a minute of the close is not acted on: the closed session may still count")
	}
	at = 165
	if _, _, over := m.Over(170); !over {
		t.Fatal("a reading a minute after the close is looked at")
	}
	rss["a2"] = 2 << 30
	if _, _, over := m.Over(0); over {
		t.Fatal("within the cap")
	}
}

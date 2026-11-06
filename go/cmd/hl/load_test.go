package main

import (
	"strings"
	"testing"
)

func TestLoadTextShowsTheMachineEachAgentAndTheRestApart(t *testing.T) {
	got := obj{"every_s": 5.0, "sampling": obj{"mean_ms": 8.5},
		"machine": obj{"cores": 10.0, "load_1m": 3.2, "load_per_core": 0.32, "ram_free_bytes": float64(12 << 30), "ram_total_bytes": float64(32 << 30), "disk_free_bytes": float64(120 << 30)},
		"agents": []any{obj{"agent": "a.b", "cpu_pct": 180.0, "rss_bytes": float64(600 << 20), "processes": 2.0, "top": []any{
			obj{"command": "pnpm test", "cpu_pct": 180.0, "rss_bytes": float64(200 << 20), "running": true},
			obj{"command": "claude", "cpu_pct": 5.0, "rss_bytes": float64(400 << 20), "running": false}}}},
		"unowned": obj{"cpu_pct": 12.0, "rss_bytes": float64(300 << 20), "processes": 1.0, "top": []any{obj{"command": "WindowServer", "cpu_pct": 12.0, "rss_bytes": float64(300 << 20), "running": true}}},
	}
	text := loadText(got)
	for _, want := range []string{"10 cores", "12.0 GB free of 32.0 GB", "a.b", "180%", "600 MB", "pnpm test 180%/200 MB", "claude 5%/400 MB (ended)", "(no agent)", "WindowServer", "every 5s; a sample takes 8.5 ms"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

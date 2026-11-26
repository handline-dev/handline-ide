package main

import (
	"testing"
)

// hl prefs prints the rules of the person the caller works for as one list; with an agent named, that agent's lessons too, each under
// its heading.
func TestPrefsPrintsTheRulesAndAnAgentsLessons(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"lessons.list": `{"agent": "p1", "bytes": 1, "cap": 3072, "items": [` +
		`{"id": 1, "agent": "p1", "text": "no hints under titles", "count": 3},` +
		`{"id": 2, "agent": "p1", "text": "tests in the same commit"}]}`})
	if out, _, code := r.run(asAgent, "prefs"); code != 0 || out != "- no hints under titles\n- tests in the same commit\n" {
		t.Fatalf("rules: %d %q", code, out)
	}
	if p := calls()[0]["params"].(map[string]any); p["agent"] != "human" {
		t.Fatalf("asked %v", p)
	}
	out, _, code := r.run(asAgent, "prefs", "ui")
	want := "# The founder's preferences\n- no hints under titles\n- tests in the same commit\n# What ui learned\n- no hints under titles\n- tests in the same commit\n"
	if code != 0 || out != want {
		t.Fatalf("with an agent: %d %q", code, out)
	}
	if p := calls()[2]["params"].(map[string]any); p["agent"] != "ui" {
		t.Fatalf("asked %v", p)
	}
	if _, _, code := r.run(asAgent, "prefs", "--area", "screen"); code == 0 {
		t.Fatal("--area is gone")
	}
}

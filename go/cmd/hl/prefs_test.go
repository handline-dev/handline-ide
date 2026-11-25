package main

import (
	"testing"
)

// hl prefs prints the rules of the person the caller works for: one area with --area, both under their headings without; a rule with no
// area is a work rule.
func TestPrefsPrintsTheRulesOfAnArea(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(map[string]string{"lessons.list": `{"agent": "p1", "bytes": 1, "cap": 3072, "items": [` +
		`{"id": 1, "agent": "p1", "text": "no hints under titles", "area": "screen", "count": 3},` +
		`{"id": 2, "agent": "p1", "text": "tests in the same commit", "area": "work"},` +
		`{"id": 3, "agent": "p1", "text": "write in Korean", "area": "work"}]}`})
	if out, _, code := r.run(asAgent, "prefs", "--area", "screen"); code != 0 || out != "- no hints under titles\n" {
		t.Fatalf("screen: %d %q", code, out)
	}
	want := "# Work\n- tests in the same commit\n- write in Korean\n# Screen\n- no hints under titles\n"
	if out, _, code := r.run(asAgent, "prefs"); code != 0 || out != want {
		t.Fatalf("both: %d %q", code, out)
	}
	if p := calls()[0]["params"].(map[string]any); p["agent"] != "human" {
		t.Fatalf("asked %v", p)
	}
	if _, _, code := r.run(asAgent, "prefs", "--area", "design"); code == 0 {
		t.Fatal("an area that is not one")
	}
}

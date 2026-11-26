package main

import (
	"strings"
	"testing"
)

// A task's page for an agent says how the gates it waited on opened, newest first: an agent cannot read the pull request itself, so the
// failed checks and a request for changes reach it here, on the task, and not only in the notice that was typed when the gate opened.
func TestTaskShowSaysHowTheGatesItWaitedOnOpened(t *testing.T) {
	task := obj{"id": float64(7), "title": "ship", "to": "w", "state": "queued",
		"gates": []any{
			obj{"task": float64(9), "label": "PR #412 to land", "result": "failed", "evidence": "ci failed: build, lint", "at": "2026-11-16T12:00:00Z"},
			obj{"task": float64(8), "label": "PR #412 to land", "result": "timed_out", "evidence": "timed out", "at": "2026-11-16T10:00:00Z"},
		}}
	got := strings.Join(detailLines(task, 0, ""), "\n")
	for _, want := range []string{"\nwaited on (newest first)\n  T9  PR #412 to land: failed - ci failed: build, lint  ", "\n  T8  PR #412 to land: timed out - timed out  "} {
		if !strings.Contains(got, want) {
			t.Fatalf("no %q in\n%s", want, got)
		}
	}
	if got := strings.Join(detailLines(obj{"id": float64(7), "title": "ship", "gates": []any{}}, 0, ""), "\n"); strings.Contains(got, "waited on") {
		t.Fatalf("a task that waited on no gate says nothing of it:\n%s", got)
	}
}

package turns

import (
	"testing"
	"time"
)

// A runner's tracker's view, held by the master's (SetView), reads there as the runner's does; Forget makes it unknown again.
func TestAViewHeldElsewhereReadsAsItsOwn(t *testing.T) {
	runner, master := &Tracker{}, &Tracker{Agent: func(string) string { return "bob" }}
	runner.Hook("bob", "UserPromptSubmit", map[string]any{}, 1)
	master.SetView("bob", runner.ViewOf("bob"))
	if master.State("bob") != Working {
		t.Fatalf("master: %v", master.State("bob"))
	}
	done := make(chan bool)
	go func() { idle, _ := master.WaitIdle("%1", 5*time.Second); done <- idle }()
	runner.Hook("bob", "Stop", map[string]any{"background_tasks": []any{}}, 2)
	master.SetView("bob", runner.ViewOf("bob"))
	if !<-done || !master.Settled("%1") {
		t.Fatal("an idle wait on the master did not see the runner's Stop")
	}
	if bg, known := master.Background("%1"); !known || bg != "" {
		t.Fatalf("background: %q %v", bg, known)
	}
	master.Forget("bob")
	if master.State("bob") != Unknown {
		t.Fatal("forgotten, it is still known")
	}
}

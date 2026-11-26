package lifecycle

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
)

// A seat starts with what the agent it is of knows: its role and its lessons in the prompt, and its memory folder as the
// session's own, to read and write, which the hide list leaves open; and its record (handoff and the daemon's), under its own heading.
func TestATempKnowsWhatItsAgentKnows(t *testing.T) {
	r := newLaunchRig(t)
	teamtest.Set(t, r.l.Home, "bob", "vendor", "claude")
	ann := teamtest.Key(t, r.l.Home, "ann")
	if _, err := db.SetInstruction(r.l.Q, ann, db.PartRole, "# ann\n\nBuilds the settings screens.", "p1", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddLesson(r.l.Q, ann, "show the screen in both themes", 0, ann); err != nil {
		t.Fatal(err)
	}
	if err := db.SetHandoff(r.l.Q, ann, "ann was half way through the dark theme"); err != nil {
		t.Fatal(err)
	}
	temp, err := team.NewSeat(r.l.Q, ann, 0)
	if err != nil {
		t.Fatal(err)
	}
	text, err := r.l.ContextFor(temp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Builds the settings screens.") || !strings.Contains(text, "- show the screen in both themes") {
		t.Fatalf("the temp's prompt lacks ann's role or lessons:\n%s", text)
	}
	if _, record, ok := strings.Cut(text, "# Your agent's record\nYou are a temp of "); !ok || !strings.Contains(record, "half way through") {
		t.Fatalf("the temp's prompt lacks ann's record:\n%s", text)
	}
	mine := filepath.Join(paths.AgentDir(r.l.Home, ann), "memory")
	hidden, err := r.l.HiddenMemories(temp)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(hidden, mine) || len(hidden) != 1 {
		t.Fatalf("hidden from the temp: %v", hidden)
	}
	if dir, err := r.l.MemoryDir(temp); err != nil || dir != mine {
		t.Fatalf("the temp keeps its memory in %s %v, not %s", dir, err, mine)
	}
}

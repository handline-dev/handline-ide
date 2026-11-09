package cmdruns

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/storetest"
)

// What is told is bounded: a long command is cut in the line, a long output keeps its end, and a window with nothing on it says so.
func TestToldCutsALongCommandAndKeepsTheEndOfALongOutput(t *testing.T) {
	q, err := storetest.OpenAt(t, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	exit := 0
	r := Run{Block: "r9:0", Command: strings.Repeat("é", maxCommand+5), Started: 10, Ended: 12, State: "done", Exit: &exit}
	got := Told(q, r, []string{strings.Repeat("x", maxTail), "last row"})
	line, out, _ := strings.Cut(got, "\nLast rows of its window:\n")
	if !strings.HasSuffix(line, "Command: "+strings.Repeat("é", maxCommand)+"…") || !strings.HasPrefix(line, "The run of your reply's code block 1 ended with exit 0 after 2s.") {
		t.Fatalf("line %q", line)
	}
	if len([]rune(out)) != maxTail+1 || !strings.HasPrefix(out, "…x") || !strings.HasSuffix(out, "\nlast row") {
		t.Fatalf("output of %d runes: %q…", len([]rune(out)), out[:20])
	}
	if got := Told(q, r, []string{"", "  "}); !strings.HasSuffix(got, "\nIts window showed nothing.") {
		t.Fatalf("%q", got)
	}
}

// An inline `! <command>` span is named as one, not as a code block numbered past a thousand.
func TestABangSpanRunIsNamedAsOne(t *testing.T) {
	st := 0
	r := Run{Block: Block{Message: 7, Reply: true, N: bangFrom + 1}.String(), Command: "gh auth login", Started: 1, Ended: 2, Exit: &st}
	if line := Told(nil, r, nil); !strings.HasPrefix(line, "The run of your reply's ! command 2 ended") {
		t.Fatal(line)
	}
}

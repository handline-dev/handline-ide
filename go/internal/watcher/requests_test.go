package watcher

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/reads"
)

func TestARequestThatAppearedOrChangedIsToldAsItsRow(t *testing.T) {
	d, l := rig(t)
	w := &Requests{Reads: &reads.Env{DB: d}}
	told := func() []string {
		if err := w.Scan(d, l.Append); err != nil {
			t.Fatal(err)
		}
		got, err := l.Since(0, []string{"requests"}, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range got {
			out = append(out, e.Type+" "+string(e.Data))
		}
		return out
	}
	now := time.Now().Unix()
	exec(t, d, fmt.Sprintf("INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'hello', 'lead', 'human', %d)", now-10))
	if n := told(); len(n) != 0 {
		t.Fatalf("a first look told %v", n)
	}
	exec(t, d, fmt.Sprintf("UPDATE tasks SET done_evidence = 'ok', done_at = %d WHERE id = 1", now-5))
	got := told()
	if len(got) != 1 || !strings.HasPrefix(got[0], "request.changed ") || !strings.Contains(got[0], `"status":"done"`) {
		t.Fatalf("%v", got)
	}
	if n := told(); len(n) != 1 {
		t.Fatalf("no change told again: %v", n)
	}
}

// A founder seeing a result moves only a cursor, and the row is told again with it.
func TestRequestsWatchesTheCursorsTheResultMarkMoves(t *testing.T) {
	if !slices.Contains((&Requests{}).Tables(), "cursors") {
		t.Fatal("a mark on a result would not reach the requests topic")
	}
}

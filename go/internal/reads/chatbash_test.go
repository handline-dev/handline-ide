package reads

import (
	"encoding/json"
	"fmt"
	"github.com/handline/handline/go/data/team/teamtest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/transcript"
)

const bashSession = "11111111-2222-3333-4444-555555555555"

// bashRig is a session transcript under an account in the env's home; write appends the lines a ! line leaves.
type bashRig struct {
	t    *testing.T
	e    *Env
	path string
}

func newBashRig(t *testing.T) *bashRig {
	t.Helper()
	e := testEnv(t)
	e.Home = t.TempDir()
	dir := filepath.Join(teamtest.AccountDir(t, e.Home, "alpha"), ".claude", "projects", "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &bashRig{t, e, filepath.Join(dir, bashSession+".jsonl")}
}

func (r *bashRig) line(uuid, parent string, at int64, content string) {
	r.t.Helper()
	b, _ := json.Marshal(map[string]any{"type": "user", "uuid": uuid, "parentUuid": parent, "timestamp": time.Unix(at, 0).UTC().Format(time.RFC3339Nano),
		"message": map[string]any{"role": "user", "content": content}})
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		r.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		r.t.Fatal(err)
	}
}

func (r *bashRig) ran(uuid string, at int64, command, stdout string) {
	r.t.Helper()
	r.line(uuid, "", at, "<bash-input>"+command+"</bash-input>")
	r.line(uuid+"-out", uuid, at, "<bash-stdout>"+stdout+"</bash-stdout><bash-stderr></bash-stderr>")
}

// sent is a founder's ! line; landed 0 never landed.
func (r *bashRig) sent(id, typed, landed int64) {
	r.t.Helper()
	l := "NULL"
	if landed != 0 {
		l = fmt.Sprint(landed)
	}
	run(r.t, r.e, fmt.Sprintf("INSERT INTO requests(id, ts, requester, target, body, state, kind, ref, typed_at, typed_session, landed_at, edge_ok) "+
		"VALUES(%d, %d, 'human', 'bob', '!x', 'landed', 'say', 'r%d', %d, '%s', %s, 1)", id, typed, id, typed, bashSession, l))
}

func (r *bashRig) items() []map[string]any {
	r.t.Helper()
	items, _, err := r.e.BashItems("bob", 0, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	return items
}

// A line takes the command recorded between its typing and landing; each command goes to one line; a command outside any is its own row.
func TestABashItemNamesTheMessageThatRanIt(t *testing.T) {
	r := newBashRig(t)
	r.ran("term", 100, "typed at the terminal", "t")
	r.sent(1, 200, 201)
	r.ran("a", 200, "first", "one")
	r.sent(2, 300, 310) // queued behind the turn with the next: both land late, and they run in order
	r.sent(3, 301, 310)
	r.ran("b", 305, "second", "two")
	r.ran("c", 306, "third", "three")
	r.sent(4, 400, 0) // never landed
	r.ran("d", 400, "unconfirmed", "four")
	got := r.items()
	want := []struct {
		command string
		request any
	}{{"typed at the terminal", nil}, {"first", int64(1)}, {"second", int64(2)}, {"third", int64(3)}, {"unconfirmed", nil}}
	if len(got) != len(want) {
		t.Fatalf("%d items: %v", len(got), got)
	}
	for i, w := range want {
		if got[i]["command"] != w.command || got[i]["request"] != w.request || got[i]["kind"] != "bash" {
			t.Fatalf("item %d, want %v: %v", i, w, got[i])
		}
	}
}

// Its session comes from the typing, else from the start the agent had by then.
func TestABashLineWithNoTypedSessionIsFoundByTheSessionItWasIn(t *testing.T) {
	r := newBashRig(t)
	run(t, r.e, "INSERT INTO session_starts(id, agent, ts, session_id, cause, resumed, text, prompt) VALUES(1, 'bob', 50, '"+bashSession+"', 'start', 0, '', ''), "+
		"(2, 'bob', 900, '99999999-2222-3333-4444-555555555555', 'start', 0, '', '')")
	run(t, r.e, "INSERT INTO requests(id, ts, requester, target, body, state, kind, ref, typed_at, landed_at, edge_ok) VALUES(1, 200, 'human', 'bob', '! x', 'landed', 'say', 'r1', 200, 201, 1)")
	r.ran("a", 200, "x", "out")
	if got := r.items(); len(got) != 1 || got[0]["stdout"] != "out" || got[0]["request"] != int64(1) {
		t.Fatalf("%v", got)
	}
}

// A span with no founder's ! line has no items, and a command is only in the span it ran in.
func TestBashItemsAreOnlyOfASpanHoldingABashLine(t *testing.T) {
	r := newBashRig(t)
	r.sent(1, 200, 201)
	r.ran("a", 200, "old", "x")
	for _, c := range []struct {
		from, to int64
		want     int
	}{{300, 0, 0}, {0, 199, 0}, {0, 0, 1}, {200, 201, 1}} {
		items, _, err := r.e.BashItems("bob", c.from, c.to)
		if err != nil || len(items) != c.want {
			t.Fatalf("%d to %d: %v %v", c.from, c.to, items, err)
		}
	}
	if items, waiting, err := r.e.BashItems("carol", 0, 0); err != nil || waiting != nil || items != nil {
		t.Fatalf("%v %v %v", items, waiting, err)
	}
}

func TestABashLineWaitingForItsOutputIsOpen(t *testing.T) {
	r := newBashRig(t)
	r.sent(1, 200, 201)
	r.line("a", "", 200, "<bash-input>sleep 100</bash-input>")
	items, waiting, err := r.e.BashItems("bob", 0, 0)
	if err != nil || len(waiting) != 1 || waiting[0] != 1 || len(items) != 1 || items[0]["done"] != false {
		t.Fatalf("%v %v %v", items, waiting, err)
	}
	if _, has := items[0]["stdout"]; has {
		t.Fatalf("output before it is done: %v", items[0])
	}
	r.line("b", "a", 201, "<bash-stdout>woke</bash-stdout><bash-stderr>e</bash-stderr>")
	items, waiting, err = r.e.BashItems("bob", 0, 0)
	if err != nil || waiting != nil || items[0]["done"] != true || items[0]["stdout"] != "woke" || items[0]["stderr"] != "e" {
		t.Fatalf("%v %v %v", items, waiting, err)
	}
}

// A terminal command waiting for output does not keep anyone polling: nothing of the founder's waits on it.
func TestACommandNoMessageSentIsNotOpen(t *testing.T) {
	r := newBashRig(t)
	r.sent(1, 200, 201)
	r.ran("a", 200, "mine", "x")
	r.line("t", "", 300, "<bash-input>sleep 100</bash-input>")
	if _, waiting, err := r.e.BashItems("bob", 0, 0); err != nil || waiting != nil {
		t.Fatalf("%v %v", waiting, err)
	}
}

func TestLongBashOutputSaysWhatWasDropped(t *testing.T) {
	long := ""
	for len(long) <= transcript.ResultMax {
		long += "a line\n"
	}
	r := newBashRig(t)
	r.sent(1, 200, 201)
	r.ran("a", 200, "yes", long)
	if got := r.items(); got[0]["dropped_lines"] == nil || len(got[0]["stdout"].(string)) > transcript.ResultMax {
		t.Fatalf("%v", got[0]["dropped_lines"])
	}
}

// The page: a bash item sits in its span, in id order, and before holds for it as for any item.
func TestAChatPageCarriesTheBashItemsInItsSpan(t *testing.T) {
	r := newBashRig(t)
	r.sent(1, 200, 201)
	r.ran("a", 200, "in", "x")
	id := func(at int64) map[string]any { return map[string]any{"id": at * 1_000_000} }
	page := []map[string]any{id(150), id(250)}
	got, err := r.e.withBashItems("bob", page, nil, nil, false)
	if err != nil || len(got) != 3 || got[1]["command"] != "in" {
		t.Fatalf("%v %v", got, err)
	}
	before := int64(190 * 1_000_000)
	if got, err = r.e.withBashItems("bob", []map[string]any{id(150)}, &before, nil, false); err != nil || len(got) != 1 {
		t.Fatalf("before it: %v %v", got, err)
	}
	if got, err = r.e.withBashItems("bob", []map[string]any{id(250)}, nil, nil, true); err != nil || len(got) != 1 {
		t.Fatalf("older than a page with more: %v %v", got, err)
	}
	since := int64(250 * 1_000_000)
	if got, err = r.e.withBashItems("bob", nil, nil, &since, false); err != nil || len(got) != 0 {
		t.Fatalf("since after it: %v %v", got, err)
	}
	since = 199 * 1_000_000
	if got, err = r.e.withBashItems("bob", nil, nil, &since, false); err != nil || len(got) != 1 {
		t.Fatalf("since before it: %v %v", got, err)
	}
}

// A command run after a line landed is not that line's, though nothing else claims it.
func TestACommandAfterALineLandedIsNotItsPair(t *testing.T) {
	r := newBashRig(t)
	r.sent(1, 200, 201)
	r.ran("later", 250, "typed at the terminal", "x")
	if got := r.items(); len(got) != 1 || got[0]["request"] != nil {
		t.Fatalf("%v", got)
	}
}

// A span's ! line opens the transcript, which holds commands of other spans too: only the span's are items.
func TestOnlyTheCommandsInTheSpanAreItems(t *testing.T) {
	r := newBashRig(t)
	r.ran("old", 100, "before", "x")
	r.sent(1, 200, 201)
	r.ran("a", 200, "in", "x")
	r.ran("new", 300, "after", "x")
	items, _, err := r.e.BashItems("bob", 150, 250)
	if err != nil || len(items) != 1 || items[0]["command"] != "in" {
		t.Fatalf("%v %v", items, err)
	}
}

// A ! line outside the span does not bring it in: its command waiting says nothing of this span.
func TestALineOutsideTheSpanIsNotReadFor(t *testing.T) {
	r := newBashRig(t)
	r.sent(1, 200, 201)
	r.line("a", "", 200, "<bash-input>sleep 100</bash-input>")
	for _, c := range [][2]int64{{300, 0}, {0, 199}} {
		if items, waiting, err := r.e.BashItems("bob", c[0], c[1]); err != nil || waiting != nil || len(items) != 0 {
			t.Fatalf("%v: %v %v %v", c, items, waiting, err)
		}
	}
	if _, waiting, err := r.e.BashItems("bob", 0, 0); err != nil || waiting == nil {
		t.Fatalf("in the span: %v %v", waiting, err)
	}
}

// before leaves out the item it names, as for every source.
func TestBeforeABashItemLeavesItOut(t *testing.T) {
	r := newBashRig(t)
	r.sent(1, 200, 201)
	r.ran("a", 200, "in", "x")
	before := r.items()[0]["id"].(int64)
	if got, err := r.e.withBashItems("bob", nil, &before, nil, false); err != nil || len(got) != 0 {
		t.Fatalf("%v %v", got, err)
	}
}

// A line just outside the span keeps its own command: the next line, whose typing reaches back over it, does not take it.
func TestALineAtTheSpansEdgeKeepsItsCommand(t *testing.T) {
	r := newBashRig(t)
	r.sent(1, 105, 105)
	r.ran("a", 105, "first", "x")
	r.sent(2, 101, 106)
	r.ran("b", 106, "second", "y")
	items, _, err := r.e.BashItems("bob", 106, 0)
	if err != nil || len(items) != 1 || items[0]["command"] != "second" || items[0]["request"] != int64(2) {
		t.Fatalf("%v %v", items, err)
	}
}

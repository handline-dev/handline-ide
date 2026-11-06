package filerw

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// applied is a with hunks made over it applied.
func applied(a string, hunks []hunk) string {
	_, set := changeSetJSON(hunks, len(encode(a)))
	out, err := set.apply(encode(a))
	if err != nil {
		return "error: " + err.Error()
	}
	return decode(out)
}

func TestDiffLinesTurnsOneTextIntoTheOtherLineByLine(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		hunks int
	}{
		{"a\nb\nc\n", "a\nb\nc\n", 0},
		{"a\nb\nc\n", "a\nB\nc\n", 1},
		{"a\nb\nc\nd\ne\n", "A\nb\nc\nd\nE\n", 2},
		{"a\nb\n", "a\nb\nc", 1},
		{"", "x\n", 1},
		{"x\n", "", 1},
		{"한\n😀\n", "한\n😀!\n", 1},
	} {
		h := diffLines(encode(c.a), encode(c.b))
		if len(h) != c.hunks || applied(c.a, h) != c.b {
			t.Errorf("%q -> %q: %d hunks %+v, gives %q", c.a, c.b, len(h), h, applied(c.a, h))
		}
	}
	var a, b strings.Builder
	for i := 0; i < maxEdits; i++ { // more edits than maxEdits: one hunk from the first to the last line that differs
		fmt.Fprintf(&a, "a%d\n", i)
		fmt.Fprintf(&b, "b%d\n", i)
	}
	a.WriteString("tail\n")
	b.WriteString("tail\n")
	if h := diffLines(encode("head\n"+a.String()), encode("head\n"+b.String())); len(h) != 1 || h[0].from != 5 ||
		h[0].to != len(encode("head\n"+a.String()))-5 || applied("head\n"+a.String(), h) != "head\n"+b.String() {
		t.Fatalf("past maxEdits: %d hunks, not the lines that differ only", len(h))
	}
}

func TestMapPosCarriesAPositionOverAChange(t *testing.T) {
	set, _ := parseChangeSet(cs(2, []any{0, "XY"}, 1, []any{2, "Z"}, 1)) // "abcdef" -> "abXYcZf"
	for _, c := range []struct{ pos, assoc, want int }{
		{0, 1, 0}, {1, 1, 1},
		{2, -1, 2}, {2, 1, 4}, // at the insertion: before it, or after it
		{3, 1, 5},             // the start of the replaced "de"
		{4, -1, 5}, {4, 1, 6}, // inside it: its start, or its end
		{5, 1, 6}, {6, 1, 7},
	} {
		if got := mapPos(set, c.pos, c.assoc); got != c.want {
			t.Errorf("mapPos(%d, %d) = %d, want %d", c.pos, c.assoc, got, c.want)
		}
	}
}

func TestRebaseKeepsAHunkOffWhatWasInsertedAtItsEdges(t *testing.T) {
	base := "a\nbb\nc\n"
	agent := diffLines(encode(base), encode("a\nB\nc\n")) // replaces line bb
	founder, _ := parseChangeSet(cs(2, []any{0, "x"}, 5)) // types x at the start of line bb: kept, it is at the hunk's edge
	_, set := changeSetJSON(rebase(agent, []changeSet{founder}), 8)
	if out, err := set.apply(encode("a\nxbb\nc\n")); err != nil || decode(out) != "a\nxB\nc\n" {
		t.Fatalf("%q %v", decode(out), err)
	}
	founder, _ = parseChangeSet(cs(3, []any{0, "x"}, 4)) // types x inside line bb: the agent's line wins
	_, set = changeSetJSON(rebase(agent, []changeSet{founder}), 8)
	if out, err := set.apply(encode("a\nbxb\nc\n")); err != nil || decode(out) != "a\nB\nc\n" {
		t.Fatalf("%q %v", decode(out), err)
	}
	founder, _ = parseChangeSet(cs(5, []any{0, "y\n"}, 2)) // a new line after bb
	_, set = changeSetJSON(rebase(agent, []changeSet{founder}), 9)
	if out, err := set.apply(encode("a\nbb\ny\nc\n")); err != nil || decode(out) != "a\nB\ny\nc\n" {
		t.Fatalf("%q %v", decode(out), err)
	}
	empty, _ := parseChangeSet(cs(6, []any{0, "z"}))
	ins := []hunk{{from: 2, to: 2, ins: encode("n\n")}} // an agent inserts a line where the founder inserted text
	_, set = changeSetJSON(rebase(ins, []changeSet{empty}), 7)
	if out, _ := set.apply(encode("a\nb\nc\nz")); decode(out) != "a\nn\nb\nc\nz" {
		t.Fatalf("%q", decode(out))
	}
}

func TestAHunkThatInsertsWhereTheFounderInsertedGoesAfterIt(t *testing.T) {
	founder, _ := parseChangeSet(cs(2, []any{0, "x"}, 4))                             // "a\nb\nc\n" -> "a\nxb\nc\n"
	got := rebase([]hunk{{from: 2, to: 2, ins: encode("n\n")}}, []changeSet{founder}) // an agent's new line before b
	if got[0].from != 3 || got[0].to != 3 {
		t.Fatalf("%+v", got[0])
	}
}

func TestDiffLinesTurnsAnyTextIntoAnyOther(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	text := func() string {
		var b strings.Builder
		for i := rng.Intn(12); i > 0; i-- {
			b.WriteString([]string{"a\n", "b\n", "c\n", "\n", "d"}[rng.Intn(5)])
		}
		return b.String()
	}
	for i := 0; i < 2000; i++ {
		a, b := text(), text()
		if got := applied(a, diffLines(encode(a), encode(b))); got != b {
			t.Fatalf("%q -> %q gives %q", a, b, got)
		}
	}
}

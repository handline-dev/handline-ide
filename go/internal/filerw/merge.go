package filerw

import (
	"strings"
	"unicode/utf16"
)

// hunk replaces [from, to) of a text with ins; positions in UTF-16 units.
type hunk struct {
	from, to int
	ins      []uint16
}

// maxEdits bounds the line diff's work: past that many lines put in or taken out, the change is taken as one hunk from the first to the
// last line that differs. The trace it keeps grows as maxEdits squared (about 8 MB at 1000).
const maxEdits = 1000

// splitLines cuts t after each \n; the last line may have none.
func splitLines(t []uint16) [][]uint16 {
	var out [][]uint16
	start := 0
	for i, u := range t {
		if u == '\n' {
			out = append(out, t[start:i+1])
			start = i + 1
		}
	}
	if start < len(t) {
		out = append(out, t[start:])
	}
	return out
}

func sameLine(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// diffLines is how to turn a into b, as hunks of whole lines over a: a Myers diff, after the lines both start and end with are put aside.
func diffLines(a, b []uint16) []hunk {
	la, lb := splitLines(a), splitLines(b)
	pre := 0
	for pre < len(la) && pre < len(lb) && sameLine(la[pre], lb[pre]) {
		pre++
	}
	suf := 0
	for suf < len(la)-pre && suf < len(lb)-pre && sameLine(la[len(la)-1-suf], lb[len(lb)-1-suf]) {
		suf++
	}
	ma, mb := la[pre:len(la)-suf], lb[pre:len(lb)-suf]
	offA := []int{0} // offA[i]: where line i of la starts
	for _, l := range la {
		offA = append(offA, offA[len(offA)-1]+len(l))
	}
	join := func(ls [][]uint16) []uint16 {
		var out []uint16
		for _, l := range ls {
			out = append(out, l...)
		}
		return out
	}
	if len(ma) == 0 && len(mb) == 0 {
		return nil
	}
	edits, ok := myers(ma, mb)
	if !ok {
		return []hunk{{offA[pre], offA[pre+len(ma)], join(mb)}}
	}
	var out []hunk
	for _, e := range edits { // e: lines [e.a0, e.a1) of ma become lines [e.b0, e.b1) of mb
		out = append(out, hunk{offA[pre+e.a0], offA[pre+e.a1], join(mb[e.b0:e.b1])})
	}
	return out
}

type edit struct{ a0, a1, b0, b1 int }

// myers is the shortest edit from a to b as runs of replaced lines; false when it takes more than maxEdits.
func myers(a, b [][]uint16) ([]edit, bool) {
	n, m := len(a), len(b)
	limit := min(n+m, maxEdits)
	off := limit + 1
	v := make([]int, 2*limit+3)
	var trace [][]int // trace[d]: v[-d..d] as round d found it
	for d := 0; d <= limit; d++ {
		trace = append(trace, append([]int(nil), v[off-d:off+d+1]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && sameLine(a[x], b[y]) {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				return backtrack(trace, n, m, d), true
			}
		}
	}
	return nil, false
}

// backtrack walks the trace from (n, m) back to (0, 0) and gathers the lines taken out of a and put in from b into runs.
func backtrack(trace [][]int, n, m, d int) []edit {
	x, y := n, m
	delA, insB := make([]bool, n), make([]bool, m)
	for ; d > 0; d-- {
		v := func(k int) int { return trace[d][k+d] }
		k := x - y
		pk := k - 1
		if k == -d || (k != d && v(k-1) < v(k+1)) {
			pk = k + 1
		}
		px := v(pk)
		py := px - pk
		for x > px && y > py { // the diagonal: kept lines
			x--
			y--
		}
		if x > px {
			x--
			delA[x] = true
		} else {
			y--
			insB[y] = true
		}
	}
	var out []edit
	i, j := 0, 0
	for i < n || j < m {
		if (i < n && delA[i]) || (j < m && insB[j]) {
			e := edit{a0: i, b0: j}
			for i < n && delA[i] {
				i++
			}
			for j < m && insB[j] {
				j++
			}
			e.a1, e.b1 = i, j
			out = append(out, e)
			continue
		}
		i++
		j++
	}
	return out
}

// mapPos is where pos of the text before cs lands in the text after it. At an insertion it goes before it (assoc < 0) or after it; inside
// a replaced range, to the range's start (assoc < 0) or end.
func mapPos(cs changeSet, pos, assoc int) int {
	old, nw := 0, 0
	for _, s := range cs {
		switch {
		case !s.replace && pos < old+s.n:
			return nw + pos - old
		case s.replace && s.n == 0 && pos == old && assoc < 0:
			return nw
		case s.replace && s.n > 0 && pos == old:
			return nw
		case s.replace && pos > old && pos < old+s.n:
			if assoc < 0 {
				return nw
			}
			return nw + len(s.ins)
		}
		old += s.n
		if s.replace {
			nw += len(s.ins)
		} else {
			nw += s.n
		}
	}
	return nw + pos - old
}

// rebase carries hunks over a, made before the changes in sets, onto the text those changes left: a hunk stays off what the
// changes inserted at its edges, and where they edited inside it, the hunk wins.
func rebase(hunks []hunk, sets []changeSet) []hunk {
	out := make([]hunk, len(hunks))
	for i, h := range hunks {
		from, to := h.from, h.to
		for _, cs := range sets {
			from, to = mapPos(cs, from, 1), mapPos(cs, to, -1)
			if to < from {
				to = from
			}
		}
		out[i] = hunk{from, to, h.ins}
	}
	return out
}

// changeSetJSON is hunks over a text of length n in ChangeSet.toJSON's form, and the ChangeSet itself.
func changeSetJSON(hunks []hunk, n int) ([]any, changeSet) {
	var js []any
	var cs changeSet
	at := 0
	for _, h := range hunks {
		if h.from > at {
			js = append(js, h.from-at)
			cs = append(cs, section{n: h.from - at})
		}
		part := []any{h.to - h.from}
		if len(h.ins) > 0 {
			for _, l := range strings.Split(string(utf16.Decode(h.ins)), "\n") {
				part = append(part, l)
			}
		}
		js = append(js, part)
		cs = append(cs, section{n: h.to - h.from, replace: true, ins: h.ins})
		at = h.to
	}
	if at < n {
		js = append(js, n-at)
		cs = append(cs, section{n: n - at})
	}
	return js, cs
}

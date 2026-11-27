package usage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/handline/handline/go/kit/atomicfile"
	"github.com/handline/handline/go/kit/paths"
)

// recordSkewS is how far a window's reset may be in the past before the window counts as ended: the reset is the server's and the
// comparison is against a local clock, so the slack is for ordinary skew.
const recordSkewS = 60

// Record leaves a reading where every reader already looks: the account's status file, merged into what is there,
// so a session's id, transcript pointer and context figures survive. A first-hand reading is the authority for its window, so it
// replaces the held figure even when lower with the same reset (the vendor can reset a limit early and keep the reset time); only a
// figure observed after this reading is kept. It reports whether the file was written.
func Record(home string, r Reading) bool {
	return RecordFile(paths.StatusFile(home, r.Account), r)
}

// RecordNewer is Record of a reading from another machine, taken only when it is newer than the one held: the newest of every
// machine's stands.
func RecordNewer(home string, r Reading) bool {
	return recordFile(paths.StatusFile(home, r.Account), r, true)
}

// RecordFile is Record to a given file. The account's directory must exist: a reading for an account that is not there is not made one.
func RecordFile(path string, r Reading) bool { return recordFile(path, r, false) }

// recordMu makes each record's read, merge and write one step: a runner's reading and this machine's can arrive at once.
var recordMu sync.Mutex

func recordFile(path string, r Reading, onlyNewer bool) bool {
	if st, err := os.Stat(filepath.Dir(path)); err != nil || !st.IsDir() {
		return false
	}
	recordMu.Lock()
	defer recordMu.Unlock()
	prev := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		var v any
		if decodeNumbers(b, &v) == nil {
			if m, ok := v.(map[string]any); ok {
				prev = m
			}
		}
	}
	if at, ok := isInt(prev["collected_at"]); ok && onlyNewer && int64(at) >= r.At {
		return false
	}
	snap := make(map[string]any, len(prev)+3)
	for k, v := range prev {
		snap[k] = v
	}
	old := map[int]map[string]any{}
	for _, e := range entries(prev) {
		if m, ok := isInt(e["minutes"]); ok {
			old[m] = e
		}
	}
	ws := append([]ReadingWindow(nil), r.Windows...)
	sort.SliceStable(ws, func(i, j int) bool { return ws[i].Minutes < ws[j].Minutes })
	out := make([]any, 0, len(ws))
	for _, w := range ws {
		var pct any
		if w.Pct != nil {
			value := *w.Pct
			before := old[w.Minutes]
			oldPct, hasPct := isInt(before["used_percent"])
			oldReset, hasReset := isInt(before["resets_at"])
			oldAt, hasAt := isInt(before["observed_at"])
			same := hasReset && int64(oldReset) == w.Reset && int64(oldReset) > r.At-recordSkewS
			// A reading that arrives after a newer one of the same window must not undo it.
			if hasPct && same && hasAt && int64(oldAt) > r.At && oldPct > value {
				value = oldPct
			}
			pct = value
		}
		var reset any
		if w.Reset != 0 {
			reset = w.Reset
		}
		// Observed first-hand, now: not carried, and not lost.
		out = append(out, map[string]any{"minutes": w.Minutes, "used_percent": pct, "resets_at": reset, "observed_at": r.At,
			"carried": false, "lost": false})
	}
	snap["windows"] = out
	snap["extra"] = r.Extra.Map()
	// Without collected_at the account fails closed as stale before its figures are read.
	snap["collected_at"] = r.At
	snap["rl_source"] = "api"

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if enc.Encode(snap) != nil {
		return false
	}
	return atomicfile.Write(path, buf.String()) == nil
}

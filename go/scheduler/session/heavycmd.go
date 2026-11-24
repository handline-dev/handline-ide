package session

import (
	"database/sql"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
)

// HeavyAfter is how long any command runs before it counts in its agent's load, whatever it is.
const HeavyAfter = 60 * time.Second

// waiting are the programs whose long run is a wait, not work: Handline's own commands and sleep.
var waiting = map[string]bool{"hl": true, "sleep": true}

// setup are the commands a script runs before its work, never what it is.
var setup = map[string]bool{"cd": true, "pushd": true, "popd": true, "export": true, "unset": true, "set": true, "source": true, ".": true,
	"echo": true, "printf": true, "true": true, "ls": true, "mkdir": true}

var plainWord = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_:-]{0,29}$`)

// HeavyShape is what a Bash command that ran for ran counts as in its agent's load: the preset the lease guard knows it as
// (@build-test, @simulator, @browser) however short, else, once it ran HeavyAfter, its program and first word ("go vet", "check.sh");
// "" when it does not count. Only the shape is kept, never the command, which can hold a secret.
func HeavyShape(script string, ran time.Duration) string {
	cmds := Commands(script)
	for _, w := range cmds {
		if p := preset(w); p != "" {
			return p
		}
	}
	if ran < HeavyAfter {
		return ""
	}
	w := work(cmds)
	if w == nil || waiting[path.Base(w[0])] {
		return ""
	}
	name, args := path.Base(w[0]), w[1:]
	if shells[name] { // bash -x tools/check.sh is check.sh
		for len(args) > 0 && strings.HasPrefix(args[0], "-") {
			args = args[1:]
		}
		if len(args) > 0 {
			name, args = path.Base(args[0]), args[1:]
		}
	}
	if ops := operands(args, valueFlags[name]); len(ops) > 0 && plainWord.MatchString(ops[0]) {
		return name + " " + ops[0]
	}
	return name
}

// filters are the commands that read what another one writes (check.sh | tail -20): the work is the other.
var filters = map[string]bool{"tail": true, "head": true, "tee": true, "grep": true, "wc": true, "sort": true, "uniq": true, "cut": true,
	"cat": true, "sed": true, "awk": true, "tr": true, "xargs": true, "less": true, "jq": true}

// work is the command of a script that took its time: a script run by its path (git add -A && tools/check.sh is check.sh), else the
// last one that is neither setup nor a filter, else the first that is not setup; nil when all are setup.
func work(cmds [][]string) []string {
	var last, first []string
	for _, w := range cmds {
		name := path.Base(w[0])
		if setup[name] {
			continue
		}
		if first == nil {
			first = w
		}
		if strings.Contains(w[0], "/") || strings.HasSuffix(name, ".sh") || shells[name] && len(w) > 1 && !strings.HasPrefix(w[1], "-c") {
			return w
		}
		if !filters[name] {
			last = w
		}
	}
	if last != nil {
		return last
	}
	return first
}

// startsDir is where an agent's PreToolUse leaves, by tool_use_id, when each Bash call it let through began and its command, for the
// call's PostToolUse to read: each hook is a process of its own.
func startsDir(home, agent string) string {
	return filepath.Join(paths.AgentDir(home, agent), "bash-started")
}

var useID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

// startsKeep is how long a start whose PostToolUse never came (an interrupted call) is kept.
const startsKeep = 24 * time.Hour

// NoteBashStart notes, at a Bash call's PreToolUse, when it began; a background call is not timed here (its PostToolUse comes at once).
func NoteBashStart(home, agent string, raw map[string]any, now time.Time) {
	in, _ := raw["tool_input"].(map[string]any)
	id, _ := raw["tool_use_id"].(string)
	command, _ := in["command"].(string)
	if name, _ := raw["tool_name"].(string); name != "Bash" || in["run_in_background"] == true || !useID.MatchString(id) {
		return
	}
	dir := startsDir(home, agent)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	old, _ := os.ReadDir(dir)
	for _, e := range old {
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > startsKeep {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	_ = os.WriteFile(filepath.Join(dir, id), []byte(strconv.FormatInt(now.UnixMilli(), 10)+"\n"+command), 0o600)
}

// ForgetBashStart takes away the start of the Bash call a PermissionRequest asks about, found by its command: the time a person takes
// to answer is not the command's.
func ForgetBashStart(home, agent string, raw map[string]any) {
	in, _ := raw["tool_input"].(map[string]any)
	command, _ := in["command"].(string)
	if name, _ := raw["tool_name"].(string); name != "Bash" {
		return
	}
	dir := startsDir(home, agent)
	notes, _ := os.ReadDir(dir)
	for _, e := range notes {
		f := filepath.Join(dir, e.Name())
		if b, err := os.ReadFile(f); err == nil {
			if _, c, _ := strings.Cut(string(b), "\n"); c == command {
				_ = os.Remove(f)
			}
		}
	}
}

// BashRan is how long the Bash call a PostToolUse is of ran, taking its start note away; ok false with none.
func BashRan(home, agent string, raw map[string]any, now time.Time) (time.Duration, bool) {
	id, _ := raw["tool_use_id"].(string)
	if name, _ := raw["tool_name"].(string); name != "Bash" || !useID.MatchString(id) {
		return 0, false
	}
	f := filepath.Join(startsDir(home, agent), id)
	b, err := os.ReadFile(f)
	if err != nil {
		return 0, false
	}
	_ = os.Remove(f)
	at, _, _ := strings.Cut(string(b), "\n")
	ms, err := strconv.ParseInt(at, 10, 64)
	if err != nil || ms > now.UnixMilli() {
		return 0, false
	}
	return time.Duration(now.UnixMilli()-ms) * time.Millisecond, true
}

// HeavyCall is what the Bash call a PostToolUse ended counts as in its agent's load (HeavyShape) and how long it ran, taking its start
// note away; "" when it does not count. One with no start (a person answered its prompt) counts only as a preset, of no time.
func HeavyCall(home, agent string, raw map[string]any, now time.Time) (string, time.Duration) {
	in, _ := raw["tool_input"].(map[string]any)
	command, _ := in["command"].(string)
	if raw["tool_name"] != "Bash" || command == "" {
		return "", 0
	}
	ran, _ := BashRan(home, agent, raw, now)
	return HeavyShape(command, ran), ran
}

// HeavyRun is a heavy background shell that ended.
type HeavyRun struct {
	Job            string
	Ended, Seconds int64
	Shape          string
}

// ClaudeTmp is where Claude Code keeps each session's background shells' output: <project>/<session>/tasks/<job>.output under it.
var ClaudeTmp = "/tmp/claude-*"

// EndedBackground is agent's heavy background shells started since then that have ended: a background Bash call returns at once, so
// its run is from its background_started event to the last write of its output, once that ends with the exit line.
func EndedBackground(q db.Q, agent string, since int64) ([]HeavyRun, error) {
	rows, err := q.Query("SELECT ts, payload FROM events WHERE agent = ? AND kind = ? AND ts >= ?", agent, BackgroundKind, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HeavyRun
	for rows.Next() {
		var ts int64
		var payload sql.NullString
		if err := rows.Scan(&ts, &payload); err != nil {
			return nil, err
		}
		var p struct {
			Command, Output, Job, Session string
		}
		if json.Unmarshal([]byte(payload.String), &p) != nil || p.Command == "" || !useID.MatchString(p.Job) {
			continue
		}
		if p.Output == "" && useID.MatchString(p.Session) {
			found, _ := filepath.Glob(filepath.Join(ClaudeTmp, "*", p.Session, "tasks", p.Job+".output"))
			if len(found) > 0 {
				p.Output = found[0]
			}
		}
		end, ok := endedAt(p.Output)
		if !ok || end < ts {
			continue
		}
		if shape := HeavyShape(p.Command, time.Duration(end-ts)*time.Second); shape != "" {
			out = append(out, HeavyRun{p.Job, end, end - ts, shape})
		}
	}
	return out, rows.Err()
}

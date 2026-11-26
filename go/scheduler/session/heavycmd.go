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
	if first := firstWord(name, args); plainWord.MatchString(first) {
		return name + " " + first
	}
	return name
}

// firstWord is the word after a program that says what it does (go's test, git's status), "" for none. It is the word straight
// after the program, or past the flags of a program whose flags are known (valueFlags). Past any other flag the next word may be
// that flag's value, a password among them (curl -u, zip -P), so nothing is taken.
func firstWord(name string, args []string) string {
	if takes, known := valueFlags[name]; known {
		if ops := operands(args, takes); len(ops) > 0 {
			return ops[0]
		}
		return ""
	}
	if len(args) == 0 || len(operands(args[:1], nil)) == 0 {
		return ""
	}
	return args[0]
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

// NoteBashStart notes, at a Bash call's PreToolUse, when it began and the time limit it gave itself; a background call is not timed
// here (its PostToolUse comes at once).
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
	first := strconv.FormatInt(now.UnixMilli(), 10)
	if ms, ok := in["timeout"].(float64); ok && ms >= 1 && ms < 1e12 {
		first += " " + strconv.FormatInt(int64(ms), 10)
	}
	_ = os.WriteFile(filepath.Join(dir, id), []byte(first+"\n"+command), 0o600)
}

// started is a start note read back: when the call began and its own time limit, both in milliseconds (limit 0 when it gave none),
// and its command.
type started struct {
	at, limit int64
	command   string
}

func readStart(b []byte) (started, bool) {
	first, command, _ := strings.Cut(string(b), "\n")
	at, limit, _ := strings.Cut(first, " ")
	s := started{command: command}
	var err error
	if s.at, err = strconv.ParseInt(at, 10, 64); err != nil {
		return s, false
	}
	if limit != "" {
		if s.limit, err = strconv.ParseInt(limit, 10, 64); err != nil || s.limit < 0 {
			s.limit = 0
		}
	}
	return s, true
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
			if st, _ := readStart(b); st.command == command {
				_ = os.Remove(f)
			}
		}
	}
}

// BashRan is how long the Bash call a PostToolUse is of ran, taking its start note away; ok false with none.
func BashRan(home, agent string, raw map[string]any, now time.Time) (time.Duration, bool) {
	ran, _, ok := bashRan(home, agent, raw, now)
	return ran, ok
}

// bashRan is BashRan and the time limit the call gave itself, 0 for none.
func bashRan(home, agent string, raw map[string]any, now time.Time) (ran, limit time.Duration, ok bool) {
	id, _ := raw["tool_use_id"].(string)
	if name, _ := raw["tool_name"].(string); name != "Bash" || !useID.MatchString(id) {
		return 0, 0, false
	}
	f := filepath.Join(startsDir(home, agent), id)
	b, err := os.ReadFile(f)
	if err != nil {
		return 0, 0, false
	}
	_ = os.Remove(f)
	st, ok := readStart(b)
	if !ok || st.at > now.UnixMilli() {
		return 0, 0, false
	}
	return time.Duration(now.UnixMilli()-st.at) * time.Millisecond, time.Duration(st.limit) * time.Millisecond, true
}

// Ended is a Bash call that ended, as its hooks timed it. One with no start (a person answered its prompt) counts only as a preset,
// of no time.
type Ended struct {
	Shape string        // what it counts as in its agent's load (HeavyShape), "" when it does not count
	Would string        // what it would count as had it run HeavyAfter: the shape its runs are counted under, long or short
	Ran   time.Duration // how long it ran, 0 when it was not timed
	Timed bool          // a start note timed it
	// TimedOut is whether it failed at the time limit it gave itself: only a call that names its limit says so.
	TimedOut bool
}

// limitSlack is how far short of its limit a failed call may have run and still count as stopped by it.
const limitSlack = 2 * time.Second

// EndedCall is the Bash call the PostToolUse or PostToolUseFailure named event ended, taking its start note away; the zero value for
// any other tool's.
func EndedCall(home, agent, event string, raw map[string]any, now time.Time) Ended {
	in, _ := raw["tool_input"].(map[string]any)
	command, _ := in["command"].(string)
	if raw["tool_name"] != "Bash" || command == "" {
		return Ended{}
	}
	ran, limit, timed := bashRan(home, agent, raw, now)
	return Ended{Shape: HeavyShape(command, ran), Would: HeavyShape(command, HeavyAfter), Ran: ran, Timed: timed,
		TimedOut: timed && event == "PostToolUseFailure" && limit > 0 && ran >= limit-limitSlack}
}

// OpenStart is a Bash call that began and has not ended, as its start note says: the note of a call whose end no hook told (its
// session was killed) stays, so a reader bounds it by the call's limit.
type OpenStart struct {
	Agent string // its folder's name: the agent's key
	ID    string // the call's tool_use_id
	At    time.Time
	Limit time.Duration // the time limit the call gave itself, 0 when it gave none
	Shape string        // what it counts as once it has run HeavyAfter (HeavyShape), "" when it never counts
}

// OpenStarts are the start notes of every agent under home. Only the shape of a command leaves here, never its text.
func OpenStarts(home string) []OpenStart {
	found, _ := filepath.Glob(filepath.Join(home, "agents", "*", "bash-started", "*"))
	more, _ := filepath.Glob(filepath.Join(home, "agents", paths.Unregistered, "*", "bash-started", "*"))
	var out []OpenStart
	for _, f := range append(found, more...) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		st, ok := readStart(b)
		if !ok {
			continue
		}
		out = append(out, OpenStart{Agent: filepath.Base(filepath.Dir(filepath.Dir(f))), ID: filepath.Base(f), At: time.UnixMilli(st.at),
			Limit: time.Duration(st.limit) * time.Millisecond, Shape: HeavyShape(st.command, HeavyAfter)})
	}
	return out
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
	shells, err := backgroundShells(q, agent, since)
	if err != nil {
		return nil, err
	}
	var out []HeavyRun
	for _, b := range shells {
		end, ok := endedAt(b.output)
		if !ok || end < b.ts {
			continue
		}
		if shape := HeavyShape(b.command, time.Duration(end-b.ts)*time.Second); shape != "" {
			out = append(out, HeavyRun{b.job, end, end - b.ts, shape})
		}
	}
	return out, nil
}

// OpenBackground is a background shell whose output is there and has not ended.
type OpenBackground struct {
	Agent   string // as its event names it
	Job     string
	Started int64  // epoch seconds
	Shape   string // HeavyShape once it has run HeavyAfter, never ""
}

// OpenBackgrounds are every agent's background shells started since then that still run: their output file is there and does not end
// with the exit line yet. One whose output is gone (its session's files were cleared) is not told.
func OpenBackgrounds(q db.Q, since int64) ([]OpenBackground, error) {
	shells, err := backgroundShells(q, "", since)
	if err != nil {
		return nil, err
	}
	var out []OpenBackground
	for _, b := range shells {
		shape := HeavyShape(b.command, HeavyAfter)
		if shape == "" || b.output == "" {
			continue
		}
		if _, err := os.Stat(b.output); err != nil {
			continue
		}
		if _, ended := endedAt(b.output); ended {
			continue
		}
		out = append(out, OpenBackground{Agent: b.agent, Job: b.job, Started: b.ts, Shape: shape})
	}
	return out, nil
}

type backgroundShell struct {
	ts                          int64
	agent, command, output, job string
}

// backgroundShells are the background_started events since then, of agent only unless it is "", each with its output file found.
func backgroundShells(q db.Q, agent string, since int64) ([]backgroundShell, error) {
	query, args := "SELECT ts, agent, payload FROM events WHERE kind = ? AND ts >= ?", []any{BackgroundKind, since}
	if agent != "" {
		query, args = "SELECT ts, agent, payload FROM events WHERE agent = ? AND kind = ? AND ts >= ?", []any{agent, BackgroundKind, since}
	}
	found, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer found.Close()
	var out []backgroundShell
	for found.Next() {
		b := backgroundShell{}
		var payload sql.NullString
		if err := found.Scan(&b.ts, &b.agent, &payload); err != nil {
			return nil, err
		}
		var p struct {
			Command, Output, Job, Session string
		}
		if json.Unmarshal([]byte(payload.String), &p) != nil || p.Command == "" || !useID.MatchString(p.Job) {
			continue
		}
		if p.Output == "" && useID.MatchString(p.Session) {
			globbed, _ := filepath.Glob(filepath.Join(ClaudeTmp, "*", p.Session, "tasks", p.Job+".output"))
			if len(globbed) > 0 {
				p.Output = globbed[0]
			}
		}
		b.command, b.output, b.job = p.Command, p.Output, p.Job
		out = append(out, b)
	}
	return out, found.Err()
}

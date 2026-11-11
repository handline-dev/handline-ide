package proposalwrites

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/adminwrites"
	"github.com/handline/handline/go/internal/prefs"
	"github.com/handline/handline/go/internal/worktreeprune"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/kit/timefmt"
	"github.com/handline/handline/go/kit/yamlcompat"
	"github.com/handline/handline/go/scheduler/cronspec"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// The appliers: what approving each kind of change does.

func (e *Env) roster() *team.Roster { return team.NewRoster(e.Home) }

// rosterIn is the roster on the step's transaction, which every write of the proposal is part of.
func (e *Env) rosterIn(p step) *team.Roster { return team.NewRosterOn(p.tx, e.Home) }

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (e *Env) roleOf(p step, with string) string {
	return fmt.Sprintf("Added on %s at %s's request%s: %s", stamp(p.get("created")), p.str("requested_by"), with, p.str("why"))
}

func (e *Env) register(name string, p step, repo, role string) error {
	_, err := e.rosterIn(p).Register(name, team.RegisterOpts{Role: role, Repo: repo, Model: strPtr(p.str("model")), Effort: strPtr(p.str("effort")),
		Vendor: strPtr(p.str("vendor"))})
	return err
}

// ensureRepo is what the terminal host says of a repo: tmux knows a folder with a .git and otherwise says how to make it one.
func ensureRepo(path string) string {
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	if r, err := filepath.EvalSymlinks(path); err == nil {
		path = r
	} else if a, err := filepath.Abs(path); err == nil {
		path = a
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return path + " is a git repository"
	}
	if top := enclosingRepo(path); top != "" && top != path && trackedIn(top, path) {
		return fmt.Sprintf("%s is part of the git repository %s, not its top folder, so it cannot be a repo: set `repo:` to %s", path, top, top)
	}
	return path + " is not a git repository: run `git init` there"
}

func enclosingRepo(path string) string {
	here := path
	for {
		if _, err := os.Stat(filepath.Join(here, ".git")); err == nil {
			return here
		}
		up := filepath.Dir(here)
		if up == here {
			return ""
		}
		here = up
	}
}

func trackedIn(top, path string) bool {
	rel, err := filepath.Rel(top, path)
	if err != nil {
		return false
	}
	out, err := exec.Command("git", "-C", top, "ls-files", "--", rel).Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func registerRepo(repo string) string {
	if repo == "" {
		return ""
	}
	return " " + ensureRepo(repo) + "."
}

func (e *Env) doAdd(p step) (bool, string, error) {
	name := p.str("agent")
	if why := applyRepoProblem(p.str("repo")); why != "" {
		return false, why, nil
	}
	if ws, _ := team.SplitLegacy(name); p.str("team") != "" && p.str("team") != ws {
		return false, fmt.Sprintf("%s would be in workspace %s, not %s: an agent's workspace is the first part of its name", name, ws, p.str("team")), nil
	}
	if err := e.register(name, p, p.str("repo"), e.roleOf(p, "")); err != nil {
		return false, "", err
	}
	repo := p.str("repo")
	p.later(func() string { return registerRepo(repo) })
	teamWord := ""
	if t := p.str("team"); t != "" {
		teamWord = ", team " + t
	}
	return true, fmt.Sprintf("%s is an agent%s.", name, teamWord), nil
}

func (e *Env) doTeam(p step) (bool, string, error) {
	name, members := p.str("agent"), p.members()
	everyone := append([]string{name}, members...)
	for _, n := range members {
		if why := applyRepoProblem(p.str("repo." + n)); why != "" {
			return false, why, nil
		}
	}
	if why := applyRepoProblem(p.str("repo")); why != "" {
		return false, why, nil
	}
	role := e.roleOf(p, ", with team "+name)
	seen := map[string]bool{}
	var repos []string
	for _, n := range everyone {
		repo := p.str("repo." + n)
		if n == name {
			repo = p.str("repo")
		}
		if err := e.register(n, p, repo, role); err != nil {
			return false, "", err
		}
		if repo != "" && !seen[repo] {
			seen[repo] = true
			repos = append(repos, repo)
		}
	}
	slices.Sort(repos)
	for _, r := range repos {
		r := r
		p.later(func() string { return registerRepo(r) })
	}
	return true, fmt.Sprintf("team %s: %s are agents.", name, strings.Join(everyone, ", ")), nil
}

func (e *Env) doRetire(p step) (bool, string, error) {
	name := p.str("agent")
	// Its worktrees go first: one that cannot go keeps the agent, so no worktree is left behind with nobody's name on it.
	mine, err := worktreeprune.Of(p.tx, name)
	if err != nil {
		return false, "", err
	}
	plans, refused, err := e.leaving(name, mine, p.str("worktree")) // worktree: the founder's answer for one that holds work
	if err != nil || refused != "" {
		return false, refused, err
	}
	cleared, refused := e.clear(name, plans)
	if refused != "" {
		return false, refused, nil
	}
	var tell taskwrites.Teller // its turns go back to the agents below them; with nobody to tell they just go
	if e.Say != nil {
		tell = e.Say
	}
	key, err := paths.AgentKeyIn(p.tx, name) // before its row goes, which names the folder
	if err != nil {
		return false, "", err
	}
	if _, err := taskwrites.RetireTurns(p.tx, e.Home, tell, name); err != nil {
		return false, "", err
	}
	if err := e.rosterIn(p).Remove(name); err != nil {
		if team.IsRefusal(err) {
			return false, err.Error() + cleared, nil
		}
		return false, "", err
	}
	// The scheduling row goes with it. Its requests and events stay - they are the record of work that was done.
	if _, err := p.tx.Exec("DELETE FROM agent_state WHERE agent = ?", name); err != nil {
		return false, "", err
	}
	if _, err := p.tx.Exec("DELETE FROM account_switch WHERE agent = ?", name); err != nil {
		return false, "", err
	}
	if err := prefs.Forget(p.tx, name); err != nil {
		return false, "", err
	}
	src := paths.AgentDir(e.Home, key) // Remove refused a name with no row, so key is set
	dest := filepath.Join(e.Home, "agents", "archive", time.Now().Format("2006-01-02")+"-"+name)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return false, "", err
	}
	if _, err := os.Stat(src); err == nil {
		if _, err := os.Stat(dest); err == nil {
			dest += "-" + p.id()
		}
		if err := os.Rename(src, dest); err != nil {
			return false, "", err
		}
	}
	if err := paths.RemoveCacheDir(e.Home, key); err != nil { // the folder is archived already: a cache left behind does not undo that
		e.logf("  %s: its build caches were not removed: %v", name, err)
	}
	return true, fmt.Sprintf("%s is retired; its folder is at %s. %s", name, dest, cleared), nil
}

// doRename gives an identity another name. Its folder and rows are by key, so they stay as they are. Not while it is running.
func (e *Env) doRename(p step) (bool, string, error) {
	old, new := p.str("agent"), p.str("to") // old: the agent's key or its old name; new: the name it takes
	was, err := paths.AgentKeyIn(p.tx, old)
	if err != nil {
		return false, "", err
	}
	now, err := paths.AgentKeyIn(p.tx, new)
	if err != nil {
		return false, "", err
	}
	if now != "" && (was == "" || was == now) {
		return true, fmt.Sprintf("%s was renamed to %s already", old, new), nil // approved again after it took effect: settled, not a failure
	}
	if e.Live == nil {
		return false, fmt.Sprintf("could not confirm whether %s has a session running just now - try again in a moment.", old), nil
	}
	live, ok := e.Live()
	if !ok {
		return false, fmt.Sprintf("could not confirm whether %s has a session running just now - try again in a moment.", old), nil
	}
	if live[was] {
		return false, fmt.Sprintf("%s has a session running. Close it first (Close in its menu in the app) - an identity cannot change under a running session.", old), nil
	}
	if now != "" {
		return false, fmt.Sprintf("There is already an agent named %s; choose another name.", new), nil
	}
	if err := e.rosterIn(p).Rename(old, new); err != nil {
		if team.IsRefusal(err) {
			return false, err.Error(), nil
		}
		return false, "", err
	}
	// Every row names the agent by its key, which a rename keeps: only its own row changes.
	return true, fmt.Sprintf("%s is now %s.", old, new), nil
}

func pyStr(v any) string {
	if v == nil {
		return "None"
	}
	return fmt.Sprint(v)
}

// ---------------------------------------------------------------- settings, context, schedule, global

// idleClose is the idle close time of an agent, in the minutes the settings form takes.
type idleClose struct {
	e *Env
	r *team.Roster // the roster it writes through: a step's transaction, inside one
}

func parseIdle(text string) (int64, bool, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false, nil
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n < 0 || strings.ContainsAny(text, "+- ") {
		return 0, false, fmt.Errorf("the idle close time is a whole number of minutes, 0 for never")
	}
	return n * 60, true, nil
}

func (idleClose) Parse(text string) error {
	_, _, err := parseIdle(text)
	return err
}

func (c idleClose) Set(text, agent string) (bool, string) {
	seconds, given, err := parseIdle(text)
	if err != nil {
		return false, err.Error()
	}
	var value any
	if given {
		value = seconds
	}
	if err := c.r.SetKeys(agent, []yamlcompat.Pair{{Key: "park_timeout_s", Val: value}}); err != nil {
		return false, err.Error()
	}
	switch {
	case !given:
		return true, agent + ": idle close follows the global time"
	case seconds == 0:
		return true, "idle close never"
	}
	return true, fmt.Sprintf("idle close %d min", (seconds+59)/60)
}

func idleMinutes(seconds int64) string { return strconv.FormatInt((seconds+59)/60, 10) }

// doSettings writes exactly the settings that were proposed, through the register path.
func (e *Env) doSettings(p step) (bool, string, error) {
	agent := p.str("agent")
	changes, problem := e.rosterIn(p).ChangesFor(e.DB, agent, p.sets(), "", idleMinutes)
	if len(changes) == 0 {
		if !strings.HasPrefix(problem, team.Nothing) {
			return false, problem, nil
		}
		return true, agent + "'s settings were already so", nil
	}
	r := e.rosterIn(p)
	ok, said := r.SaveSettings(agent, changes, idleClose{e, r})
	return ok, said, nil
}

// SetByAgent is an agent's own change of its settings (the legacy agent_set): only its own, only the ordinary ones.
func (e *Env) SetByAgent(agent string, values map[string]string, speaker string) (bool, string) {
	r := e.roster()
	return r.SetByAgent(e.DB, agent, values, speaker, "", idleMinutes, idleClose{e, r})
}

var contextFiles = map[string]bool{"ROLE.md": true, "base.md": true}

func contextProblem(texts [][2]string) string {
	if len(texts) == 0 {
		return "name ROLE.md or base.md, with the whole new text"
	}
	for _, t := range texts {
		fn, text := t[0], t[1]
		switch {
		case !contextFiles[fn]:
			return fmt.Sprintf("only ROLE.md and base.md can be changed this way, not %s", pyRepr(fn))
		case strings.TrimSpace(text) == "":
			return fn + " cannot be empty"
		case textsafe.HasUnsafe(strings.NewReplacer("\t", "", "\n", "").Replace(text)):
			return fn + " carries a control character"
		}
	}
	return ""
}

// doContext writes exactly the files that were proposed, as the founder's own change.
func (e *Env) doContext(p step) (bool, string, error) {
	var texts [][2]string
	for _, kv := range p.setPairs() {
		var text string
		if err := json.Unmarshal([]byte(kv[1]), &text); err != nil {
			return false, fmt.Sprintf("%s is not text", kv[0]), nil
		}
		texts = append(texts, [2]string{kv[0], text})
	}
	if problem := contextProblem(texts); problem != "" {
		return false, problem, nil
	}
	agent := p.str("agent")
	key, err := paths.AgentKeyIn(p.tx, agent) // through the proposal's transaction, which may have added it
	if err != nil {
		return false, "", err
	}
	dir := paths.AgentDir(e.Home, key)
	if st, err := os.Stat(dir); key == "" || err != nil || !st.IsDir() {
		return false, agent + " is not an agent", nil
	}
	names := make([]string, len(texts))
	for i, t := range texts {
		if err := writeAtomic(filepath.Join(dir, t[0]), t[1]); err != nil {
			return false, "", err
		}
		names[i] = t[0]
	}
	return true, fmt.Sprintf("%s: %s replaced, from its next launch", agent, strings.Join(names, ", ")), nil
}

// doSchedule makes exactly the schedule change that was proposed.
func (e *Env) doSchedule(p step) (bool, string, error) {
	agent, st := p.str("agent"), p.sets()
	have, err := cronspec.Load(e.Home, agent)
	if err != nil {
		if bad, isBad := err.(*cronspec.ScheduleError); isBad {
			return false, bad.Msg, nil
		}
		return false, "", err
	}
	write := func(items []cronspec.Schedule) (bool, string, error) {
		next, err := cronspec.ParseSchedules(items)
		if err != nil {
			return false, err.Error(), nil
		}
		if err := adminwrites.WriteSchedules(e.rosterIn(p), agent, next); err != nil {
			return false, "", err
		}
		return true, fmt.Sprintf("%s's schedules: %s done", agent, st["action"]), nil
	}
	if st["action"] == "add" {
		return write(append(have, cronspec.Schedule{Cron: st["cron"], Message: st["message"]}))
	}
	index := -1
	for i, it := range have {
		if cronspec.SID(it) == st["sid"] {
			index = i
			break
		}
	}
	if index < 0 {
		return false, fmt.Sprintf("%s's schedule '%s' is no longer there as it was shown; nothing changed", agent, st["message_was"]), nil
	}
	if st["action"] == "edit" {
		have[index] = cronspec.Schedule{Cron: st["cron"], Message: st["message"]}
		return write(have)
	}
	rest := append(append([]cronspec.Schedule(nil), have[:index]...), have[index+1:]...)
	if len(rest) > 0 {
		if err := adminwrites.WriteSchedules(e.rosterIn(p), agent, rest); err != nil {
			return false, "", err
		}
	} else if err := adminwrites.WriteSchedules(e.rosterIn(p), agent, nil); err != nil {
		return false, "", err
	}
	return true, fmt.Sprintf("%s's schedules: %s done", agent, st["action"]), nil
}

// doGlobal sets one global switch.
func (e *Env) doGlobal(p step) (bool, string, error) {
	switchName, value := p.str("agent"), p.sets()["value"]
	var set func(get func(string) any, set func(string, any))
	switch switchName {
	case "idle-timeout":
		n := int64(0)
		if value != "never" {
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil || v < 0 || strings.ContainsAny(value, "+- ") {
				return false, switchName + " takes a number of seconds, or never", nil
			}
			n = v
		}
		set = func(_ func(string) any, put func(string, any)) { put("park_timeout_s", n) }
	case "notice-digest":
		n, err := timefmt.ParseSpan(value)
		if err != nil || n <= 0 {
			return false, switchName + " takes a positive span", nil
		}
		set = func(_ func(string) any, put func(string, any)) { put("notice_digest_s", n) }
	case "ignore-cache-expiry", "dev-mode":
		if value != "on" && value != "off" {
			return false, switchName + " takes on or off", nil
		}
		on := value == "on"
		key := map[string]string{"ignore-cache-expiry": "ignore_cache_expiry", "dev-mode": "dev_mode"}[switchName]
		set = func(_ func(string) any, put func(string, any)) {
			switch {
			case switchName == "ignore-cache-expiry" && !on:
				put(key, nil)
			default:
				put(key, on)
			}
		}
	default:
		return false, switchName + " is not a global switch", nil
	}
	if err := adminwrites.EditConfig(e.Home, set); err != nil {
		return false, "", err
	}
	return true, fmt.Sprintf("%s set to %s", switchName, value), nil
}

// pyRepr is Python's repr of a short text, as a refusal quotes a name.
func pyRepr(s string) string {
	if strings.ContainsAny(s, "'\\\n\r\t") {
		return fmt.Sprintf("%q", s)
	}
	return "'" + s + "'"
}

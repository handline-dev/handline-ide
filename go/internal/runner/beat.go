package runner

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/sysres"
)

// heartbeats sends node.heartbeat while the master is reached: at once when the link comes up or the sessions here change, then
// BeatEvery after each answer, so one is in flight at a time and a slow network delays the next instead of stacking them.
func (r *Runner) heartbeats() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stop:
			return
		case <-r.beatNow:
		case <-time.After(BeatEvery):
		}
		if err := r.SendBeat(); err != nil {
			r.cfg.Log("runner: the heartbeat did not reach the master: %v", err)
		}
	}
}

// SendBeat is one heartbeat, nothing while the master is not reached. Its answer names the sessions the master no longer places here:
// their work goes to a rescue branch and they stop. A session whose terminal is gone (the machine restarted) goes the same way first.
func (r *Runner) SendBeat() error {
	l := r.linked()
	if l == nil {
		return nil
	}
	r.endGone()
	got, err := l.Call("node.heartbeat", r.Beat(), hlclient.Quick)
	if err != nil {
		return err
	}
	if doors, ok := got["doors"]; ok {
		r.keepDoors(doors)
	}
	r.keepAppUpdate(got["app_update"])
	if raw, ok := got["build"]; ok {
		var o BuildOffer
		if b, err := json.Marshal(raw); err == nil && json.Unmarshal(b, &o) == nil {
			r.takeBuild(o)
		}
	}
	stop, _ := got["stop"].([]any)
	for _, v := range stop {
		if agent, _ := v.(string); agent != "" {
			r.startRescue(agent)
		}
	}
	var check []string
	list, _ := got["check"].([]any)
	for _, v := range list {
		if s, _ := v.(string); s != "" {
			check = append(check, s)
		}
	}
	r.mu.Lock()
	changed := strings.Join(check, "\n") != strings.Join(r.check, "\n")
	r.check = check
	r.mu.Unlock()
	if changed {
		r.beatSoon() // the master hears at once which of them are here
	}
	return nil
}

// beatSoon has the next heartbeat go at once.
func (r *Runner) beatSoon() {
	select {
	case r.beatNow <- struct{}{}:
	default:
	}
}

// Beat is this runner's heartbeat now, as node.heartbeat's params.
func (r *Runner) Beat() map[string]any {
	b := Beat{Version: r.cfg.Sha, Platform: Platform(), Boot: bootID(), Accounts: r.accounts(), Has: r.has(), Battery: r.cfg.Battery(), Door: r.door()}
	if s, ok := joined.SetAsideOf(r.cfg.Home); ok {
		b.SetAside = &s
	}
	b.App = appOf(r.cfg.Home)
	if m, err := sysres.Read(); err == nil {
		b.Load = Load{Cores: m.Cores, Load1: m.Load1, RAMTotal: m.RAMTotal, RAMFree: m.RAMFree}
	}
	r.mu.Lock()
	b.BuildRefused = r.refusedBuild[r.cfg.Master.Version]
	sessions := make(map[string]session, len(r.sessions))
	panes := make(map[string]int, len(r.sessions))
	for a, s := range r.sessions {
		sessions[a] = s
		panes[a] = s.Pid
	}
	check := r.check
	r.mu.Unlock()
	b.Has.Programs, b.Has.Asked = []string{}, append([]string{}, check...)
	for _, prog := range check {
		if _, err := exec.LookPath(prog); err == nil {
			b.Has.Programs = append(b.Has.Programs, prog)
		}
	}
	sampler := &sysres.Sampler{Home: r.cfg.Home, Panes: func() map[string]int { return panes }}
	if sampler.Sample(0) == nil {
		_, rss := sampler.AgentsMemory()
		for _, n := range rss {
			b.Load.AgentsRSS += n
		}
	}
	for a, s := range sessions {
		bs := BeatSession{Agent: a, Handle: s.Handle}
		if s.Folder != "" {
			if out, err := r.gitOut("-C", s.Folder, "symbolic-ref", "-q", "--short", "HEAD"); err == nil {
				bs.Branch = out
			}
			if out, err := r.gitOut("-C", s.Folder, "rev-parse", "-q", "--verify", "@{upstream}"); err == nil {
				bs.Pushed = out
			}
		}
		b.Sessions = append(b.Sessions, bs)
	}
	return toParams(b)
}

// endGone puts the work of each session whose terminal is gone (the machine restarted) on a rescue branch and forgets it.
func (r *Runner) endGone() {
	if r.cfg.Term == nil {
		return
	}
	live := r.cfg.Term.ListTerminals()
	if live == nil {
		return
	}
	r.mu.Lock()
	var gone []string
	for a, s := range r.sessions {
		if !live[s.Handle] {
			gone = append(gone, a)
		}
	}
	r.mu.Unlock()
	for _, a := range gone {
		r.startRescue(a)
	}
}

// rescueTries is how many times a rescue's push is tried before the session is let go, its worktree left on disk as it was.
const rescueTries = 3

// startRescue rescues agent's session beside the heartbeats, which a slow push must not hold up; one rescue an agent at a time.
func (r *Runner) startRescue(agent string) {
	r.mu.Lock()
	if r.stopped || r.rescuing[agent] {
		r.mu.Unlock()
		return
	}
	r.rescuing[agent] = true
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		r.rescue(agent)
		r.mu.Lock()
		delete(r.rescuing, agent)
		r.mu.Unlock()
	}()
}

// rescue commits what agent's worktree holds that its branch's remote does not, staged or not, on rescue/<agent>-<date> and pushes it
// once, then closes agent's session here and forgets it. Its branch is left as it was.
func (r *Runner) rescue(agent string) {
	r.mu.Lock()
	s, ok := r.sessions[agent]
	r.mu.Unlock()
	if !ok {
		return
	}
	if s.Folder != "" && !s.Rescued {
		branch, err := r.rescueBranch(agent, s.Folder)
		r.mu.Lock()
		if err != nil {
			r.rescueFails[agent]++
			tries := r.rescueFails[agent]
			r.mu.Unlock()
			if tries < rescueTries {
				r.cfg.Log("runner: %s: the work left in %s was not rescued, again at the next heartbeat: %v", agent, s.Folder, err)
				return
			}
			r.cfg.Log("runner: %s: the work left in %s was not rescued after %d tries, so it stays there: %v", agent, s.Folder, tries, err)
			r.mu.Lock()
		} else if branch != "" {
			r.cfg.Log("runner: %s: the work left in %s is on %s", agent, s.Folder, branch)
		}
		delete(r.rescueFails, agent)
		if cur, ok := r.sessions[agent]; ok && cur == s {
			s.Rescued = true // pushed once: a close that fails is tried again without another push
			r.sessions[agent] = s
		}
		err = r.save()
		r.mu.Unlock()
		if err != nil {
			r.cfg.Log("runner: %s: %v", agent, err)
		}
	}
	if r.cfg.Term != nil && r.cfg.Term.ListTerminals()[s.Handle] {
		if _, err := r.close(agent, s.Handle); err != nil {
			r.cfg.Log("runner: %s: its session %s did not stop: %v", agent, s.Handle, err)
		}
		return
	}
	if err := r.forgetSession(agent, s); err != nil {
		r.cfg.Log("runner: %s: %v", agent, err)
	}
}

// rescueBranch is the branch rescue pushed folder's leftover work to, "" when it had none: no change in the tree and no commit its
// upstream lacks. The rescue commit is made from a scratch index on top of HEAD, so the worktree, its index and its branch stay as
// they were.
func (r *Runner) rescueBranch(agent, folder string) (string, error) {
	if _, err := os.Stat(folder); err != nil {
		return "", nil // the worktree is gone with what it held
	}
	if r.git("-C", folder, "rev-parse", "--verify", "-q", "HEAD") != nil {
		return "", nil // not a repo, or one with no commit: nothing to push it to
	}
	dirty, err := r.gitOut("-C", folder, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	ahead := "1" // no upstream: none of it was pushed
	if out, err := r.gitOut("-C", folder, "rev-list", "--count", "@{upstream}..HEAD"); err == nil {
		ahead = out
	}
	if dirty == "" && ahead == "0" {
		return "", nil
	}
	commit := "HEAD"
	if dirty != "" {
		if commit, err = r.snapshot(folder); err != nil {
			return "", err
		}
	}
	branch := "rescue/" + agent + "-" + time.Now().Format("20060102-150405")
	return branch, r.git("-C", folder, "push", "-q", "--no-verify", "origin", commit+":refs/heads/"+branch)
}

// snapshot is a commit on HEAD of everything in folder's tree, untracked files too, made through a scratch index.
func (r *Runner) snapshot(folder string) (string, error) {
	index, err := os.CreateTemp("", "rescue-index-")
	if err != nil {
		return "", err
	}
	index.Close()
	defer os.Remove(index.Name())
	env := []string{"GIT_INDEX_FILE=" + index.Name()}
	if _, err := r.gitEnv(env, "-C", folder, "read-tree", "HEAD"); err != nil {
		return "", err
	}
	if _, err := r.gitEnv(env, "-C", folder, "add", "-A"); err != nil {
		return "", err
	}
	tree, err := r.gitEnv(env, "-C", folder, "write-tree")
	if err != nil {
		return "", err
	}
	// Handline's commit, as worktreeprune's rescue is: a machine lent with no git identity still saves the work
	env = append(env, "GIT_AUTHOR_NAME=handline", "GIT_COMMITTER_NAME=handline", "GIT_AUTHOR_EMAIL=handline@localhost", "GIT_COMMITTER_EMAIL=handline@localhost")
	return r.gitEnv(env, "-C", folder, "commit-tree", tree, "-p", "HEAD", "-m", "Left on this machine when its session was taken back")
}

// accounts is this machine's Claude account folders with the email each is logged in as, its config dir and its reading: a runner keeps no
// accounts table, so a folder under accounts/ (a key) or accounts/.unregistered/ (a name) is an account when its .claude.json names one.
func (r *Runner) accounts() []BeatAccount {
	out := loggedInAccounts(r.cfg.Home)
	for i := range out {
		out[i].Reading, out[i].LoginFailed = r.reading(out[i].Name, out[i].Email)
	}
	return out
}

func loggedInAccounts(home string) []BeatAccount {
	out := []BeatAccount{}
	for _, name := range AccountFolders(home) {
		email := (team.Accounts{Home: home}).Email(name)
		dir, err := team.AccountConfigDir(home, name)
		if email == "" || err != nil {
			continue
		}
		if rel, err := filepath.Rel(home, dir); err == nil && filepath.IsLocal(rel) {
			out = append(out, BeatAccount{Name: name, Email: email, Dir: rel})
		}
	}
	return out
}

// AccountFolders is the names of the account folders in home: each under accounts/ and accounts/.unregistered/.
func AccountFolders(home string) []string {
	var out []string
	add := func(dir string) {
		es, _ := os.ReadDir(dir)
		for _, e := range es {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				out = append(out, e.Name())
			}
		}
	}
	add(paths.AccountsDir(home))
	add(filepath.Join(paths.AccountsDir(home), paths.Unregistered))
	return out
}

// loggedIn is the account folders its heartbeat names: what the runner counts the usage of (node.tokens).
func loggedIn(home string) []string {
	var out []string
	for _, a := range loggedInAccounts(home) {
		out = append(out, a.Name)
	}
	return out
}

// hasEvery is how long what the machine has is kept before it is looked at again: asking for its simulators takes a second.
const hasEvery = 10 * time.Minute

// hasKept is what the machine has, as last looked at.
type hasKept struct {
	sync.Mutex
	at  time.Time
	has Has
}

// has is what this machine has for an agent's work.
func (r *Runner) has() Has {
	r.hasKept.Lock()
	defer r.hasKept.Unlock()
	if !r.hasKept.at.IsZero() && time.Since(r.hasKept.at) < hasEvery {
		return r.hasKept.has
	}
	h := Has{CLIs: []string{}, Simulators: []string{}}
	for _, v := range team.KnownVendors() {
		if _, err := exec.LookPath(v); err == nil {
			h.CLIs = append(h.CLIs, v)
		}
	}
	h.MCP = mcpPrograms(r.cfg.Home)
	if out, err := exec.Command("xcode-select", "-p").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		h.Xcode = true
		if out, err := exec.Command("xcrun", "simctl", "list", "devices", "available").Output(); err == nil {
			h.Simulators = simulators(string(out))
		}
	}
	r.hasKept.at, r.hasKept.has = time.Now(), h
	return h
}

// simulators is the device names simctl list devices lists, once each.
func simulators(out string) []string {
	seen := map[string]bool{}
	list := []string{}
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "--") || strings.HasPrefix(l, "==") {
			continue
		}
		name, _, ok := strings.Cut(l, " (")
		if ok && !seen[name] {
			seen[name] = true
			list = append(list, name)
		}
	}
	return list
}

// mcpPrograms is the MCP servers this machine's accounts have set up (mcpServers in each account's .claude.json) whose program is here.
func mcpPrograms(home string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, name := range AccountFolders(home) {
		dir, err := team.AccountConfigDir(home, name)
		if err != nil {
			continue
		}
		var cfg struct {
			Servers map[string]struct {
				Command string `json:"command"`
				URL     string `json:"url"`
			} `json:"mcpServers"`
		}
		if b, err := os.ReadFile(filepath.Join(dir, ".claude.json")); err != nil || json.Unmarshal(b, &cfg) != nil {
			continue
		}
		for server, c := range cfg.Servers {
			if seen[server] {
				continue
			}
			if _, err := exec.LookPath(c.Command); c.URL != "" || c.Command != "" && err == nil {
				seen[server] = true
				out = append(out, server)
			}
		}
	}
	sort.Strings(out)
	return out
}

// toParams is v as the params of a call: its JSON fields.
func toParams(v any) map[string]any {
	b, _ := json.Marshal(v)
	var p map[string]any
	json.Unmarshal(b, &p)
	return p
}

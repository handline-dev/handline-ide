package nodes

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/homefiles"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// Launched is a launch the master built for an agent on a runner, its paths still the master's: what hl execs there.
type Launched struct{ Argv, Env, Warnings []string }

// LaunchFunc builds the launch hl launch on node asks for agent (session.launch_prepare's params): a refusal is an *rpc.Error.
type LaunchFunc func(node, agent string, params map[string]any) (*Launched, error)

// SharedItems are the shared files every claude account sees, under shared/.claude (team's shared items, plugins left out: each machine
// keeps its own).
var SharedItems = []string{"settings.json", "CLAUDE.md", "skills", "agents", "commands"}

func (r *Registry) sharedItems() []string {
	var out []string
	for _, it := range SharedItems {
		out = append(out, filepath.Join(r.Home, "shared", team.ConfigDirName(team.DefaultVendor), it))
	}
	return out
}

// SharedEvery is how often the master looks for a change to the shared items.
const SharedEvery = 2 * time.Second

// runnerHome is node's home, asked once per link.
func (r *Registry) runnerHome(node string) (string, error) {
	r.mu.Lock()
	c := r.links[node]
	home, ok := r.homes[c]
	r.mu.Unlock()
	if c == nil {
		return "", errors.New("node " + node + " is not linked to this master")
	}
	if ok {
		return home, nil
	}
	got, err := c.Call("runner.home", nil, ReadWait)
	if err != nil {
		return "", err
	}
	home, _ = got["home"].(string)
	if !filepath.IsAbs(home) {
		return "", errors.New("node " + node + " named no home")
	}
	r.mu.Lock()
	if r.links[node] == c {
		r.homes[c] = home
	}
	r.mu.Unlock()
	return home, nil
}

// swap is s with every path under from made the same path under to: from as a whole path, not the tail of a longer one.
func swap(s, from, to string) string { return swapAll(s, from, to).(string) }

// swapAll is v with swap applied to every text in it.
func swapAll(v any, from, to string) any {
	if from == "" || from == to {
		return v
	}
	re := regexp.MustCompile(`(^|[^\w./-])` + regexp.QuoteMeta(from) + `(/|[^\w-]|$)`)
	one := func(s string) string {
		if !strings.Contains(s, from) {
			return s
		}
		return re.ReplaceAllStringFunc(s, func(m string) string { return strings.Replace(m, from, to, 1) })
	}
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case string:
			return one(x)
		case []any:
			out := make([]any, len(x))
			for i, e := range x {
				out[i] = walk(e)
			}
			return out
		case []string:
			out := make([]string, len(x))
			for i, e := range x {
				out[i] = one(e)
			}
			return out
		case map[string]any:
			out := make(map[string]any, len(x))
			for k, e := range x {
				out[one(k)] = walk(e)
			}
			return out
		}
		return v
	}
	return walk(v)
}

// attachmentRefs are the files under the master's attachments folder that text names; a path that climbs out of it is none.
func (r *Registry) attachmentRefs(text string) []string {
	at := filepath.Join(r.Home, "attachments")
	if !strings.Contains(text, at+"/") {
		return nil
	}
	r.refOnce.Do(func() { r.refRE = regexp.MustCompile(regexp.QuoteMeta(at+"/") + `[^\s"'<>()\[\]` + "`" + `]+`) })
	var out []string
	for _, p := range r.refRE.FindAllString(text, -1) {
		p = filepath.Clean(strings.TrimRight(p, ".,;:!?")) // a path that ends a sentence
		if rel, err := filepath.Rel(at, p); err == nil && homefiles.Inside(rel) {
			out = append(out, p)
		}
	}
	return out
}

// sendNamed sends node the attachments the texts in v name, before the agent is told them.
func (r *Registry) sendNamed(node string, v any) error {
	var refs []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			refs = append(refs, r.attachmentRefs(x)...)
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	if len(refs) == 0 {
		return nil
	}
	files, err := homefiles.Collect(r.Home, refs...)
	if err != nil || len(files) == 0 {
		return err
	}
	return r.sendFiles(node, files, nil)
}

func (r *Registry) sendFiles(node string, files []homefiles.File, deleted []string) error {
	run, err := r.Runner(node)
	if err != nil {
		return err
	}
	gone := make([]any, 0, len(deleted))
	for _, d := range deleted {
		gone = append(gone, d)
	}
	_, err = run.c.Call("runner.files", map[string]any{"files": homefiles.Wire(files), "deleted": gone}, CallWait)
	return err
}

// forRunner is text the master tells an agent on node: its paths are node's, and the attachments it names are there first.
func (r *Registry) forRunner(node, text string) (string, error) {
	if !strings.Contains(text, r.Home) {
		return text, nil
	}
	home, err := r.runnerHome(node)
	if err != nil {
		return "", err
	}
	if err := r.sendNamed(node, text); err != nil {
		return "", err
	}
	return swap(text, r.Home, home), nil
}

// answerFor is a forwarded call's answer as the agent on node (whose home is home) is told it: its paths node's, the attachments it
// names sent there first. The call has had its effect, so nothing here fails it: an attachment not sent is only logged.
func (r *Registry) answerFor(node, home string, res any) any {
	b, err := json.Marshal(res)
	if err != nil || !bytes.Contains(b, []byte(r.Home)) {
		return res
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber() // an id past 2^53 stays itself
	var v any
	if err := dec.Decode(&v); err != nil {
		return res
	}
	if err := r.sendNamed(node, v); err != nil {
		r.log("  attachments not sent to node %s: %v", node, err)
	}
	return swapAll(v, r.Home, home)
}

func (r *Registry) log(format string, a ...any) {
	if r.Log != nil {
		r.Log(format, a...)
	}
}

// sessionNode is the node agent's session is on; "" for none.
func (r *Registry) sessionNode(agent string) (string, error) {
	var node string
	err := r.DB.QueryRow("SELECT node FROM node_sessions WHERE agent = ?", agent).Scan(&node)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return node, err
}

// Flush brings the agent's files up from the runner its session is on, before the master reads them (its handoff note at a close).
// An agent with no session on a runner has nothing to bring.
func (r *Registry) Flush(agent string) error {
	node, err := r.sessionNode(agent)
	if err != nil || node == "" {
		return err
	}
	run, err := r.Runner(node)
	if err != nil {
		return err
	}
	_, err = run.c.Call("runner.flush", map[string]any{"agent": agent}, ReadWait) // the tick waits on it: a slow runner is passed by
	return err
}

// WatchShared sends each linked runner the shared items as they change, every SharedEvery until stop: a runner linked anew gets them
// all.
func (r *Registry) WatchShared(stop <-chan struct{}) {
	sent := map[*rpc.Conn]map[string]homefiles.Stamp{}
	t := time.NewTicker(SharedEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		r.mu.Lock()
		links := make(map[string]*rpc.Conn, len(r.links))
		for node, c := range r.links {
			links[node] = c
		}
		r.mu.Unlock()
		if len(links) == 0 {
			clear(sent)
			continue
		}
		now := homefiles.Stamps(r.Home, r.sharedItems()...)
		for c := range sent {
			if !linked(links, c) {
				delete(sent, c)
			}
		}
		for node, c := range links {
			changed, gone := homefiles.Changed(sent[c], now)
			if len(changed) == 0 && len(gone) == 0 {
				continue
			}
			var abs []string
			for _, p := range changed {
				abs = append(abs, filepath.Join(r.Home, p))
			}
			files, err := homefiles.Collect(r.Home, abs...)
			if err == nil {
				err = r.sendFiles(node, files, gone)
			}
			if err != nil {
				r.log("  shared items not sent to node %s: %v", node, err)
				continue
			}
			sent[c] = now
		}
	}
}

func linked(links map[string]*rpc.Conn, c *rpc.Conn) bool {
	for _, l := range links {
		if l == c {
			return true
		}
	}
	return false
}

// linkNode is the node c is the link of, refused for any other connection, and whether agent's session is on it.
func (r *Registry) linkNode(c *rpc.Conn, agent string) (string, error) {
	node := r.NodeOf(c.Root())
	if node == "" {
		return "", rpc.Refused("this is a runner's own call, on its link", "node", "")
	}
	if agent == "" {
		return "", rpc.BadRequestError("agent is a string", "agent", rpc.InvalidParams)
	}
	on, err := r.sessionNode(agent)
	if err != nil {
		return "", err
	}
	if on != node {
		return "", rpc.Refused(agent+" has no session on this runner", "not_on_node", "")
	}
	return node, nil
}

// launch is node.launch: the launch hl launch asks for on a runner, built here with its paths made the runner's, and the files it
// starts with.
func (r *Registry) launch(c *rpc.Conn, p map[string]any) (any, error) {
	agent, _ := p["agent"].(string)
	node, err := r.linkNode(c, agent)
	if err != nil {
		return nil, err
	}
	if r.Launch == nil {
		return nil, rpc.Refused("this master starts no agents", "launch", "")
	}
	home, err := r.runnerHome(node)
	if err != nil {
		return nil, err
	}
	asked, _ := p["params"].(map[string]any)
	params := make(map[string]any, len(asked))
	for k, v := range asked {
		params[k] = v
	}
	if pf, _ := asked["prompt_file"].(string); pf != "" { // the wake's prompt, written here and named as it was typed there
		pf = filepath.Clean(swap(pf, home, r.Home))
		if filepath.Dir(pf) != filepath.Join(r.Home, "wake") || !strings.HasPrefix(filepath.Base(pf), agent+".") {
			return nil, rpc.Refused("a runner's launch takes only the prompt the master wrote for its agent", "prompt_file", "")
		}
		params["prompt_file"] = pf
	}
	agentDir := paths.AgentDir(r.Home, agent)
	dir, err := homefiles.Rel(r.Home, agentDir)
	if err != nil {
		return nil, err
	}
	// It starts with its memory and handoff note, the MCP servers and the shared items every account sees: read before the launch is
	// built, which uses up its prompt.
	files, err := homefiles.Collect(r.Home, append([]string{filepath.Join(agentDir, "memory"), paths.HandoffNoteFile(r.Home, agent),
		paths.MCPConfig(r.Home)}, r.sharedItems()...)...)
	if err != nil {
		return nil, err
	}
	got, err := r.Launch(node, agent, params)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, s := range append(append(append([]string{}, got.Argv...), got.Env...), got.Warnings...) {
		refs = append(refs, r.attachmentRefs(s)...)
	}
	if named, err := homefiles.Collect(r.Home, refs...); err != nil {
		r.log("  attachments of %s's launch not sent to node %s: %v", agent, node, err)
	} else {
		files = append(files, named...)
	}
	return map[string]any{"argv": swapAll(got.Argv, r.Home, home), "env": swapAll(got.Env, r.Home, home),
		"warnings": swapAll(got.Warnings, r.Home, home), "files": homefiles.Wire(files), "agent_dir": dir}, nil
}

// agentFiles is node.agent_files: the agent's memory and handoff note as its session on the runner wrote them.
func (r *Registry) agentFiles(c *rpc.Conn, p map[string]any) (any, error) {
	agent, _ := p["agent"].(string)
	if _, err := r.linkNode(c, agent); err != nil {
		return nil, err
	}
	files, err := homefiles.FromWire(p["files"])
	if err != nil {
		return nil, rpc.BadRequestError(err.Error(), "files", rpc.InvalidParams)
	}
	deleted := homefiles.Strings(p["deleted"])
	ok := func(rel string) bool { return rel == "handoff.md" || strings.HasPrefix(rel, "memory/") }
	for _, f := range files {
		if !ok(f.Path) {
			return nil, rpc.Refused(f.Path+" is not the agent's memory or handoff note", "path", "")
		}
	}
	for _, d := range deleted {
		if !ok(d) {
			return nil, rpc.Refused(d+" is not the agent's memory or handoff note", "path", "")
		}
	}
	if err := homefiles.Apply(paths.AgentDir(r.Home, agent), files, deleted); err != nil {
		return nil, rpc.Refused(err.Error(), "path", "")
	}
	return map[string]any{"accepted": true}, nil
}

// Package projects puts each workspace's repo on the master's runners (docs/several-machines.md): a runner clones it from the
// repo's remote when it links (it joined, or came back) and when a workspace is added, so an agent placed there has the project.
package projects

import (
	"database/sql"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/protocol/rpc"
)

// States of a workspace's repo on a runner.
const (
	Cloning  = "cloning"
	Cloned   = "cloned"
	Failed   = "failed"
	NoRemote = "no_remote" // the repo has no origin: the workspace runs only on the machine that holds its folder
)

// Project is a workspace and its repo's remote, "" when it has none.
type Project struct {
	Workspace, Name, Remote string
}

// List is every workspace, or the one named name when name is not "", and the origin of the repo at its folder, read from git now.
func List(db *sql.DB, name string) ([]Project, error) {
	rs, err := db.Query("SELECT id, name, folder FROM workspaces WHERE ? = '' OR name = ? ORDER BY id", name, name)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []Project
	for rs.Next() {
		var id int64
		var ws, folder string
		if err := rs.Scan(&id, &ws, &folder); err != nil {
			return nil, err
		}
		out = append(out, Project{Workspace: actor.Key(actor.Workspace, id), Name: ws, Remote: remoteOf(folder)})
	}
	return out, rs.Err()
}

func remoteOf(folder string) string {
	r := gitrun.Run(folder, "remote", "get-url", "origin")
	if r.RC != 0 {
		return ""
	}
	return strings.TrimSpace(r.Stdout)
}

// Syncer sends the projects to the runners and keeps what each answered, until the master restarts: a runner is synced again each
// time it links, and when a workspace is added. A repo that failed there is said once and tried again after a wait that doubles from
// RetryAfter up to an hour, a link meanwhile not bringing it sooner.
type Syncer struct {
	DB         *sql.DB
	Reg        *nodes.Registry
	Log        func(format string, args ...any)
	RetryAfter time.Duration   // the first wait after a failure; 0 is DefaultRetryAfter
	Stop       <-chan struct{} // closed when the master stops: no retry starts after it
	// Clone has node clone remote; nil is its runner's runner.project.sync over the link.
	Clone func(node, remote string) error

	mu    sync.Mutex
	state map[string]map[string]State // node -> workspace key -> its repo there
	fails map[string]map[string]fail  // node -> workspace key -> its failures in a row
	wg    sync.WaitGroup
}

// DefaultRetryAfter is the first wait before a repo that failed on a runner is tried again.
const DefaultRetryAfter = 30 * time.Second

// maxRetryAfter bounds the wait between tries of a repo that keeps failing.
const maxRetryAfter = time.Hour

type fail struct {
	n    int
	next time.Time
}

// State is a workspace's repo on one runner.
type State struct {
	State, Error string
}

// Node syncs every workspace with a remote to node.
func (s *Syncer) Node(node string) {
	ps, err := List(s.DB, "")
	if err != nil {
		s.Log("projects: the workspaces could not be read for node %s: %v", node, err)
		return
	}
	for _, p := range ps {
		s.sync(node, p)
	}
}

// Workspace syncs the workspace named name to every runner linked now; one linked later gets it when it links.
func (s *Syncer) Workspace(name string) {
	ps, err := List(s.DB, name)
	if err != nil {
		s.Log("projects: the workspaces could not be read for %s: %v", name, err)
		return
	}
	for _, p := range ps {
		for _, node := range s.Reg.Linked() {
			s.sync(node, p)
		}
	}
}

// sync has node clone p's repo, beside the caller: a clone may take minutes. One already running there is not started again.
func (s *Syncer) sync(node string, p Project) {
	if p.Remote == "" {
		s.set(node, p.Workspace, State{State: NoRemote})
		return
	}
	if !s.start(node, p.Workspace) {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := s.clone(node, p.Remote)
		if err == nil {
			s.mu.Lock()
			delete(s.fails[node], p.Workspace)
			s.setLocked(node, p.Workspace, State{State: Cloned})
			s.mu.Unlock()
			return
		}
		var answered *rpc.Error
		if !errors.As(err, &answered) { // the link, not the repo: its next link tries again
			s.Log("projects: %s on node %s: %v", p.Name, node, err)
			s.set(node, p.Workspace, State{State: Failed, Error: err.Error()})
			return
		}
		wait := s.failed(node, p, err)
		time.AfterFunc(wait, func() { s.retry(node, p) })
	}()
}

func (s *Syncer) clone(node, remote string) error {
	if s.Clone != nil {
		return s.Clone(node, remote)
	}
	r, err := s.Reg.Runner(node)
	if err != nil {
		return err
	}
	return r.SyncProject(remote)
}

// failed keeps p's failure on node, one its runner answered, and is the wait before it is tried again; only the first of a row of failures is logged.
func (s *Syncer) failed(node string, p Project, err error) time.Duration {
	ws := p.Workspace
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fails == nil {
		s.fails = map[string]map[string]fail{}
	}
	if s.fails[node] == nil {
		s.fails[node] = map[string]fail{}
	}
	f := s.fails[node][ws]
	first := s.RetryAfter
	if first <= 0 {
		first = DefaultRetryAfter
	}
	wait := min(first, maxRetryAfter)
	for range f.n {
		wait = min(2*wait, maxRetryAfter)
	}
	if f.n == 0 {
		s.Log("projects: %s on node %s: %v; tried again after a wait that grows from %s", p.Name, node, err, wait)
	}
	s.fails[node][ws] = fail{n: f.n + 1, next: time.Now().Add(wait)}
	s.setLocked(node, ws, State{State: Failed, Error: err.Error()})
	return wait
}

// retry syncs p's workspace to node again once its wait is over, as it is now, while node is linked: one not linked then is synced
// when it links.
func (s *Syncer) retry(node string, p Project) {
	select {
	case <-s.Stop:
		return
	default:
	}
	if !slices.Contains(s.Reg.Linked(), node) {
		return
	}
	ps, err := List(s.DB, p.Name)
	if err != nil {
		s.Log("projects: the workspace %s could not be read for node %s: %v", p.Name, node, err)
		return
	}
	for _, now := range ps {
		if now.Workspace == p.Workspace {
			s.sync(node, now)
		}
	}
}

// Wait waits for the syncs running now.
func (s *Syncer) Wait() { s.wg.Wait() }

// start marks ws cloning on node; false when it is already.
func (s *Syncer) start(node, ws string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state[node][ws].State == Cloning || time.Now().Before(s.fails[node][ws].next) {
		return false
	}
	s.setLocked(node, ws, State{State: Cloning})
	return true
}

func (s *Syncer) set(node, ws string, st State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setLocked(node, ws, st)
}

func (s *Syncer) setLocked(node, ws string, st State) {
	if s.state == nil {
		s.state = map[string]map[string]State{}
	}
	if s.state[node] == nil {
		s.state[node] = map[string]State{}
	}
	s.state[node][ws] = st
}

// Holds is whether node has cloned the repo of the workspace keyed ws.
func (s *Syncer) Holds(node, ws string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state[node][ws].State == Cloned
}

// Of is node's projects as node.status shows them, by workspace key; nil for a node never synced since the master started.
func (s *Syncer) Of(node string) []any {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.state[node]
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		st := m[k]
		row := map[string]any{"workspace": k, "state": st.State}
		if st.Error != "" {
			row["error"] = st.Error
		}
		out = append(out, row)
	}
	return out
}

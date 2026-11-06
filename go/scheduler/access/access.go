// Package access is which paths an agent's session may read without asking, and the founder's grants that add to them (contract x-access).
package access

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/worktreepaths"
)

// Grant is a path the founder let an agent read. After is the agent's last session_starts id when it was granted: a once grant is the
// first fresh session's started after it, and is gone once a second has started.
type Grant struct {
	Path  string `json:"path"`
	Scope string `json:"scope"`
	At    int64  `json:"at"`
	After int64  `json:"after,omitempty"`
}

func key(agent string) string { return "access:" + agent }

// fresh is how many sessions of the agent started fresh, not resumed, after session_starts id after.
func fresh(q db.Q, agent string, after int64) (int, error) {
	var n int
	err := q.QueryRow("SELECT count(*) FROM session_starts WHERE agent = ? AND id > ? AND resumed = 0", agent, after).Scan(&n)
	return n, err
}

// stored is every grant kept for the agent, a used-up once grant too.
func stored(q db.Q, agent string) ([]Grant, error) {
	v, ok, err := db.Meta(q, key(agent))
	if err != nil || !ok || v == "" {
		return []Grant{}, err
	}
	var gs []Grant
	return gs, json.Unmarshal([]byte(v), &gs)
}

// Grants is the agent's grants in effect, newest first.
func Grants(q db.Q, agent string) ([]Grant, error) {
	gs, err := stored(q, agent)
	if err != nil {
		return nil, err
	}
	out := gs[:0:0]
	for _, g := range gs {
		if g.Scope == "once" {
			if n, err := fresh(q, agent, g.After); err != nil {
				return nil, err
			} else if n >= 2 {
				continue
			}
		}
		out = append(out, g)
	}
	return out, nil
}

func save(q db.Q, agent string, gs []Grant) error {
	sort.SliceStable(gs, func(i, j int) bool { return gs[i].At > gs[j].At })
	b, err := json.Marshal(gs)
	if err != nil {
		return err
	}
	return db.SetMeta(q, key(agent), string(b))
}

// Put grants path with scope, replacing an earlier grant of the same path, and drops the used-up ones. q is the write's transaction.
// A once grant is the next session's.
func Put(q db.Q, agent, path, scope string, now int64) (Grant, error) {
	gs, err := Grants(q, agent)
	if err != nil {
		return Grant{}, err
	}
	g := Grant{Path: path, Scope: scope, At: now}
	if err := q.QueryRow("SELECT coalesce(max(id), 0) FROM session_starts WHERE agent = ?", agent).Scan(&g.After); err != nil {
		return Grant{}, err
	}
	return g, save(q, agent, append(without(gs, path), g))
}

// lastFresh is the id of the agent's last fresh session start, 0 for none.
func lastFresh(q db.Q, agent string) (int64, error) {
	var id int64
	err := q.QueryRow("SELECT coalesce(max(id), 0) FROM session_starts WHERE agent = ? AND resumed = 0", agent).Scan(&id)
	return id, err
}

// Running is whether a session start of the agent is on record, one a grant can be given to now.
func Running(q db.Q, agent string) (bool, error) {
	id, err := lastFresh(q, agent)
	return id > 0, err
}

// GivenNow records that the running session was given path: a once grant of it is then that session's, not the next one's.
func GivenNow(q db.Q, agent, path string) error {
	id, err := lastFresh(q, agent)
	if err != nil || id == 0 {
		return err
	}
	gs, err := stored(q, agent)
	if err != nil {
		return err
	}
	for i := range gs {
		if gs[i].Path == path && gs[i].Scope == "once" {
			gs[i].After = id - 1
		}
	}
	return save(q, agent, gs)
}

// Remove takes back the grant of path, and says whether one was in effect. q is the write's transaction.
func Remove(q db.Q, agent, path string) (bool, error) {
	gs, err := Grants(q, agent)
	if err != nil {
		return false, err
	}
	kept := without(gs, path)
	return len(kept) < len(gs), save(q, agent, kept)
}

func without(gs []Grant, path string) []Grant {
	out := make([]Grant, 0, len(gs))
	for _, g := range gs {
		if g.Path != path {
			out = append(out, g)
		}
	}
	return out
}

// ForLaunch is the granted paths a session of the agent starts with, read before its start is recorded: a fresh session takes a once
// grant no session has started after yet; a resumed one is the session that took it.
func ForLaunch(q db.Q, agent string, resuming bool) ([]string, error) {
	gs, err := Grants(q, agent)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, g := range gs {
		if g.Scope == "once" && !resuming {
			if n, err := fresh(q, agent, g.After); err != nil {
				return nil, err
			} else if n > 0 {
				continue
			}
		}
		out = append(out, g.Path)
	}
	return out, nil
}

// Roots is every path the agent's sessions may read: its worktrees, its agent folder, the scratch root, the attachments folder and its grants.
func Roots(q db.Q, home, agent string) ([]string, error) {
	roots := []string{paths.AgentDir(home, agent), paths.ScratchRoot(), filepath.Join(home, "attachments")}
	row, err := db.GetAgentState(q, agent)
	if err != nil {
		return nil, err
	}
	if row != nil {
		if row.Worktree.Valid && row.Worktree.String != "" {
			roots = append(roots, row.Worktree.String)
		}
		roots = append(roots, worktreepaths.ExtraPaths(row)...)
	}
	gs, err := Grants(q, agent)
	if err != nil {
		return nil, err
	}
	for _, g := range gs {
		roots = append(roots, g.Path)
	}
	return roots, nil
}

// Outside is each of refs no root holds, in order. Both sides are taken where their links lead, so a link in the worktree to /etc is
// outside, and /tmp is /private/tmp.
func Outside(roots, refs []string) []string {
	var out []string
	for _, p := range refs {
		in := false
		for _, r := range roots {
			if under(paths.Realpath(p), paths.Realpath(r)) {
				in = true
				break
			}
		}
		if !in {
			out = append(out, p)
		}
	}
	return out
}

// under is whether p is root or inside it; a grant of / holds everything.
func under(p, root string) bool {
	root = strings.TrimRight(root, "/")
	return p == root || strings.HasPrefix(p, root+"/")
}

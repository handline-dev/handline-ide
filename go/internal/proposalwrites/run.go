package proposalwrites

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/kit/paths"
)

// Carrying out a proposal: one change, or every step of a team diff, in order. A step that is refused or raises puts back everything the steps
// before it did, and nothing of the diff stays.

// snapshot is what a change to the roster can touch, kept so a step that fails puts all of it back: the routing table and its backup, the
// config, the folder of each agent a step names (and what lands in the archive), the seals kept for them in the protected store.
// The database is put back by the transaction the steps run in (undo).
type snapshot struct {
	home     string
	dir      string
	files    map[string][]byte // nil: the file was not there
	dirs     []string          // each named agent's folder
	folders  map[string]string // folder -> its copy
	archived map[string]bool
}

func takeSnapshot(home string, names map[string]bool) (*snapshot, error) {
	dir, err := os.MkdirTemp(home, "proposal-snapshot-")
	if err != nil {
		return nil, err
	}
	s := &snapshot{home: home, dir: dir, files: map[string][]byte{}, folders: map[string]string{}, archived: map[string]bool{}}
	for _, p := range []string{paths.ConfigFile(home)} {
		if b, err := os.ReadFile(p); err == nil {
			s.files[p] = b
		} else {
			s.files[p] = nil
		}
	}
	for n := range names {
		s.dirs = append(s.dirs, paths.AgentDir(home, n))
	}
	sortStrings(s.dirs)
	for i, d := range s.dirs {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			to := filepath.Join(dir, fmt.Sprint(i))
			if err := copyTree(d, to); err != nil {
				s.drop()
				return nil, err
			}
			s.folders[d] = to
		}
	}
	if entries, err := os.ReadDir(filepath.Join(home, "agents", "archive")); err == nil {
		for _, c := range entries {
			s.archived[c.Name()] = true
		}
	}
	return s, nil
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// copyTree copies a folder with its links as links.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(target, dst)
		case d.IsDir():
			return os.MkdirAll(dst, info.Mode().Perm()|0o700)
		default:
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(dst, b, info.Mode().Perm())
		}
	})
}

func (s *snapshot) restore() {
	for p, data := range s.files {
		cur, err := os.ReadFile(p)
		switch {
		case data == nil:
			os.Remove(p)
		case err != nil || string(cur) != string(data):
			os.WriteFile(p, data, 0o644)
		}
	}
	archive := filepath.Join(s.home, "agents", "archive")
	if entries, err := os.ReadDir(archive); err == nil {
		for _, c := range entries {
			if !s.archived[c.Name()] {
				os.RemoveAll(filepath.Join(archive, c.Name()))
			}
		}
	}
	for _, d := range s.dirs {
		os.RemoveAll(d)
		if from, ok := s.folders[d]; ok {
			copyTree(from, d)
		}
	}
}

// undo rolls tx back and puts the files back. The agents tx added are gone with it, and the next agent made would be given one of
// their ids, so their folders go first, while tx still holds the write lock and no other add can take the id.
func (s *snapshot) undo(tx *sql.Tx, committed *sql.DB) {
	added, _ := addedAgents(tx, committed)
	s.removeFolders(added)
	tx.Rollback()
	s.restore()
}

// addedAgents are the keys of the agent rows tx has that committed has not.
func addedAgents(tx *sql.Tx, committed *sql.DB) ([]string, error) {
	ids := func(q interface {
		Query(string, ...any) (*sql.Rows, error)
	}) (map[string]bool, error) {
		rows, err := q.Query("SELECT 'a' || id FROM agents")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := map[string]bool{}
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				return nil, err
			}
			out[k] = true
		}
		return out, rows.Err()
	}
	mine, err := ids(tx)
	if err != nil {
		return nil, err
	}
	theirs, err := ids(committed)
	if err != nil {
		return nil, err
	}
	var out []string
	for k := range mine {
		if !theirs[k] {
			out = append(out, k)
		}
	}
	return out, nil
}

func (s *snapshot) removeFolders(keys []string) {
	for _, k := range keys {
		os.RemoveAll(paths.AgentDir(s.home, k))
		paths.RemoveCacheDir(s.home, k)
	}
}

func (s *snapshot) drop() { os.RemoveAll(s.dir) }

// namesOf are the agents whose folders and grants a step may change.
func namesOf(items []*Prop) map[string]bool {
	out := map[string]bool{}
	for _, it := range items {
		if it.kind() == "global" {
			continue // its name is a switch, and config.yaml is kept whole
		}
		out[it.str("agent")] = true
		if it.kind() == "rename" {
			out[it.str("to")] = true
		}
		if it.kind() == "team" {
			for _, m := range it.members() {
				out[m] = true
			}
		}
	}
	return out
}

// step is what a doer is given: the change, the transaction the database changes are made in, and what to do after the last step.
type step struct {
	*Prop
	tx    *sql.Tx
	after *[]func() string
}

// later runs fn (the terminal host's) after the last step, which has the database locked until then.
func (s step) later(fn func() string) string {
	*s.after = append(*s.after, fn)
	return ""
}

// hostServed refuses a change that needs a terminal host this daemon does not reach: only tmux, which keeps no repos or labels, is served.
func (e *Env) hostServed(items []*Prop) string {
	for _, it := range items {
		switch it.kind() {
		case "add", "team", "rename":
			if name := agenthost.TerminalName(e.Home); name != "tmux" {
				return fmt.Sprintf("the terminal host %s is not served by this daemon yet: nothing was changed", name)
			}
		}
	}
	return ""
}

func (e *Env) doer(kind string) func(*Env, step) (bool, string, error) {
	return map[string]func(*Env, step) (bool, string, error){
		"add": (*Env).doAdd, "settings": (*Env).doSettings, "context": (*Env).doContext, "retire": (*Env).doRetire,
		"rename": (*Env).doRename, "team": (*Env).doTeam,
		"schedule": (*Env).doSchedule, "global": (*Env).doGlobal,
	}[kind]
}

// stepsOf are a proposal's changes in the order they are applied: its own, or each of a team diff's.
func stepsOf(prop *Prop) []*Prop {
	if prop.kind() == bundle {
		return ordered(prop.items())
	}
	return []*Prop{prop}
}

// run carries out a proposal. A step that is refused or fails puts back everything the steps before it did; a diff says which step and why.
func (e *Env) run(prop *Prop) (bool, string, error) {
	isBundle := prop.kind() == bundle
	items := stepsOf(prop)
	if why := e.hostServed(items); why != "" {
		return false, why, nil
	}
	snap, err := takeSnapshot(e.Home, namesOf(items))
	if err != nil {
		return false, "", err
	}
	defer snap.drop()
	tx, err := e.DB.Begin()
	if err != nil {
		return false, "", err
	}
	var after []func() string
	var said []string
	fail := func(err error) (bool, string, error) {
		snap.undo(tx, e.DB)
		return false, "", err
	}
	for n, item := range items {
		cur := item.clone()
		if isBundle {
			cur.set("id", prop.id())
			cur.set("requested_by", prop.get("requested_by"))
			cur.set("why", prop.get("why"))
			cur.set("created", prop.get("created"))
		}
		doer := e.doer(item.kind())
		if doer == nil {
			return fail(fmt.Errorf("no applier for %s", item.kind()))
		}
		ok, text, err := doer(e, step{cur, tx, &after})
		if err != nil {
			return fail(err)
		}
		if !ok {
			snap.undo(tx, e.DB)
			if !isBundle {
				return false, text, nil
			}
			return false, fmt.Sprintf("step %d of %d (%s %s) failed: %s Nothing was changed: the steps before it were put back.",
				n+1, len(items), item.kind(), item.str("agent"), text), nil
		}
		said = append(said, text)
	}
	added, err := addedAgents(tx, e.DB)
	if err != nil {
		return fail(err)
	}
	if err := tx.Commit(); err != nil {
		snap.removeFolders(added)
		snap.restore()
		return false, "", err
	}
	var notes string
	for _, fn := range after {
		notes += fn()
	}
	if !isBundle {
		return true, said[0] + notes, nil
	}
	lines := make([]string, len(said))
	for i, t := range said {
		lines[i] = fmt.Sprintf("  %d. %s", i+1, t)
	}
	return true, fmt.Sprintf("team diff applied, %d changes:\n%s%s", len(items), strings.Join(lines, "\n"), notes), nil
}

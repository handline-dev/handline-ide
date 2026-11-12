package team

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/paths"
)

// NotGit is a workspace folder that is no git checkout: the founder may make it one (git init) or name another.
type NotGit struct{ Folder string }

func (e *NotGit) Error() string {
	return fmt.Sprintf("%s is not in a git checkout: a workspace's folder is one (run git init there, or choose another folder)", e.Folder)
}

// Toplevel is the top folder of the git checkout folder is in; *NotGit when it is in none.
func Toplevel(folder string) (string, error) {
	path := expandUser(folder)
	if !filepath.IsAbs(path) {
		return "", refuse("a workspace's folder is an absolute path, not %s", pyQuote(folder))
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return "", refuse("%s is not a folder", path)
	}
	r := gitrun.Run(path, "rev-parse", "--show-toplevel")
	if r.RC != 0 || strings.TrimSpace(r.Stdout) == "" {
		return "", &NotGit{path}
	}
	top := filepath.Clean(strings.TrimSpace(r.Stdout))
	if real, err := filepath.EvalSymlinks(path); err == nil && real == top { // the top itself, kept as it was written (/tmp, not /private/tmp)
		return filepath.Clean(path), nil
	}
	return top, nil
}

// createWorkspace makes workspace name's row, its folder the top of the git checkout folder is in. A folder inside another workspace's,
// or holding one, is refused: a checkout is one workspace's. An empty folder is a workspace with none yet.
func createWorkspace(q db.Q, name, folder string) (int64, string, error) {
	if err := names.Check(q, names.Workspace, name, 0); err != nil {
		return 0, "", err
	}
	if folder != "" {
		top, err := Toplevel(folder)
		if err != nil {
			return 0, "", err
		}
		if other, at, err := sharing(q, top, 0); err != nil {
			return 0, "", err
		} else if other != "" {
			return 0, "", refuse("%s is workspace %s's folder %s, or shares it: a checkout is one workspace's", top, other, at)
		}
		folder = top
	}
	res, err := q.Exec("INSERT INTO workspaces(name, folder, created_at) VALUES(?, ?, ?)", name, folder, time.Now().Unix())
	if err != nil {
		return 0, "", err
	}
	id, err := res.LastInsertId()
	return id, folder, err
}

// sharing is the workspace other than id whose folder is top, holds it or is under it, and that folder; "" when none. The rows are
// read out and closed before the caller writes in the same transaction.
func sharing(q db.Q, top string, id int64) (string, string, error) {
	rows, err := q.Query("SELECT name, folder FROM workspaces WHERE folder != '' AND id != ?", id)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()
	for rows.Next() {
		var name, at string
		if err := rows.Scan(&name, &at); err != nil {
			return "", "", err
		}
		if at = toplevelOr(at); within(top, at) || within(at, top) {
			return name, at, nil
		}
	}
	return "", "", rows.Err()
}

// toplevelOr is folder's checkout top, or folder itself when it is in none: a folder recorded before the check may be either.
func toplevelOr(folder string) string {
	if top, err := Toplevel(folder); err == nil {
		return top
	}
	return folder
}

// within is whether path is dir or under it.
func within(path, dir string) bool {
	dir = filepath.Clean(expandUser(dir))
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// Create makes workspace name on folder (the top of its git checkout) with its lead, who reports to the owner, and the workspace's
// base.md. Returns the workspace's key.
func (r *Roster) Create(name, folder string) (string, error) {
	if err := requireFounder(); err != nil {
		return "", err
	}
	if r.err != nil {
		return "", r.err
	}
	if folder == "" {
		return "", refuse("a workspace needs its folder")
	}
	var ws int64
	err := inTx(r.DB, func(q db.Q) error {
		var err error
		if ws, _, err = createWorkspace(q, name, folder); err != nil {
			return err
		}
		return r.register(q, name, RegisterOpts{}, "") // a bare name is the workspace's lead
	})
	if err != nil {
		return "", err
	}
	if _, err := r.folders(name, RegisterOpts{}); err != nil {
		return "", err
	}
	key := actor.Key(actor.Workspace, ws)
	return key, writeRules(r.Home, key, name)
}

// writeRules writes the workspace's base.md when it has none.
func writeRules(home, key, name string) error {
	p := filepath.Join(paths.WorkspaceDir(home, key), "base.md")
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte("# base - "+name+"\n\n(the team's rules: injected into every session of the workspace, so it is fixed overhead.)\n"), 0o644)
}

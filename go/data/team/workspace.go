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
	if r.RC != 0 && strings.Contains(r.Stderr, "not a git repository") {
		return "", &NotGit{path}
	}
	if r.RC != 0 || strings.TrimSpace(r.Stdout) == "" { // git missing, timed out or refusing the folder: a failure, not an answer
		return "", fmt.Errorf("git rev-parse in %s: %s", path, strings.TrimSpace(r.Stderr))
	}
	top := filepath.Clean(strings.TrimSpace(r.Stdout))
	if real(path) == top { // the top itself, kept as it was written (/tmp, not /private/tmp)
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
		if err := sharing(q, top, 0); err != nil {
			return 0, "", err
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

// sharing refuses top when the folder of a workspace other than id is top, holds it or is under it. Its rows are closed before it
// returns, so the caller may write in the same transaction.
func sharing(q db.Q, top string, id int64) error {
	rows, err := q.Query("SELECT name, folder FROM workspaces WHERE folder != '' AND id != ?", id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, at string
		if err := rows.Scan(&name, &at); err != nil {
			return err
		}
		if within(real(top), real(at)) || within(real(at), real(top)) {
			return refuse("%s is workspace %s's folder %s, or shares it: a checkout is one workspace's", top, name, at)
		}
	}
	return rows.Err()
}

// real is folder with its symlinks followed, or as written when it cannot be: /tmp and /private/tmp are one folder.
func real(folder string) string {
	folder = filepath.Clean(expandUser(folder))
	if r, err := filepath.EvalSymlinks(folder); err == nil {
		return r
	}
	return folder
}

// within is whether path is dir or under it.
func within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// Create makes workspace name on folder (the top of its git checkout) with its lead, who reports to the owner, and seeds its rules as
// a launch would. Returns the workspace's key. Should the folders fail after the rows are in, Register(name) makes them.
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
		if at, err := find(q, name); err != nil {
			return err
		} else if at.ws != 0 {
			return refuse("workspace %s exists already", name)
		}
		var err error
		ws, err = r.register(q, name, RegisterOpts{Repo: folder}, "") // a bare name is the workspace's lead
		return err
	})
	if err != nil {
		return "", err
	}
	if _, err := r.folders(name, RegisterOpts{}); err != nil {
		return "", err
	}
	key := actor.Key(actor.Workspace, ws)
	return key, seedWorkspace(r.Home, key)
}

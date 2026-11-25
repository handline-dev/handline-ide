package team

import (
	"encoding/json"
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

// Folder is folder as a workspace keeps it: the top of the git checkout it is in, or, in none, the plain folder itself (git false).
func Folder(folder string) (kept string, git bool, err error) {
	path := expandUser(folder)
	if !filepath.IsAbs(path) {
		return "", false, refuse("a workspace's folder is an absolute path, not %s", pyQuote(folder))
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return "", false, refuse("%s is not a folder", path)
	}
	r := gitrun.Run(path, "rev-parse", "--show-toplevel")
	if r.RC != 0 && strings.Contains(r.Stderr, "not a git repository") {
		return filepath.Clean(path), false, nil
	}
	if r.RC != 0 || strings.TrimSpace(r.Stdout) == "" { // git missing, timed out or refusing the folder: a failure, not an answer
		return "", false, fmt.Errorf("git rev-parse in %s: %s", path, strings.TrimSpace(r.Stderr))
	}
	top := filepath.Clean(strings.TrimSpace(r.Stdout))
	if real(path) == top { // the top itself, kept as it was written (/tmp, not /private/tmp)
		return filepath.Clean(path), true, nil
	}
	return top, true, nil
}

// IsGit is whether a folder a workspace keeps is a git checkout's top.
func IsGit(folder string) bool {
	_, err := os.Stat(filepath.Join(expandUser(folder), ".git"))
	return err == nil
}

// Folders are workspace ws's folders: its default (folder) first, then the ones added. A workspace with no folder has none.
func Folders(q db.Q, ws int64) ([]string, error) {
	var folder, added string
	if err := q.QueryRow("SELECT folder, added_folders FROM workspaces WHERE id = ?", ws).Scan(&folder, &added); err != nil {
		return nil, err
	}
	return foldersOf(folder, added)
}

func foldersOf(folder, added string) ([]string, error) {
	var more []string
	if err := json.Unmarshal([]byte(added), &more); err != nil {
		return nil, fmt.Errorf("added_folders: %w", err)
	}
	if folder == "" {
		return more, nil
	}
	return append([]string{folder}, more...), nil
}

// setFolder makes folder the one agent at works in. One of its workspace's folders becomes the agent's own (NULL for the default);
// any other replaces the workspace's default folder, which only its lead may change, and the lead works there. "" clears the default
// of a workspace that has no other folder.
func setFolder(q db.Q, at row, folder string) error {
	if works := at.works(); folder == works || (folder != "" && works != "" && real(folder) == real(works)) {
		return nil // unchanged: a settings save sends the folder back as it is
	}
	var fs []string
	if at.ws != 0 {
		var err error
		if fs, err = Folders(q, at.ws); err != nil {
			return err
		}
	}
	if folder == "" {
		if len(fs) > 1 {
			return refuse("workspace %s has other folders: take its default away with workspace.set instead", at.wsName)
		}
	} else {
		kept, _, err := Folder(folder)
		if err != nil {
			return err
		}
		folder = kept
		for i, f := range fs {
			if real(f) == real(folder) && at.id != 0 {
				var own any
				if i > 0 {
					own = f
				}
				_, err := q.Exec("UPDATE agents SET folder = ? WHERE id = ?", own, at.id)
				return err
			}
		}
		if !at.lead && at.folder != "" {
			return refuse("%s is not a folder of workspace %s: add it to the workspace, or set the default on its lead", folder, at.wsName)
		}
		if err := sharing(q, folder, at.ws); err != nil {
			return err
		}
	}
	if _, err := q.Exec("UPDATE workspaces SET folder = ? WHERE id = ?", folder, at.ws); err != nil {
		return err
	}
	_, err := q.Exec("UPDATE agents SET folder = NULL WHERE id = ?", at.id)
	return err
}

// AddFolder adds folder, kept as Folder keeps it (the caller resolves it first, outside its transaction), to workspace ws (its key),
// after its others; one it has already is left.
func (r *Roster) AddFolder(ws, folder string) error {
	id, err := r.workspaceID(ws)
	if err != nil {
		return err
	}
	return inTx(r.DB, func(q db.Q) error {
		fs, err := Folders(q, id)
		if err != nil {
			return err
		}
		for _, f := range fs {
			if real(f) == real(folder) {
				return nil
			}
		}
		if err := sharing(q, folder, id); err != nil {
			return err
		}
		if len(fs) == 0 {
			_, err = q.Exec("UPDATE workspaces SET folder = ? WHERE id = ?", folder, id)
			return err
		}
		return setAdded(q, id, append(fs[1:], folder))
	})
}

// RemoveFolder takes folder from workspace ws (its key). Refused while an agent works in it; the default taken, the first added one is
// the default.
func (r *Roster) RemoveFolder(ws, folder string) error {
	id, err := r.workspaceID(ws)
	if err != nil {
		return err
	}
	return inTx(r.DB, func(q db.Q) error {
		fs, err := Folders(q, id)
		if err != nil {
			return err
		}
		at := -1
		for i, f := range fs {
			if real(f) == real(folder) {
				at = i
			}
		}
		if at < 0 {
			return refuse("%s is not a folder of this workspace", folder)
		}
		var n int
		cond := "folder = ?"
		if at == 0 {
			cond = "(folder IS NULL OR folder = ?)"
		}
		if err := q.QueryRow("SELECT COUNT(*) FROM agents WHERE workspace = ? AND "+cond, id, fs[at]).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return refuse("%d of its agents work in %s: give them another folder first", n, fs[at])
		}
		rest := append(append([]string{}, fs[:at]...), fs[at+1:]...)
		def := ""
		if len(rest) > 0 {
			def, rest = rest[0], rest[1:]
		}
		if _, err := q.Exec("UPDATE workspaces SET folder = ? WHERE id = ?", def, id); err != nil {
			return err
		}
		return setAdded(q, id, rest)
	})
}

func setAdded(q db.Q, ws int64, added []string) error {
	if added == nil {
		added = []string{}
	}
	b, _ := json.Marshal(added) // a []string always marshals
	_, err := q.Exec("UPDATE workspaces SET added_folders = ? WHERE id = ?", string(b), ws)
	return err
}

// workspaceID is the id of workspace ws (its key); the founder's session only.
func (r *Roster) workspaceID(ws string) (int64, error) {
	if err := requireFounder(); err != nil {
		return 0, err
	}
	if r.err != nil {
		return 0, r.err
	}
	k, id, ok := actor.Parse(ws)
	if !ok || k != actor.Workspace {
		return 0, fmt.Errorf("%s is no workspace key", ws)
	}
	var n int
	if err := r.DB.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", id).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, &NotRegistered{ws}
	}
	return id, nil
}

// createWorkspace makes workspace name's row on folder, kept as Folder keeps it. A folder inside another workspace's, or holding one,
// is refused: a folder is one workspace's. An empty folder is a workspace with none yet.
func createWorkspace(q db.Q, name, folder string) (int64, string, error) {
	if err := names.Check(q, names.Workspace, name, 0); err != nil {
		return 0, "", err
	}
	if folder != "" {
		kept, _, err := Folder(folder)
		if err != nil {
			return 0, "", err
		}
		if err := sharing(q, kept, 0); err != nil {
			return 0, "", err
		}
		folder = kept
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
	rows, err := q.Query("SELECT name, folder, added_folders FROM workspaces WHERE id != ?", id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, folder, added string
		if err := rows.Scan(&name, &folder, &added); err != nil {
			return err
		}
		fs, err := foldersOf(folder, added)
		if err != nil {
			return err
		}
		for _, at := range fs {
			if within(real(top), real(at)) || within(real(at), real(top)) {
				return refuse("%s is workspace %s's folder %s, or shares it: a folder is one workspace's", top, name, at)
			}
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

// Create makes workspace name on folder (as Folder keeps it) with its lead, the workspace's contact, and seeds its rules as
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
	return key, seedWorkspace(r.DB, key, time.Now().Unix())
}

// RenameWorkspace gives workspace ws (its key) the name name. Its agents' dotted names follow it; their keys, folders and rows stay.
func (r *Roster) RenameWorkspace(ws, name string) error {
	id, err := r.workspaceID(ws)
	if err != nil {
		return err
	}
	return inTx(r.DB, func(q db.Q) error {
		var was string
		if err := q.QueryRow("SELECT name FROM workspaces WHERE id = ?", id).Scan(&was); err != nil || was == name {
			return err
		}
		if err := names.Check(q, names.Workspace, name, 0); err != nil {
			return err
		}
		_, err := q.Exec("UPDATE workspaces SET name = ? WHERE id = ?", name, id)
		return err
	})
}

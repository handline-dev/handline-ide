package sessionrpc

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/kit/wslock"
	"github.com/handline/handline/go/kit/yamlcompat"
	"github.com/handline/handline/go/protocol/rpc"
)

const workspaceCreateKind = "workspace.create"

// workspaceCreate is workspace.create: a workspace on a folder (the top of its git checkout, or a plain folder) and its lead, started there, as one
// operation like agent.create's. What can be told at once (the name, the folder, a workspace of that name) is refused before any
// operation starts; the rest is the operation's. A name not given comes from the folder's (else "workspace"), the first one free; a
// folder not given is made under paths.Projects, named as the workspace.
func (e *Env) workspaceCreate(c *rpc.Conn, params map[string]any) (any, error) {
	name, err := createText(params, "name", false)
	if err != nil {
		return nil, err
	}
	if name != "" && (strings.ContainsAny(name, "./") || textsafe.HasUnsafe(name) || !yamlcompat.IsPlainText(name)) {
		return nil, bad("name is a workspace's name: plain text with no dot or slash", "name")
	}
	folder, err := createText(params, "folder", false)
	if err != nil {
		return nil, err
	}
	top := ""
	if folder != "" {
		if top, _, err = team.Folder(folder); err != nil {
			if team.IsRefusal(err) {
				return nil, notFound("folder", folder, err.Error())
			}
			return nil, err
		}
	}
	if name == "" {
		from := "workspace"
		if w := names.From(filepath.Base(top)); top != "" && w != "" {
			from = w
		}
		if name, err = names.Free(e.DB, names.Workspace, 0, from); err != nil {
			return nil, err
		}
	} else if ws, err := paths.WorkspaceKeyIn(e.DB, name); err != nil {
		return nil, err
	} else if ws != "" {
		return nil, rpc.Refused("workspace "+name+" exists already", "state", "")
	} else if err := names.Check(e.DB, names.Workspace, name, 0); err != nil {
		if _, refused := errors.AsType[*names.Refusal](err); refused {
			return nil, bad(err.Error(), "name")
		}
		return nil, err
	}
	made := false
	if top == "" {
		if top, made, err = e.madeFolder(name); err != nil {
			return nil, err
		}
	}
	res, err := e.operate(c, workspaceCreateKind, name, top, "", func() error {
		if _, err := e.Roster.Create(name, top); err != nil {
			return err
		}
		if e.WorkspaceAdded != nil {
			e.WorkspaceAdded(name)
		}
		return nil
	})
	if err != nil && made {
		os.Remove(top) // the empty folder this call made, as nothing took it; Remove takes no folder with anything in it
	}
	return res, err
}

// madeFolder is a new workspace's folder under paths.Projects: <name>, or <name>-2 and so on past one that holds anything or is
// a workspace's folder already; made says it was made here. Projects inside a git checkout is refused: the checkout's top would be
// the workspace's folder, not the one made.
func (e *Env) madeFolder(name string) (top string, made bool, err error) {
	projects := paths.Projects()
	if projects == "" {
		return "", false, bad("this Mac user has no home folder to make a workspace's folder in: give folder", "folder")
	}
	for i := 1; i < 1000; i++ {
		dir := filepath.Join(projects, name)
		if i > 1 {
			dir = fmt.Sprintf("%s-%d", dir, i)
		}
		if ents, err := os.ReadDir(dir); err == nil && len(ents) > 0 {
			continue
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			continue // a file, or a folder it cannot read
		}
		var held int
		if err := e.DB.QueryRow("SELECT COUNT(*) FROM workspaces WHERE folder = ? OR instr(added_folders, ?) > 0", dir, dir).Scan(&held); err != nil {
			return "", false, err
		} else if held > 0 {
			continue
		}
		_, statErr := os.Stat(dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", false, rpc.Refused("could not make "+dir+": "+err.Error(), "state", "")
		}
		made = errors.Is(statErr, os.ErrNotExist)
		top, _, err := team.Folder(dir)
		if err == nil && top != filepath.Clean(dir) {
			err = rpc.Refused(projects+" is inside the git checkout "+top+": give folder", "state", "")
		}
		if err != nil && made {
			os.Remove(dir)
		}
		return top, made, err
	}
	return "", false, rpc.Refused("no free folder for "+name+" in "+projects, "state", "")
}

// useWorkspace holds the workspace agent is in (or would be: a new agent's workspace/name, a new workspace's name) against its delete while a call starts or adds
// an agent there. A workspace that does not exist yet needs no hold.
func (e *Env) useWorkspace(agent string) (release func(), err error) {
	var ws string
	if e.DB == nil {
		return func() {}, nil
	}
	if k, id, ok := actor.Parse(agent); ok && k == actor.Agent {
		var n int64
		if err := e.DB.QueryRow("SELECT workspace FROM agents WHERE id = ?", id).Scan(&n); err == nil {
			ws = actor.Key(actor.Workspace, n)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	} else {
		wsName, _, _ := strings.Cut(agent, "/")
		if ws, err = paths.WorkspaceKeyIn(e.DB, wsName); err != nil {
			return nil, err
		}
	}
	if ws == "" {
		return func() {}, nil
	}
	release, ok := wslock.Use(e.Home, ws)
	if !ok {
		return nil, rpc.Refused("the workspace of "+agent+" is being deleted", "state", "")
	}
	return release, nil
}

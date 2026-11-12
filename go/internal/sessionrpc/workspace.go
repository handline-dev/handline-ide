package sessionrpc

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/kit/wslock"
	"github.com/handline/handline/go/kit/yamlcompat"
	"github.com/handline/handline/go/protocol/rpc"
)

const workspaceCreateKind = "workspace.create"

// workspaceCreate is workspace.create: a workspace on a folder (the top of its git checkout) and its lead, started there, as one
// operation like agent.create's. What can be told at once (the name, the folder, a workspace of that name) is refused before any
// operation starts; the rest is the operation's.
func (e *Env) workspaceCreate(c *rpc.Conn, params map[string]any) (any, error) {
	name, err := createText(params, "name", true)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(name, "./") || textsafe.HasUnsafe(name) || !yamlcompat.IsPlainText(name) {
		return nil, bad("name is a workspace's name: plain text with no dot or slash", "name")
	}
	folder, err := createText(params, "folder", true)
	if err != nil {
		return nil, err
	}
	top, err := team.Toplevel(folder)
	if err != nil {
		var ng *team.NotGit
		switch {
		case errors.As(err, &ng):
			return nil, rpc.Refused(err.Error(), "not_git", "")
		case team.IsRefusal(err):
			return nil, notFound("folder", folder, err.Error())
		}
		return nil, err
	}
	if ws, err := paths.WorkspaceKeyIn(e.DB, name); err != nil {
		return nil, err
	} else if ws != "" {
		return nil, rpc.Refused("workspace "+name+" exists already", "state", "")
	}
	return e.operate(c, workspaceCreateKind, name, top, "", func() error {
		_, err := e.Roster.Create(name, top)
		return err
	})
}

// useWorkspace holds the workspace agent is in (or would be, by its dotted name) against its delete while a call starts or adds
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
		wsName, _ := team.SplitLegacy(agent)
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

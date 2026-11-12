package sessionrpc

import (
	"errors"
	"strings"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/textsafe"
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
	if paths.AgentKey(e.Home, name) != "" {
		return nil, rpc.Refused("workspace "+name+" exists already", "state", "")
	}
	return e.operate(c, workspaceCreateKind, name, top, "", func() error {
		_, err := e.Roster.Create(name, top)
		return err
	})
}

package main

import (
	"fmt"
	"path/filepath"

	"github.com/handline/handline/go/internal/hlclient"
)

// workspaceCmd is the founder's `hl workspace new|rename|delete`. An agent is pointed to a proposal: workspaces are the founder's.
func workspaceCmd(c *hlclient.Client, args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	sub, rest := args[0], args[1:]
	var method string
	params := map[string]any{}
	switch sub {
	case "new":
		p, ok := parse(rest, []string{"--folder"}, nil, nil)
		if !ok || len(p.pos) != 1 {
			return 0, false
		}
		folder, _ := p.opt("--folder")
		abs, err := filepath.Abs(expandHome(folder)) // the daemon reads it from its own folder: as typed here, none is here
		if err != nil {
			return wrongArgs("--folder %s cannot be read: %v", folder, err)
		}
		method, params["name"], params["folder"] = "workspace.create", p.pos[0], abs
	case "rename":
		p, ok := parse(rest, []string{"--to"}, nil, nil)
		if !ok || len(p.pos) != 1 {
			return 0, false
		}
		to, has := p.opt("--to")
		if !has || to == "" {
			return wrongArgs("--to is missing: the new name")
		}
		method, params["workspace"], params["name"] = "workspace.set", p.pos[0], to
	case "delete":
		p, ok := parse(rest, []string{"--worktree"}, nil, nil)
		if !ok || len(p.pos) != 1 {
			return 0, false
		}
		if w, has := p.opt("--worktree"); has {
			if w != "save" && w != "force" {
				return wrongArgs("--worktree is save (commit and keep each worktree's work on its branch) or force (drop it)")
			}
			params["worktree"] = w
		}
		method, params["workspace"] = "workspace.delete", p.pos[0]
	default:
		return 0, false
	}
	if hlclient.Agent() != "" {
		fmt.Println("workspaces are the founder's: propose one with `hl propose team <name> --member <name> ... --repo PATH --why \"<why>\"`, or ask the founder to rename or delete one")
		return 1, true
	}
	c.NoteVerbChain([]string{"workspace", sub})
	ok, text, got := c.Written(method, params)
	if ok && sub == "new" {
		text = fmt.Sprintf("workspace %s is being made in %s and its lead started (operation %s)", params["name"], params["folder"], line(got["operation"]))
	}
	return said(ok, text), true
}

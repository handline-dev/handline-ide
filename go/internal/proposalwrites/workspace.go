package proposalwrites

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/worktreeprune"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/wslock"
	"github.com/handline/handline/go/protocol/rpc"
)

// workspaceAt is the workspace a call names by its key or its name: its key, name and agents' keys, its lead last.
type workspaceAt struct {
	key, name string
	agents    []string
}

func (e *Env) workspaceOf(params map[string]any) (workspaceAt, error) {
	who, _ := params["workspace"].(string)
	if strings.TrimSpace(who) == "" {
		return workspaceAt{}, rpc.BadRequestError("workspace is a workspace's key or name", "workspace", rpc.InvalidParams)
	}
	var w workspaceAt
	var id int64
	var lead sql.NullInt64
	q := "SELECT id, name, lead FROM workspaces WHERE name = ?"
	arg := any(who)
	if k, n, ok := actor.Parse(who); ok && k == actor.Workspace {
		q, arg = "SELECT id, name, lead FROM workspaces WHERE id = ?", n
	}
	if err := e.DB.QueryRow(q, arg).Scan(&id, &w.name, &lead); errors.Is(err, sql.ErrNoRows) {
		nf := rpc.NotFoundError("no workspace " + who)
		nf.Data = map[string]any{"what": "workspace", "name": who}
		return w, nf
	} else if err != nil {
		return w, err
	}
	w.key = actor.Key(actor.Workspace, id)
	rows, err := e.DB.Query("SELECT id FROM agents WHERE workspace = ? ORDER BY id = ?, id", id, lead.Int64)
	if err != nil {
		return w, err
	}
	defer rows.Close()
	for rows.Next() {
		var a int64
		if err := rows.Scan(&a); err != nil {
			return w, err
		}
		w.agents = append(w.agents, actor.Key(actor.Agent, a))
	}
	return w, rows.Err()
}

// workspaceSet is workspace.set: a workspace's name, the one thing it changes. Its agents keep their keys, folders and sessions.
func (e *Env) workspaceSet(c *rpc.Conn, params map[string]any) (any, error) {
	w, err := e.workspaceOf(params)
	if err != nil {
		return nil, err
	}
	name, _ := params["name"].(string)
	if strings.TrimSpace(name) == "" {
		return nil, rpc.BadRequestError("name is the workspace's new name", "name", rpc.InvalidParams)
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		if err := team.NewRosterOn(tx, e.Home).RenameWorkspace(w.key, name); err != nil {
			if team.IsRefusal(err) {
				return nil, rpc.Refused(err.Error(), "state", "")
			}
			return nil, err
		}
		return map[string]any{"accepted": true, "said": fmt.Sprintf("workspace %s is %s", w.name, name)}, nil
	})
}

// workspaceDelete is workspace.delete: every agent of the workspace retired as agent.retire retires one, its lead last and the
// workspace with it, in one step that puts everything back when any part is refused. The project folder is never touched. Refused,
// with nothing done, while any of its agents has a session running, or holds work in a worktree and worktree gives no answer for it.
func (e *Env) workspaceDelete(c *rpc.Conn, params map[string]any) (any, error) {
	w, err := e.workspaceOf(params)
	if err != nil {
		return nil, err
	}
	choice, _ := params["worktree"].(string)
	if v, has := params["worktree"]; has && choice != worktreeprune.Save && choice != worktreeprune.Force {
		return nil, rpc.BadRequestError(fmt.Sprintf("worktree is save or force, not %v", v), "worktree", rpc.InvalidParams)
	}
	release, held := wslock.Delete(e.Home, w.key) // before the checks: no agent is added or started in it from here on
	if !held {
		return nil, rpc.Refused(w.name+" has an agent being created or started just now: try again once it is.", "state", "")
	}
	defer release()
	live, ok := map[string]bool(nil), false
	if e.Live != nil {
		live, ok = e.Live()
	}
	if !ok {
		return nil, rpc.Refused("could not confirm whether the agents of "+w.name+" have a session running just now - try again in a moment.", "state", "")
	}
	var running []string
	for _, a := range w.agents {
		if live[a] {
			running = append(running, addr.Ref(a))
		}
	}
	if len(running) > 0 {
		return nil, rpc.Refused(fmt.Sprintf("%s has sessions running (%s): stop them first. Nothing was deleted.", w.name, strings.Join(running, ", ")), "state", "")
	}
	for _, a := range w.agents { // every refusal before the first removal: what is left to fail part way is git or the disk
		if choice == "" {
			if err := e.refuseHeldWork(a); err != nil {
				return nil, err
			}
		}
		mine, err := worktreeprune.Of(e.DB, a)
		if err != nil {
			return nil, err
		}
		if _, refused, err := e.leaving(a, mine, choice); err != nil {
			return nil, err
		} else if refused != "" {
			return nil, rpc.Refused(refused+" Nothing was deleted.", "worktree", "")
		}
	}
	prop := &Prop{}
	prop.set("kind", bundle)
	prop.set("id", fmt.Sprintf("ws-%s-%d", w.key, time.Now().UnixNano()))
	for i, a := range w.agents { // the lead last: it goes only as the workspace's last agent, and the workspace with it
		prop.set(fmt.Sprintf("i%d.kind", i), "retire")
		prop.set(fmt.Sprintf("i%d.agent", i), a)
		if choice != "" {
			prop.set(fmt.Sprintf("i%d.worktree", i), choice)
		}
	}
	if len(w.agents) == 0 { // a workspace no agent is left in
		return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			if _, err := tx.Exec("DELETE FROM workspaces WHERE name = ? AND NOT EXISTS (SELECT 1 FROM agents WHERE workspace = workspaces.id)", w.name); err != nil {
				return nil, err
			}
			return map[string]any{"accepted": true, "said": "workspace " + w.name + " is deleted"}, nil
		})
	}
	ok, said, err := e.run(prop)
	if err != nil {
		return nil, err
	}
	if ok {
		said = "workspace " + w.name + " is deleted; its project folder is left as it was.\n" + said
	}
	return e.retired(c, ok, said)
}

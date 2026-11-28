package proposalwrites

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
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
	id        int64
	key, name string
	agents    []string
}

func (e *Env) workspaceOf(params map[string]any) (workspaceAt, error) {
	who, _ := params["workspace"].(string)
	if strings.TrimSpace(who) == "" {
		return workspaceAt{}, rpc.BadRequestError("workspace is a workspace's key or name", "workspace", rpc.InvalidParams)
	}
	var w workspaceAt
	var lead sql.NullInt64
	q := "SELECT id, name, lead FROM workspaces WHERE name = ?"
	arg := any(who)
	if k, n, ok := actor.Parse(who); ok && k == actor.Workspace {
		q, arg = "SELECT id, name, lead FROM workspaces WHERE id = ?", n
	}
	if err := e.DB.QueryRow(q, arg).Scan(&w.id, &w.name, &lead); errors.Is(err, sql.ErrNoRows) {
		nf := rpc.NotFoundError("no workspace " + who)
		nf.Data = map[string]any{"what": "workspace", "name": who}
		return w, nf
	} else if err != nil {
		return w, err
	}
	w.key = actor.Key(actor.Workspace, w.id)
	rows, err := e.DB.Query("SELECT id FROM agents WHERE workspace = ? ORDER BY id = ?, id", w.id, lead.Int64)
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

// workspaceSet is workspace.set: a workspace's name, and a folder added to it or taken from it. Its agents keep their keys, folders and
// sessions; a folder one of them works in is not taken.
func (e *Env) workspaceSet(c *rpc.Conn, params map[string]any) (any, error) {
	w, err := e.workspaceOf(params)
	if err != nil {
		return nil, err
	}
	var name, add, remove, mode string
	for _, f := range []struct {
		k string
		v *string
	}{{"name", &name}, {"add_folder", &add}, {"remove_folder", &remove}, {"session_mode", &mode}} {
		if params[f.k] == nil {
			continue
		}
		s, ok := params[f.k].(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, rpc.BadRequestError(f.k+" is a non-empty string", f.k, rpc.InvalidParams)
		}
		*f.v = s
	}
	if name == "" && add == "" && remove == "" && mode == "" {
		return nil, rpc.BadRequestError("name, add_folder, remove_folder or session_mode says what changes", "name", rpc.InvalidParams)
	}
	if mode != "" && !slices.Contains(team.SessionModes, mode) {
		return nil, rpc.BadRequestError("session_mode is one of "+strings.Join(team.SessionModes, ", "), "session_mode", rpc.InvalidParams)
	}
	if add != "" { // git is asked here, outside the write's transaction
		kept, _, err := team.Folder(add)
		if team.IsRefusal(err) {
			return nil, rpc.Refused(err.Error(), "state", "")
		} else if err != nil {
			return nil, err
		}
		add = kept
	}
	if name != "" && name != w.name {
		if err := e.refuseRename(w); err != nil {
			return nil, err
		}
	}
	return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
		r := team.NewRosterOn(tx, e.Home)
		var said []string
		err := func() error {
			if name != "" {
				if err := r.RenameWorkspace(w.key, name); err != nil {
					return err
				}
				said = append(said, fmt.Sprintf("workspace %s is %s", w.name, name))
			}
			if add != "" {
				if err := r.AddFolder(w.key, add); err != nil {
					return err
				}
				said = append(said, "added "+add)
			}
			if remove != "" {
				if err := r.RemoveFolder(w.key, remove); err != nil {
					return err
				}
				said = append(said, "took "+remove+" away")
			}
			if mode != "" {
				changed, err := r.SetWorkspaceSessionMode(w.key, mode)
				if err != nil {
					return err
				}
				if changed {
					said = append(said, "its sessions run in "+mode+" mode unless an agent says")
				}
			}
			return nil
		}()
		if err != nil {
			if team.IsRefusal(err) {
				return nil, rpc.Refused(err.Error(), "state", "")
			}
			return nil, err
		}
		return map[string]any{"accepted": true, "said": strings.Join(said, "; ")}, nil
	})
}

// refuseRename refuses a rename while an open proposal names one of the agents as workspace/name, which would then reach nobody. A
// running session goes on: hl names its agent by the session its process descends from, not by the name it started under.
func (e *Env) refuseRename(w workspaceAt) error {
	for _, p := range e.listing() {
		if st := p.str("state"); st != "proposed" && st != "approved" && st != applying {
			continue
		}
		for _, kv := range p.pairs {
			field := kv.Key
			if m := itemKey.FindStringSubmatch(field); m != nil {
				field = m[2]
			}
			if field != "agent" && field != "to" && field != "members" {
				continue
			}
			for _, n := range strings.Split(fmt.Sprint(kv.Val), ",") {
				if n == w.name || strings.HasPrefix(n, w.name+"/") {
					return rpc.Refused(fmt.Sprintf("proposal %s names %s: decide or withdraw it before renaming %s.", p.id(), n, w.name), "state", "")
				}
			}
		}
	}
	return nil
}

// workspaceDelete is workspace.delete: its agents retired in one bundle, the lead last and the workspace with it, after every
// refusal is checked for all of them. The project folder is never touched.
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
	if choice == "" {
		if err := e.refuseHeldWork(w.agents...); err != nil {
			return nil, err
		}
	}
	for _, a := range w.agents {
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
	for i, a := range w.agents { // the lead last, so none of the others is made lead on the way
		prop.set(fmt.Sprintf("i%d.kind", i), "retire")
		prop.set(fmt.Sprintf("i%d.agent", i), a)
		if choice != "" {
			prop.set(fmt.Sprintf("i%d.worktree", i), choice)
		}
	}
	if len(w.agents) == 0 { // a workspace no agent is left in
		return c.Effect(func(tx *sql.Tx) (map[string]any, error) {
			res, err := tx.Exec("DELETE FROM workspaces WHERE id = ? AND NOT EXISTS (SELECT 1 FROM agents WHERE workspace = workspaces.id)", w.id)
			if err != nil {
				return nil, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				if err := team.ClearRules(tx, w.id); err != nil {
					return nil, err
				}
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

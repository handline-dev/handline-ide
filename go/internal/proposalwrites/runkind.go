package proposalwrites

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/internal/cmdruns"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/pyjson"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/worktreepaths"
)

// A run proposal (docs/design/agent-run-proposals.md): an agent proposes a shell command, and the founder's approve runs it in a window of
// the terminal host beside that agent, which is told how it ended. The window is opened by the daemon, outside the agent's sandbox.

const (
	maxRunCommand = 4000 // as shell.open's
	maxOpenRuns   = 5    // open run proposals per agent
)

// Runs is what a run proposal needs of the terminal host (termchan.Env).
type Runs interface {
	StartRun(agent, cwd, command string, block cmdruns.Block) (string, error)
	CloseRun(r cmdruns.Run) error
}

// draftRun checks a run proposal's command and cwd and keeps them on prop, or says why not.
func (e *Env) draftRun(asker, agent string, args map[string]any, prop *Prop) string {
	if asker != agent && !actor.IsPerson(asker) {
		return fmt.Sprintf("an agent proposes its own commands: name yourself, not %s", agent)
	}
	command := argStr(args, "command")
	if command == "" || len(command) > maxRunCommand || strings.ContainsRune(command, 0) {
		return fmt.Sprintf("a run needs a command (--command), 1 to %d characters with no NUL", maxRunCommand)
	}
	cwd := argStr(args, "cwd")
	if cwd == "" || strings.ContainsAny(cwd, "\x00\n\r") {
		return "a run needs the folder it runs in (--cwd)"
	}
	if !filepath.IsAbs(cwd) {
		var recorded sql.NullString
		if err := e.DB.QueryRow("SELECT worktree FROM agent_state WHERE agent = ?", agent).Scan(&recorded); err != nil && err != sql.ErrNoRows {
			return fmt.Sprintf("the worktree of %s cannot be read (%v)", agent, err)
		}
		wt := worktreepaths.WorktreePath(recorded.String)
		if wt == "" {
			return fmt.Sprintf("cwd %s is relative and %s has no worktree to take it from: give an absolute folder", cwd, agent)
		}
		cwd = filepath.Join(wt, cwd)
	}
	cwd = filepath.Clean(cwd)
	if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
		return fmt.Sprintf("cwd %s is not a folder", cwd)
	}
	open := 0
	for _, p := range e.listing() {
		if p.kind() == "run" && p.str("agent") == agent && p.str("state") == "proposed" {
			open++
		}
	}
	if open >= maxOpenRuns {
		return fmt.Sprintf("%s has %d run proposals waiting for the founder: wait for those or withdraw one", agent, open)
	}
	prop.set("set.command", pyjson.PyJSON(command, false, true)) // a proposal file holds no line break
	prop.set("set.cwd", cwd)
	return ""
}

// runCommand is the command and cwd a run proposal keeps.
func (p *Prop) runCommand() (command, cwd string) {
	sets := p.sets()
	if json.Unmarshal([]byte(sets["command"]), &command) != nil {
		command = sets["command"]
	}
	return command, sets["cwd"]
}

func (p *Prop) runBlock() cmdruns.Block { return cmdruns.ProposalBlock(proposalNumber(p.id())) }

// started is whether a run proposal's command was started already: it runs once, even when that run failed.
func (e *Env) started(p *Prop) (bool, error) {
	if p.kind() != "run" {
		return false, nil
	}
	_, ok, err := cmdruns.OfProposalRun(e.DB, proposalNumber(p.id()))
	return ok, err
}

// startRun is the apply of an approved run proposal: its window is opened in its cwd. A start that fails leaves it approved with the error,
// and approving it again tries again.
func (e *Env) startRun(prop *Prop) (bool, string, error) {
	if e.Runs == nil {
		return false, "this daemon cannot run commands (no terminal host)", nil
	}
	command, cwd := prop.runCommand()
	if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
		return false, fmt.Sprintf("cwd %s is not a folder any more", cwd), nil
	}
	handle, err := e.Runs.StartRun(prop.str("agent"), cwd, command, prop.runBlock())
	if ran, rerr := e.started(prop); err != nil && rerr == nil && ran {
		// a second approve that raced the first: that one started it, and this one leaves its record as it is
		return true, "its command was started already; you are told how it ends", nil
	}
	if err != nil {
		var re *rpc.Error
		if errors.As(err, &re) {
			return false, re.Message, nil
		}
		return false, "", err
	}
	return true, fmt.Sprintf("its command runs in window %s; you are told how it ends", handle), nil
}

// stopMethod is proposal.stop: the founder closes the window of a run proposal's running command. The run ends stopped at the next look,
// and its agent is told then.
func (e *Env) stopMethod(c *rpc.Conn, params map[string]any) (any, error) {
	prop, err := e.proposalNamed(params)
	if err != nil {
		return nil, err
	}
	if c.Caller.Actor() != actor.OwnerKey {
		return nil, rpc.Refused("a proposal's command is the founder's to stop", "role", "")
	}
	if prop.kind() != "run" {
		return nil, rpc.Refused(fmt.Sprintf("proposal %s is not a run: there is no command to stop", prop.id()), "state", "")
	}
	r, ok, err := cmdruns.OfProposalRun(e.DB, proposalNumber(prop.id()))
	if err != nil {
		return nil, err
	}
	if !ok || r.State != "running" {
		return nil, rpc.Refused(fmt.Sprintf("the command of proposal %s is not running", prop.id()), "state", "")
	}
	if e.Runs == nil {
		return nil, rpc.Refused("this daemon cannot close a window (no terminal host)", "unsupported", "")
	}
	if err := e.Runs.CloseRun(r); err != nil {
		return nil, err
	}
	return c.Effect(func(*sql.Tx) (map[string]any, error) {
		return map[string]any{"accepted": true, "said": fmt.Sprintf("proposal %s: window %s closed", prop.id(), r.Terminal)}, nil
	})
}

// Package temps hires and lets go the temps of busy agents. A task that comes to an agent mid-turn on another goes to a temp of it
// (supervisor.Env.hireTemps): a row of its own named after the agent with a number, with its settings, whose role, lessons and memory
// folder are the agent's (paths.KnowsAs). It starts in a desk of its own like any agent with work waiting, and once its task is over and
// its turn has ended it leaves: session closed, desk given back, rows gone (proposalwrites.Env.Dismiss), and what still names it reaches
// the agent (team.LeftRows, addr.Resolve). The hire and the leave are events.
package temps

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/worktreepool"
)

// The config.yaml key that turns hiring on or off: Auto, the default, hires; Manual keeps a task waiting for its busy agent.
const (
	Key    = "temp_workers"
	Auto   = "auto"
	Manual = "manual"
)

// ModeOf is temp_workers as config.yaml says it, Auto when unset or unknown.
func ModeOf(cfg map[string]any) string {
	if cfg[Key] == Manual {
		return Manual
	}
	return Auto
}

// The events a hire and a leave add, under the temp's key.
const (
	HiredEvent = "agent.hired"
	LeftEvent  = "agent.left"
)

// Env is what hiring and letting go read and do.
type Env struct {
	DB *sql.DB
	// Mode is temp_workers now (ModeOf).
	Mode func() string
	// Room is why one more session like agent's, beside its own, could not start now (the machine's load, then
	// lifecycle.Launcher.RoomBeside), "" when it could. It is asked before any row is written.
	Room func(agent string) string
	// Session is whether agent has a session, and whether that session is at its prompt.
	Session func(agent string) (present, idle bool)
	// Close ends agent's session at its prompt.
	Close func(agent string) (ok bool, why string)
	// Desks is the pool a temp's desk goes back to; nil when there is none.
	Desks *worktreepool.Pool
	// HandBack hands task tid to agent as the daemon, with note (taskwrites.HandOffAsDaemon).
	HandBack func(tid int64, agent, note string) error
	// Dismiss removes a temp's rows (proposalwrites.Env.Dismiss); refused is why it stays.
	Dismiss func(temp string) (refused string, err error)
	Log     func(string, ...any)
	Now     func() time.Time
}

func (e *Env) now() int64 {
	if e.Now != nil {
		return e.Now().Unix()
	}
	return time.Now().Unix()
}

func (e *Env) log(f string, a ...any) {
	if e.Log != nil {
		e.Log(f, a...)
	}
}

// Hire adds a temp of agent and is its key, or "" when temps are Manual or a session of it could not start now (then nothing is written).
func (e *Env) Hire(agent string) string {
	if e.Mode != nil && e.Mode() == Manual {
		return ""
	}
	origin, err := paths.AgentKeyIn(e.DB, agent)
	if err != nil || origin == "" {
		return ""
	}
	if from, err := paths.HiredFrom(e.DB, origin); err != nil {
		return ""
	} else if from != "" {
		origin = from // a temp of a temp is the agent's
	}
	if why := e.Room(origin); why != "" {
		return ""
	}
	temp, err := team.HireRow(e.DB, origin, e.now())
	if err != nil {
		e.log("  %s: no temp hired: %v", agent, err)
		return ""
	}
	if _, err := db.AddEvent(e.DB, temp, HiredEvent, db.KV{K: "hired_from", V: origin}); err != nil {
		e.log("  %s: its hire is not recorded: %v", temp, err)
	}
	e.log("  %s: hired %s", addr.Ref(agent), temp)
	return temp
}

// Waiting is whether agent has a temp with work and no session: one hired that has not started yet, or could not. Another is not hired
// for it meanwhile, since a start not under way counts against no limit yet.
func (e *Env) Waiting(agent string) bool {
	origin, err := paths.AgentKeyIn(e.DB, agent)
	if err != nil {
		return true
	}
	temps, err := team.TempsOf(e.DB, origin)
	if err != nil {
		return true
	}
	for _, t := range temps {
		if present, _ := e.Session(t); !present {
			return true
		}
	}
	return false
}

// Sweep lets each temp go whose task is over: one that holds no open task and is not mid-turn. A session at its prompt is closed
// first; the temp leaves on a later sweep, once the session is gone.
func (e *Env) Sweep() error {
	temps, err := team.Temps(e.DB)
	if err != nil {
		return err
	}
	for temp, origin := range temps {
		if held, err := team.Holds(e.DB, temp); err != nil {
			return err
		} else if held {
			if present, _ := e.Session(temp); !present {
				e.handBack(temp, origin)
			}
			continue
		}
		if present, idle := e.Session(temp); present {
			if idle {
				if ok, why := e.Close(temp); !ok {
					e.log("  %s: its task is over but its session did not close: %s", temp, why)
				}
			}
			continue
		}
		if err := e.leave(temp); err != nil {
			e.log("  %s: did not leave: %v", temp, err)
		}
	}
	return nil
}

// handBack gives the tasks of temp, which has no session, that it never began back to origin once origin is free: a temp that could
// not start leaves no task stranded on it. One it began stays; its session comes back for it as any agent's does.
func (e *Env) handBack(temp, origin string) {
	if e.HandBack == nil {
		return
	}
	if row, err := db.GetAgentState(e.DB, origin); err != nil || row != nil && row.State == states.Running {
		return
	}
	if asking, err := db.CurrentTaskStillAsking(e.DB, origin); err != nil || asking {
		return
	}
	open, err := db.OpenTasksOf(e.DB, temp)
	if err != nil {
		return
	}
	for _, t := range open {
		if begun, err := db.Begun(e.DB, t.ID); err != nil || begun {
			continue
		}
		if err := e.HandBack(t.ID, origin, fmt.Sprintf("%s, hired for it, never started; %s is free now.", temp, addr.Ref(origin))); err != nil {
			e.log("  %s: T%d was not given back to %s: %v", temp, t.ID, origin, err)
		}
	}
}

// leave gives temp's desk back and removes it. A desk that holds work git cannot simply commit stays, kept for the founder, and the
// memo on temp's last task names it.
func (e *Env) leave(temp string) error {
	last, err := lastTask(e.DB, temp)
	if err != nil {
		return err
	}
	if e.Desks != nil {
		g, err := e.Desks.GiveBack(temp, last)
		if err != nil {
			return err
		}
		if g.Desk != "" && !g.Freed {
			if err := keepDesk(e.DB, g.Desk); err != nil {
				return err
			}
			if last != 0 {
				memo := fmt.Sprintf("%s left; its desk %s holds work git cannot simply commit, so it is kept for the founder (`hl desk unpin` lets it go).", temp, g.Desk)
				if _, err := db.AddMemo(e.DB, last, "daemon", memo); err != nil {
					return err
				}
			}
		}
	}
	refused, err := e.Dismiss(temp)
	if err != nil {
		return err
	}
	if refused != "" {
		return fmt.Errorf("%s", refused)
	}
	if _, err := db.AddEvent(e.DB, temp, LeftEvent, db.KV{K: "task_id", V: last}); err != nil {
		e.log("  %s: its leave is not recorded: %v", temp, err)
	}
	e.log("  %s: its task is over - it left", temp)
	return nil
}

// keepDesk lets nobody take desk but the founder, and frees it of the temp that held it.
func keepDesk(q db.Q, desk string) error {
	if err := worktreepool.Pin(q, desk, worktreepool.Founder); err != nil {
		return err
	}
	_, err := q.Exec("UPDATE worktree_folders SET holder = NULL WHERE path = ?", desk)
	return err
}

// lastTask is the newest task temp was given, 0 for none.
func lastTask(q db.Q, temp string) (int64, error) {
	var tid int64
	err := q.QueryRow("SELECT COALESCE(MAX(id), 0) FROM tasks WHERE owner = ?", temp).Scan(&tid)
	return tid, err
}

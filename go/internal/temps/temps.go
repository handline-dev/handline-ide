// Package temps gives a task that comes to an agent mid-turn on another to a seat of it (supervisor.Env.hireTemps): a row of its own named
// after the agent with a number, with its settings, whose role, lessons and memory folder are the agent's (paths.KnowsAs). A hire takes
// the agent's lowest idle seat, or adds one when every seat is at work, and starts it on a fresh conversation in a desk of its own. Once
// the task is over and the turn has ended the seat closes its session, gives its desk back and sits idle; a word sent to it then reaches
// the agent (paths.Reaches). The hire and the leave are events in the agent's own chat.
package temps

import (
	"database/sql"
	"fmt"
	"os"
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

// The events a hire and a leave add, under the key of the agent the seat is of: lines in its chat.
const (
	HiredEvent = "agent.hired"
	LeftEvent  = "agent.left"
)

// Env is what hiring and letting go read and do.
type Env struct {
	DB   *sql.DB
	Home string
	// Mode is temp_workers now (ModeOf).
	Mode func() string
	// Room is why one more session like agent's, beside its own, could not start now (the machine's load, then
	// lifecycle.Launcher.RoomBeside), "" when it could. It is asked before any row is written.
	Room func(agent string) string
	// Session is whether agent has a session, and whether that session is at its prompt.
	Session func(agent string) (present, idle bool)
	// Close ends agent's session at its prompt.
	Close func(agent string) (ok bool, why string)
	// Desks is the pool a seat's desk goes back to; nil when there is none.
	Desks *worktreepool.Pool
	// HandOff hands task tid to agent as the daemon, with note (taskwrites.HandOffAsDaemon).
	HandOff func(tid int64, agent, note string) error
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

// Hire hands task tid, waiting for agent, to a seat of agent with note, and is the seat's key; "" when temps are Manual, a session of it
// could not start now, or the handoff failed (then no seat is at work for it).
func (e *Env) Hire(agent string, tid int64, note string) string {
	if e.Mode != nil && e.Mode() == Manual {
		return ""
	}
	origin, err := paths.AgentKeyIn(e.DB, agent)
	if err != nil || origin == "" {
		return ""
	}
	if origin, err = paths.KnowsAs(e.DB, origin); err != nil { // a seat's task goes to a seat of its agent
		return ""
	}
	if why := e.Room(origin); why != "" {
		return ""
	}
	seat, err := e.seatFor(origin)
	if err == nil {
		err = team.SetHiredFor(e.DB, seat, tid)
	}
	if err == nil {
		err = e.HandOff(tid, seat, note)
		if err != nil {
			if err := team.SetHiredFor(e.DB, seat, 0); err != nil {
				e.log("  %s: still marked at work: %v", seat, err)
			}
		}
	}
	if err != nil {
		e.log("  %s: T%d went to no seat: %v", addr.Ref(origin), tid, err)
		return ""
	}
	if _, err := db.AddEvent(e.DB, origin, HiredEvent, db.KV{K: "seat", V: seat}, db.KV{K: "task_id", V: tid}); err != nil {
		e.log("  %s: its hire is not recorded: %v", seat, err)
	}
	e.log("  %s: T%d went to its seat %s", addr.Ref(origin), tid, seat)
	return seat
}

// seatFor is origin's lowest idle seat with no session, made fresh (no conversation to resume, no handoff), or a new one.
func (e *Env) seatFor(origin string) (string, error) {
	seats, err := team.TempsOf(e.DB, origin)
	if err != nil {
		return "", err
	}
	for _, s := range seats {
		if at, err := team.HiredFor(e.DB, s); err != nil {
			return "", err
		} else if at != 0 {
			continue
		}
		if present, _ := e.Session(s); present {
			continue
		}
		return s, e.fresh(s)
	}
	return team.NewSeat(e.DB, origin, e.now())
}

// fresh has seat's next session start a new conversation with nothing carried from its last task.
func (e *Env) fresh(seat string) error {
	if err := db.ForgetResume(e.DB, seat); err != nil {
		return err
	}
	if err := db.ClearHandoff(e.DB, seat); err != nil {
		return err
	}
	if err := os.Remove(paths.HandoffNoteFile(e.Home, seat)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Waiting is whether agent has a seat at work with no session: one hired that has not started yet, or could not. Another is not hired
// for it meanwhile, since a start not under way counts against no limit yet.
func (e *Env) Waiting(agent string) bool {
	origin, err := paths.AgentKeyIn(e.DB, agent)
	if err != nil {
		return true
	}
	seats, err := team.TempsOf(e.DB, origin)
	if err != nil {
		return true
	}
	for _, s := range seats {
		at, err := team.HiredFor(e.DB, s)
		if err != nil {
			return true
		}
		if present, _ := e.Session(s); at != 0 && !present {
			return true
		}
	}
	return false
}

// Sweep lets each seat at work go whose task is over: one that holds no open task and is not mid-turn. A session at its prompt is closed
// first; the seat leaves on a later sweep, once the session is gone.
func (e *Env) Sweep() error {
	seats, err := team.Temps(e.DB)
	if err != nil {
		return err
	}
	for seat, origin := range seats {
		at, err := team.HiredFor(e.DB, seat)
		if err != nil {
			return err
		}
		held, err := team.Holds(e.DB, seat)
		if err != nil {
			return err
		}
		if held && at == 0 { // a task that came back to an idle seat (reopened) goes to its agent
			e.giveIdle(seat, origin)
		}
		if held && at != 0 {
			if present, _ := e.Session(seat); !present {
				e.handBack(seat, origin)
			}
			continue
		}
		if present, idle := e.Session(seat); present {
			if idle && !held {
				if ok, why := e.Close(seat); !ok {
					e.log("  %s: its task is over but its session did not close: %s", seat, why)
				}
			}
			continue
		}
		if at != 0 {
			if err := e.leave(seat, origin, at); err != nil {
				e.log("  %s: did not leave: %v", seat, err)
			}
		}
	}
	return nil
}

// handBack gives the tasks of seat, which has no session, that it never began back to origin once origin is free: a seat that could
// not start leaves no task stranded on it. One it began stays; its session comes back for it as any agent's does.
func (e *Env) handBack(seat, origin string) {
	if e.HandOff == nil {
		return
	}
	if row, err := db.GetAgentState(e.DB, origin); err != nil || row != nil && row.State == states.Running {
		return
	}
	if asking, err := db.CurrentTaskStillAsking(e.DB, origin); err != nil || asking {
		return
	}
	open, err := db.OpenTasksOf(e.DB, seat)
	if err != nil {
		return
	}
	for _, t := range open {
		if begun, err := db.Begun(e.DB, t.ID); err != nil || begun {
			continue
		}
		if err := e.HandOff(t.ID, origin, fmt.Sprintf("%s, hired for it, never started; %s is free now.", seat, addr.Ref(origin))); err != nil {
			e.log("  %s: T%d was not given back to %s: %v", seat, t.ID, origin, err)
		}
	}
}

// giveIdle hands the open tasks idle seat acts on to origin.
func (e *Env) giveIdle(seat, origin string) {
	if e.HandOff == nil {
		return
	}
	open, err := db.OpenTasksOf(e.DB, seat)
	if err != nil {
		e.log("  %s: its tasks were not read: %v", seat, err)
		return
	}
	for _, t := range open {
		if err := e.HandOff(t.ID, origin, fmt.Sprintf("T%d came back to %s, which is idle; it is %s's.", t.ID, seat, addr.Ref(origin))); err != nil {
			e.log("  %s: T%d was not given to %s: %v", seat, t.ID, origin, err)
		}
	}
}

// leave gives seat's desk back and marks it idle, its hire for task tid over. A desk that holds work git cannot simply commit stays, kept
// for the founder, and a memo on the task names it.
func (e *Env) leave(seat, origin string, tid int64) error {
	if e.Desks != nil {
		g, err := e.Desks.GiveBack(seat, tid)
		if err != nil {
			return err
		}
		if g.Desk != "" && !g.Freed {
			if err := keepDesk(e.DB, g.Desk); err != nil {
				return err
			}
			memo := fmt.Sprintf("%s left; its desk %s holds work git cannot simply commit, so it is kept for the founder (`hl desk unpin` lets it go).", seat, g.Desk)
			if _, err := db.AddMemo(e.DB, tid, "daemon", memo); err != nil {
				return err
			}
		}
	}
	ended, err := endedAs(e.DB, tid)
	if err != nil {
		return err
	}
	if err := team.SetHiredFor(e.DB, seat, 0); err != nil {
		return err
	}
	if _, err := db.AddEvent(e.DB, origin, LeftEvent, db.KV{K: "seat", V: seat}, db.KV{K: "task_id", V: tid}, db.KV{K: "ended", V: ended}); err != nil {
		e.log("  %s: its leave is not recorded: %v", seat, err)
	}
	e.log("  %s: T%d is over (%s) - it is idle", seat, tid, ended)
	return nil
}

// endedAs is how a seat's task tid ended for it: done, discarded, or handed_off when it went on to another (or back to the agent).
func endedAs(q db.Q, tid int64) (string, error) {
	t, err := db.GetTask(q, tid)
	if err != nil || t == nil {
		return "discarded", err
	}
	switch {
	case t.DiscardedBy.String != "":
		return "discarded", nil
	case t.DoneEvidence.String != "":
		return "done", nil
	}
	return "handed_off", nil
}

// keepDesk lets nobody take desk but the founder, and frees it of the seat that held it.
func keepDesk(q db.Q, desk string) error {
	if err := worktreepool.Pin(q, desk, worktreepool.Founder); err != nil {
		return err
	}
	_, err := q.Exec("UPDATE worktree_folders SET holder = NULL WHERE path = ?", desk)
	return err
}

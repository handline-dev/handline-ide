// Package verdict is the judgements the daemon makes of plain values: no I/O and no internal imports, so each is tested without a database.
package verdict

import "strings"

// State is what an agent's session is doing, read from the session file, the turn tracker and the screen.
type State int

const (
	Unknown            State = iota // a process runs and what it says of itself cannot be read
	Starting                        // launched, no session file yet
	Working                         // in a turn
	IdleWithBackground              // idle, with background work still running
	Idle                            // waiting for a message
	WaitingPermission               // blocked on a permission prompt
	Dialog                          // blocked on any other dialog, question or choice
	ShellLeft                       // the session ended and the terminal is at a shell
	Closed                          // no session runs
)

// Turn is the turn tracker's answer, Turn unknown when none is kept.
type Turn int

const (
	TurnUnknown Turn = iota
	TurnWorking
	TurnIdle
)

// Screen is what the terminal shows beside the session file.
type Screen int

const (
	ScreenClear Screen = iota
	ScreenBackground
	ScreenShell
)

// Input is everything AgentState reads. Status is the session file's status: "idle", "busy", "waiting", "" for no session, anything
// else for a file that cannot be read. Launching says the process was started and has no file yet.
type Input struct {
	Status, WaitingFor string
	Turn               Turn
	Screen             Screen
	Launching          bool
}

// AgentState is the one reading of an agent's session. A known turn decides before the file does.
func AgentState(in Input) State {
	switch in.Turn {
	case TurnWorking:
		return Working
	case TurnIdle:
		return Idle
	}
	if in.Screen == ScreenShell {
		return ShellLeft
	}
	switch in.Status {
	case "":
		if in.Launching {
			return Starting
		}
		return Closed
	case "busy":
		return Working
	case "idle":
		if in.Screen == ScreenBackground {
			return IdleWithBackground
		}
		return Idle
	case "waiting":
		if strings.Contains(strings.ToLower(in.WaitingFor), "permission") {
			return WaitingPermission
		}
		return Dialog
	}
	return Unknown
}

// Busy is the screen's word: in a turn. A permission prompt is not busy, the agent waits on a person.
func Busy(s State) bool { return s == Working }

// Blocked is whether the session waits on a person.
func Blocked(s State) bool { return s == WaitingPermission || s == Dialog }

// SafeToMove is whether a move may end the session: only an idle one. A session that is not there has nothing to end, and the caller
// asks Closed for that.
func SafeToMove(s State) bool { return s == Idle }

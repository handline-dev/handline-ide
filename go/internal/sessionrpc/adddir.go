package sessionrpc

import (
	"strings"
	"time"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/lock"
)

// addDirOption is the option of Claude Code's /add-dir dialog that adds the folder to the running session only.
const addDirOption = "Yes, for this session"

// addDirWait is how long AddDir waits for the dialog /add-dir opens, addDirProbe how often it looks.
var addDirWait, addDirProbe = 3 * time.Second, 100 * time.Millisecond

// AddDir gives agent's running session the folder dir at once, as a person would: /add-dir typed at its idle composer, then "Yes, for
// this session" on the dialog it opens (contract x-access, applies_from now). false when the terminal is busy or held by someone, a turn
// runs, something waits on the screen, the path has white space, or the dialog is not that one; a dialog left open is then declined and
// the typed line cleared.
func (e *Env) AddDir(agent, dir string) bool {
	if strings.ContainsAny(dir, " \t") || e.Holds == nil {
		return false
	}
	hold := lock.TakeTerminal(e.Home, agent)
	if hold == nil {
		return false
	}
	defer hold.Release()
	if row, err := e.Holds.Holder(agent); err != nil || row != nil {
		return false
	}
	handle, err := db.Terminal(e.DB, agent)
	if err != nil || handle == "" || !e.Term.Backend.Available() || e.Term.ShellLeft(handle) != "" || e.Term.ScreenBlocker(handle) != nil {
		return false
	}
	if shown, ok := e.Term.TurnShown(handle); !ok || shown {
		return false
	}
	if ok, _ := e.Term.Send(handle, "/add-dir "+dir, true); !ok {
		return false
	}
	if e.answerAddDir(handle) {
		return true
	}
	if e.Term.DialogOnScreen(handle) != nil {
		e.Term.DeclineDialog(handle)
	}
	if text, ok := e.Term.ComposerOnScreen(handle); ok && text != "" {
		e.Term.Send(handle, ctrlU, false)
	}
	return false
}

// ctrlU deletes the composer's line, what /add-dir left there unsent.
const ctrlU = "\x15"

// answerAddDir waits for the /add-dir dialog and picks "Yes, for this session"; false when it does not come, or does not clear.
func (e *Env) answerAddDir(handle string) bool {
	for end := time.Now().Add(addDirWait); time.Now().Before(end); time.Sleep(addDirProbe) {
		d := e.Term.DialogOnScreen(handle)
		if d == nil {
			continue
		}
		for _, o := range d.Options {
			if strings.HasPrefix(o.Label, addDirOption) {
				outcome, _, _ := e.Term.AnswerDialog(handle, func(string, *term.Dialog) int { return o.Number })
				return outcome == term.Cleared
			}
		}
		return false
	}
	return false
}

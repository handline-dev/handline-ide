package cmdruns

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/textsafe"
)

// TailRows is how many of an ended run's last rows its agent is told.
const TailRows = 40

const (
	maxCommand = 200  // the most of the command the line quotes
	maxTail    = 6000 // the most output told, so rows a program drew very wide still make a message an agent can take
)

// Tell queues for the run's agent how the run ended, as a say: it is told like any message, once the agent is at its prompt. tail is the
// last rows of the run's window as the host kept them. A run with no agent recorded tells no one.
func Tell(q db.Q, r Run, tail []string) error {
	if r.Agent == "" {
		return nil
	}
	_, err := db.AddRequest(q, db.NewRequest{Requester: db.NoticeRequester, Target: r.Agent, Body: Told(q, r, tail), Kind: "say"})
	return err
}

// bangFrom is where an inline `! <command>` span's block number starts (shell.open's block, contract terminal.json).
const bangFrom = 1000

// Told is what Tell says: one line with the command and how it ended, then the window's last rows. A stopped run's window was closed
// before the command ended, so it has no exit code and no output.
func Told(q db.Q, r Run, tail []string) string {
	b, _ := ParseBlock(r.Block)
	from := "code block " + strconv.FormatInt(b.N+1, 10)
	if b.N >= bangFrom {
		from = "! command " + strconv.FormatInt(b.N-bangFrom+1, 10)
	}
	if b.Reply {
		from = "your reply's " + from
	} else {
		from += " of " + db.MsgName(q, b.Message)
	}
	command := textsafe.Oneline(r.Command)
	if rs := []rune(command); len(rs) > maxCommand {
		command = string(rs[:maxCommand]) + "…"
	}
	if r.Exit == nil {
		return fmt.Sprintf("The run of %s was stopped: its window was closed before the command ended. Command: %s", from, command)
	}
	line := fmt.Sprintf("The run of %s ended with exit %d after %ds. Command: %s", from, *r.Exit, r.Seconds(), command)
	out := textsafe.Lines(strings.Join(tail, "\n"))
	if rs := []rune(out); len(rs) > maxTail {
		out = "…" + string(rs[len(rs)-maxTail:])
	}
	if strings.TrimSpace(out) == "" {
		return line + "\nIts window showed nothing."
	}
	return line + "\nLast rows of its window:\n" + out
}

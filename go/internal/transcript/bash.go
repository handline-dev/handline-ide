package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

// BashTail is how much of a session's transcript BashCommands reads, from the end.
const BashTail = 8 << 20

// Bash is a person's ! line in a session: the command and what it printed. Done is false until its output is recorded. The transcript
// records no exit code, and a command that succeeds can have its stderr in Stdout, so neither says whether it failed.
type Bash struct {
	ID, At         int64
	Command        string
	Stdout, Stderr string
	Dropped        int64
	Done           bool
}

const (
	bashIn     = "<bash-input>"
	bashInEnd  = "</bash-input>"
	bashOut    = "<bash-stdout>"
	bashMiddle = "</bash-stdout><bash-stderr>"
	bashErrEnd = "</bash-stderr>"
	// bashNoOutput is what the session records in place of an empty stdout.
	bashNoOutput = "(Bash completed with no output)"
)

// bashEscapes undoes what the session does to a command's output, though not to the command.
var bashEscapes = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")

// BashCommands are the ! lines in the last BashTail of a session's transcript, oldest first. An item's number is its input line's place in
// the file, which later writes do not move. A subagent's lines are not the person's and are left out.
func BashCommands(path string) ([]Bash, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := max(fi.Size()-BashTail, 0)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	r := bufio.NewReader(f)
	if offset > 0 { // the read began inside a line
		skipped, err := r.ReadBytes('\n')
		if err != nil {
			return nil, nil
		}
		offset += int64(len(skipped))
	}
	var out []Bash
	byUUID := map[string]int{}
	for {
		line, err := r.ReadBytes('\n')
		if err != nil { // a line with no end is still being written
			return out, nil
		}
		at := offset
		offset += int64(len(line))
		if !bytes.Contains(line, []byte("bash-input")) && !bytes.Contains(line, []byte("bash-stdout")) {
			continue
		}
		var e struct {
			Type      string `json:"type"`
			Sidechain bool   `json:"isSidechain"`
			Timestamp string `json:"timestamp"`
			UUID      string `json:"uuid"`
			Parent    string `json:"parentUuid"`
			Message   struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &e) != nil || e.Type != "user" || e.Sidechain {
			continue
		}
		var text string
		if json.Unmarshal(e.Message.Content, &text) != nil {
			continue
		}
		if strings.HasPrefix(text, bashIn) && strings.HasSuffix(text, bashInEnd) {
			when := time.Now().Unix()
			if ts, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
				when = ts.Unix()
			}
			byUUID[e.UUID] = len(out)
			out = append(out, Bash{ID: ItemID(when, ItemBash, at), At: when, Command: text[len(bashIn) : len(text)-len(bashInEnd)]})
			continue
		}
		i, ok := byUUID[e.Parent]
		if !ok {
			continue
		}
		stdout, stderr, ok := bashOutput(text)
		if !ok {
			continue
		}
		if stdout == bashNoOutput {
			stdout = ""
		}
		kept, droppedOut := FitOutput(bashEscapes.Replace(stdout), ResultMax)
		keptErr, droppedErr := FitOutput(bashEscapes.Replace(stderr), ResultMax)
		out[i].Stdout, out[i].Stderr, out[i].Dropped, out[i].Done = kept, keptErr, droppedOut+droppedErr, true
	}
}

// bashOutput splits an output line.
func bashOutput(text string) (stdout, stderr string, ok bool) {
	if !strings.HasPrefix(text, bashOut) || !strings.HasSuffix(text, bashErrEnd) {
		return "", "", false
	}
	inner := text[len(bashOut) : len(text)-len(bashErrEnd)]
	i := strings.LastIndex(inner, bashMiddle)
	if i < 0 {
		return "", "", false
	}
	return inner[:i], inner[i+len(bashMiddle):], true
}

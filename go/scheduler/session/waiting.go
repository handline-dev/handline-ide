package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/handline/handline/go/kit/jsontext"
)

// An agent stopped at a permission prompt only the founder can answer. Such an agent emits no Stop, so its state stays RUNNING and
// nothing else says a person is wanted. The hook that sees the prompt writes one small file per agent; the hooks that see it
// answered take it away. A file, not a row: the clearing hook is PostToolUse, which fires on every tool call, so what it does when
// nothing is waiting has to be one stat and no database. That only clears the one tool call it names; a prompt answered without ever
// firing its own PostToolUse is caught at display time, in compose.

// ShownChars is how much of a tool's input a wait shows.
const ShownChars = 120

var inputFields = []string{"command", "file_path", "path", "pattern", "url", "query", "description"}

// MovedOnKinds is what proves the agent's own turn continued past the prompt that raised a wait, not merely that some other tool
// call finished. Deliberately narrower than the agent-activity kinds: SubagentStop looks the same shape as PostToolUse, and one of
// several parallel subagents finishing fires it while a sibling is still stuck at a prompt; TeammateIdle is another agent's status
// reaching this session. An `hl` verb only runs once the tool call that ran it has itself returned, and PostCompact is this
// session's own transcript growing; neither is a side effect an unrelated tool call could produce.
var MovedOnKinds = []string{"ask", "cancel", "commit", "raise_decision", "report", "resolve", "withdraw", "no_task", "PostCompact"}

// Wait is the permission prompt an agent is stopped at.
type Wait struct {
	Tool, Command, Reason string
	Since                 int64
	AfterEventID          *int64
	QuestionData          map[string]any
}

func (e *Env) waitFile(agent string) string { return filepath.Join(e.Home, "waiting", agent+".json") }

func printable(s string, n int) string {
	var b []rune
	for _, c := range s {
		if unicode.IsPrint(c) {
			b = append(b, c)
		} else {
			b = append(b, ' ')
		}
	}
	if len(b) > n {
		b = b[:n]
	}
	return string(b)
}

// Summary is the one line that says what the tool was asked to do, control characters flattened so it cannot draw over the screen
// it is shown on.
func Summary(toolInput any) string {
	var text string
	switch in := toolInput.(type) {
	case map[string]any:
		found := false
		for _, k := range inputFields {
			if v, ok := in[k].(string); ok && strings.TrimSpace(v) != "" {
				text, found = v, true
				break
			}
		}
		if !found {
			text = jsontext.Spaced(in, true, true)
		}
	case nil:
	case string:
		text = in
	case bool:
		text = map[bool]string{true: "True", false: "False"}[in]
	default:
		text = fmt.Sprint(in)
	}
	return printable(text, ShownChars)
}

// ReadWait is the agent's wait file, nil when there is none.
func (e *Env) ReadWait(agent string) *Wait {
	b, err := os.ReadFile(e.waitFile(agent))
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil || m == nil {
		return nil
	}
	since, _ := m["since"].(float64)
	if since == 0 {
		return nil
	}
	w := &Wait{Since: int64(since)}
	w.Tool, _ = m["tool"].(string)
	w.Command, _ = m["command"].(string)
	w.Reason, _ = m["reason"].(string)
	if id, ok := m["after_event_id"].(float64); ok {
		v := int64(id)
		w.AfterEventID = &v
	}
	w.QuestionData, _ = m["question_data"].(map[string]any)
	return w
}

// ClearWait is the prompt answered.
func (e *Env) ClearWait(agent string) {
	_ = os.Remove(e.waitFile(agent))
}

// ClearWaitFor clears only the prompt of this tool call: another tool finishing while this one still waits is not an answer.
func (e *Env) ClearWaitFor(agent, tool string, toolInput any) {
	if _, err := os.Stat(e.waitFile(agent)); err != nil {
		return
	}
	if held := e.ReadWait(agent); held != nil && (held.Tool != tool || held.Command != Summary(toolInput)) {
		return
	}
	e.ClearWait(agent)
}

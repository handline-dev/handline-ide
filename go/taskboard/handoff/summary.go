package handoff

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/textsafe"
)

// When a fresh start is about to read a record with no note the agent wrote, Handline writes one from the record's transcript with one
// model call and adds it to the record. The record's facts never depend on it: a call that fails leaves the record as it was.

// SummaryMarker heads a note Handline wrote, where an agent's own note says "written by the agent just before the close".
const SummaryMarker = "written by Handline from the transcript"

const (
	// SummaryBudget is how much of the transcript the call is given, in bytes: the user turns first, then the newest part of the rest.
	SummaryBudget = 32 << 10
	// SummaryTimeout bounds the one call; a wake waits for a note in flight no longer than this.
	SummaryTimeout = 90 * time.Second
	// turnMax is what one user turn or one reply keeps of its text.
	turnMax = 2000
	// lineMax is the longest transcript line read; a longer one (a huge tool result) is skipped.
	lineMax = 16 << 20
)

func summaryKey(agent string) string { return "handoff_summary:" + agent }

// SummaryPrompt is what the call is told, the transcript excerpt after it. Its items are NoteItems, the agent's own ask's.
func SummaryPrompt(excerpt string) string {
	return "An agent's session ended before it could write its handoff note, so you write it from an excerpt of its transcript. " +
		"The agent's next session starts fresh and reads it first. Write only the note, at most " + fmt.Sprint(NoteMax) +
		" characters, these items in order: " + NumberedItems(NoteItems) + ". " +
		"A user turn is the founder's own words unless Handline delivered it: a delivery begins with a message or task id (msg_..., T<n>), " +
		"or tells the agent it was woken, restarted, moved or given a fresh session, or asks for its handoff. Copy the founder's words " +
		"verbatim with their times; never paraphrase them. Mark anything you infer rather than read with '(guess)'. " +
		"Cite T and # numbers instead of restating them. If the excerpt shows nothing for an item, write 'none' for it.\n\n" +
		"<transcript-excerpt>\n" + excerpt + "\n</transcript-excerpt>"
}

type entry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	IsMeta    bool   `json:"isMeta"`
	Sidechain bool   `json:"isSidechain"`
	Message   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	IsError bool            `json:"is_error"`
	Content json.RawMessage `json:"content"`
}

func blocks(raw json.RawMessage) []block {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []block{{Type: "text", Text: s}}
	}
	var bs []block
	_ = json.Unmarshal(raw, &bs) // content in no shape we know reads as nothing
	return bs
}

func clipText(s string) string {
	s = textsafe.Strip(textsafe.Text(strings.ToValidUTF8(s, "�")))
	if r := []rune(s); len(r) > turnMax {
		return string(r[:turnMax]) + " (cut)"
	}
	return s
}

// resultText is a tool result's text, for an error's first line.
func resultText(raw json.RawMessage) string {
	var parts []string
	for _, b := range blocks(raw) {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return textsafe.Oneline(cutRunes(strings.Join(parts, " "), 200))
}

// SummaryInput is the excerpt the call reads, at most budget bytes: the newest user turns that were typed (not a tool result, a meta
// line or a subagent's), verbatim and timed, in up to half the budget; then the newest replies, tool calls by name and failed tool
// results by their first line in the rest. Tool results' content is never in it. Only what fits the budget is held while it reads.
func SummaryInput(ctx context.Context, path string, budget int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	typed, rest := &window{max: budget / 2}, &window{max: budget}
	r := bufio.NewReaderSize(f, 1<<16)
	for n := 0; ; n++ {
		if n%1000 == 0 && ctx.Err() != nil {
			return "", ctx.Err()
		}
		line, err := readLine(r)
		if len(line) > 0 {
			appendEntry(line, typed, rest)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	head := typed.newest(typed.max)
	used := len(head) + len("user turns:\n") + len("\nthe end of the session:\n")
	return "user turns:\n" + head + "\nthe end of the session:\n" + rest.newest(budget-used), nil
}

// window keeps the newest lines whose bytes fit in max.
type window struct {
	lines []string
	size  int
	max   int
}

func (w *window) add(line string) {
	w.lines = append(w.lines, line)
	w.size += len(line) + 1
	drop := 0
	for w.size > w.max && drop < len(w.lines) {
		w.size -= len(w.lines[drop]) + 1
		drop++
	}
	w.lines = w.lines[drop:]
}

// newest is the last lines that fit in budget bytes, in their order.
func (w *window) newest(budget int) string {
	n, used := len(w.lines), 0
	for n > 0 && used+len(w.lines[n-1])+1 <= budget {
		used += len(w.lines[n-1]) + 1
		n--
	}
	return strings.Join(w.lines[n:], "\n")
}

// readLine is the next line, or nil for one over lineMax, which is read past.
func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	over := false
	for {
		part, err := r.ReadSlice('\n')
		if !over {
			buf = append(buf, part...)
			if len(buf) > lineMax {
				buf, over = nil, true
			}
		}
		if err != bufio.ErrBufferFull {
			return buf, err
		}
	}
}

func appendEntry(line []byte, typed, rest *window) {
	var e entry
	if json.Unmarshal(line, &e) != nil || e.IsMeta || e.Sidechain {
		return
	}
	at := e.Timestamp
	if t, err := time.Parse(time.RFC3339Nano, e.Timestamp); err == nil {
		at = t.Local().Format("01-02 15:04")
	}
	switch e.Type {
	case "user":
		bs := blocks(e.Message.Content)
		var text []string
		for _, b := range bs {
			switch {
			case b.Type == "tool_result" && b.IsError:
				rest.add(fmt.Sprintf("[%s] tool error: %s", at, resultText(b.Content)))
			case b.Type == "text" && b.Text != "":
				text = append(text, b.Text)
			}
		}
		if len(text) > 0 && !strings.HasPrefix(text[0], "<") { // a hook's or the CLI's own wrapper, not a typed turn
			typed.add(fmt.Sprintf("[%s] %s", at, clipText(strings.Join(text, "\n"))))
		}
	case "assistant":
		for _, b := range blocks(e.Message.Content) {
			switch {
			case b.Type == "text" && strings.TrimSpace(b.Text) != "":
				rest.add(fmt.Sprintf("[%s] agent: %s", at, clipText(b.Text)))
			case b.Type == "tool_use":
				rest.add(fmt.Sprintf("[%s] tool: %s", at, textsafe.Oneline(b.Name)))
			}
		}
	}
}

// Summarizer writes notes off the tick, one at a time per agent. Run is the one model call.
type Summarizer struct {
	Run func(ctx context.Context, agent, prompt string) (string, error)
	Log func(format string, a ...any)

	mu       sync.Mutex
	inflight map[string]bool
	stopped  bool
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

// Pending is whether a note for agent is being written now.
func (s *Summarizer) Pending(agent string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inflight[agent]
}

// Stop ends the calls in flight, starts no more, and returns once every note being written has ended.
func (s *Summarizer) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stopped = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Summarizer) log(format string, a ...any) {
	if s.Log != nil {
		s.Log(format, a...)
	}
}

// start runs write for agent unless one runs already or it was stopped; whether it runs now.
func (s *Summarizer) start(agent string, write func(ctx context.Context)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return false
	}
	if s.inflight[agent] {
		return true
	}
	if s.inflight == nil {
		s.inflight = map[string]bool{}
		s.ctx, s.cancel = context.WithCancel(context.Background())
	}
	s.inflight[agent] = true
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.inflight, agent)
			s.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(s.ctx, SummaryTimeout)
		defer cancel()
		write(ctx)
	}()
	return true
}

// noteHeadingRE is a note's heading line in a record, the agent's or Handline's.
var noteHeadingRE = regexp.MustCompile(`(?m)^(` + regexp.QuoteMeta(agentNoteHeading) + `|` + regexp.QuoteMeta(SummaryMarker+":") + `)$`)

// recordSessionRE is the record's session line (sessionLines).
var recordSessionRE = regexp.MustCompile(`(?m)^previous session: (\S+), recorded at (.+)$`)

// NoteFromTranscript is whether a fresh start should wait for the note Handline is writing into the agent's record: true while one is
// being written, starting it when the record has no note and its session was not tried yet. Each session is tried once.
func (e *Env) NoteFromTranscript(agent string) bool {
	s := e.Summary
	if s == nil || s.Run == nil {
		return false
	}
	if s.Pending(agent) {
		return true
	}
	record, err := db.GetHandoff(e.DB, agent)
	if err != nil || record == "" || noteHeadingRE.MatchString(record) {
		return false
	}
	m := recordSessionRE.FindStringSubmatch(record)
	if m == nil {
		return false
	}
	session, path := m[1], m[2]
	if tried, _, err := db.Meta(e.DB, summaryKey(agent)); err != nil || tried == session {
		return false
	}
	return s.start(agent, func(ctx context.Context) {
		err := e.WriteSummary(ctx, agent, record, path)
		if errors.Is(ctx.Err(), context.Canceled) {
			return // stopped with the daemon: the next one tries again
		}
		if err := db.SetMeta(e.DB, summaryKey(agent), session); err != nil {
			s.log("  %s: the transcript note's try was not recorded: %v", agent, err)
		}
		if err != nil {
			s.log("  %s: no handoff note written from its transcript: %v", agent, err)
			return
		}
		s.log("  %s: handoff note written from its transcript", agent)
	})
}

// errTaken is a record that changed or was handed on before the note was in.
var errTaken = errors.New("the record changed before the note was in")

// WriteSummary writes the note from the transcript at path and adds it to the agent's record, which must still be record. An error leaves
// the record as it was.
func (e *Env) WriteSummary(ctx context.Context, agent, record, path string) error {
	excerpt, err := SummaryInput(ctx, path, SummaryBudget)
	if err != nil {
		return err
	}
	out, err := e.Summary.Run(ctx, agent, SummaryPrompt(excerpt))
	if err != nil {
		return err
	}
	body := textsafe.Strip(textsafe.Text(strings.ToValidUTF8(out, "�")))
	if body == "" {
		return errors.New("the model wrote nothing")
	}
	if r := []rune(body); len(r) > NoteMax {
		body = strings.TrimRightFunc(string(r[:NoteMax]), textsafe.IsSpace) + fmt.Sprintf("\n(cut at %d characters)", NoteMax)
	}
	lines := []string{SummaryMarker + ":"}
	for _, l := range textsafe.SplitLines(body) {
		lines = append(lines, "  "+l)
	}
	text := record + "\n" + strings.Join(lines, "\n")
	if ok, err := db.ReplaceHandoff(e.DB, agent, record, text); err != nil {
		return err
	} else if !ok {
		return errTaken
	}
	return e.archive(agent, text)
}

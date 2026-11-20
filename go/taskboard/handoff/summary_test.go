package handoff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
)

func jsonl(t *testing.T, path string, entries ...map[string]any) {
	t.Helper()
	var b strings.Builder
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func userTurn(at, text string) map[string]any {
	return map[string]any{"type": "user", "timestamp": at, "message": map[string]any{"content": text}}
}

func toolResult(at, text string, isErr bool) map[string]any {
	return map[string]any{"type": "user", "timestamp": at, "message": map[string]any{"content": []any{
		map[string]any{"type": "tool_result", "is_error": isErr, "content": []any{map[string]any{"type": "text", "text": text}}}}}}
}

func reply(at string, parts ...map[string]any) map[string]any {
	content := make([]any, len(parts))
	for i, p := range parts {
		content[i] = p
	}
	return map[string]any{"type": "assistant", "timestamp": at, "message": map[string]any{"content": content}}
}

func TestTheExcerptKeepsTypedTurnsVerbatimAndDropsToolResults(t *testing.T) {
	path := t.TempDir() + "/s.jsonl"
	meta := userTurn("2026-11-18T01:00:00Z", "meta line")
	meta["isMeta"] = true
	side := reply("2026-11-18T01:00:00Z", map[string]any{"type": "text", "text": "a subagent's words"})
	side["isSidechain"] = true
	jsonl(t, path,
		userTurn("2026-11-18T01:00:00Z", "좋아 커밋."),
		meta, side,
		userTurn("2026-11-18T01:01:00Z", "<system-reminder>a hook's line</system-reminder>"),
		reply("2026-11-18T01:02:00Z", map[string]any{"type": "text", "text": "I will commit."}, map[string]any{"type": "tool_use", "name": "Bash"}),
		toolResult("2026-11-18T01:03:00Z", "SECRET FILE CONTENT", false),
		toolResult("2026-11-18T01:04:00Z", "exit 1: tests failed", true),
	)
	got, err := SummaryInput(context.Background(), path, SummaryBudget)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"] 좋아 커밋.", "agent: I will commit.", "tool: Bash", "tool error: exit 1: tests failed"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	for _, not := range []string{"SECRET FILE CONTENT", "meta line", "a subagent's words", "a hook's line"} {
		if strings.Contains(got, not) {
			t.Errorf("%q is in the excerpt:\n%s", not, got)
		}
	}
}

func TestTheExcerptStaysInItsBudgetKeepingTheNewest(t *testing.T) {
	path := t.TempDir() + "/s.jsonl"
	var entries []map[string]any
	for i := 0; i < 400; i++ {
		entries = append(entries, userTurn("2026-11-18T01:00:00Z", "turn "+strings.Repeat("x", 300)+" #"+string(rune('A'+i%26))))
		entries = append(entries, reply("2026-11-18T01:00:00Z", map[string]any{"type": "text", "text": strings.Repeat("y", 300)}))
	}
	entries = append(entries, userTurn("2026-11-18T02:00:00Z", "the newest turn"), reply("2026-11-18T02:00:00Z", map[string]any{"type": "text", "text": "the last reply"}))
	jsonl(t, path, entries...)
	got, err := SummaryInput(context.Background(), path, 8<<10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 8<<10 {
		t.Errorf("%d bytes over a budget of %d", len(got), 8<<10)
	}
	if !strings.Contains(got, "the newest turn") || !strings.Contains(got, "the last reply") {
		t.Errorf("the newest were not kept:\n%s", got[len(got)-300:])
	}
}

func TestTheSummaryPromptAsksEveryNoteItem(t *testing.T) {
	p := SummaryPrompt("excerpt")
	for _, item := range NoteItems {
		if !strings.Contains(p, item) {
			t.Errorf("the prompt misses %q", item)
		}
	}
	for _, want := range []string{"verbatim", "(guess)", "next: none", "excerpt"} {
		if !strings.Contains(p, want) {
			t.Errorf("the prompt misses %q", want)
		}
	}
}

// summarized is a world whose agent "a" has a closed session s1 with a transcript, and a record taken of it with no note.
func summarized(t *testing.T, run func(context.Context, string, string) (string, error)) *world {
	w := newWorld(t)
	w.agent("a", "IDLE", "alpha", "")
	path := w.transcript("alpha", "-p", "s1")
	jsonl(t, path, userTurn("2026-11-18T01:00:00Z", "carry on with T1"))
	w.status["a"] = [2]string{"s1", path}
	if _, err := w.Record("a", "s1"); err != nil {
		t.Fatal(err)
	}
	w.Summary = &Summarizer{Run: run}
	return w
}

// fromTranscript asks for the note as a fresh start does and waits for it.
func fromTranscript(t *testing.T, w *world) bool {
	t.Helper()
	started := w.NoteFromTranscript("a")
	w.Summary.wg.Wait()
	return started
}

func TestAFailedSummaryLeavesTheRecordAsItWas(t *testing.T) {
	for name, run := range map[string]func(context.Context, string, string) (string, error){
		"error":   func(context.Context, string, string) (string, error) { return "", errors.New("walled") },
		"nothing": func(context.Context, string, string) (string, error) { return "  ", nil },
	} {
		w := summarized(t, run)
		today := recordOf(t, w)
		var logs []string
		w.Summary.Log = func(f string, a ...any) { logs = append(logs, f) }
		if !fromTranscript(t, w) {
			t.Fatalf("%s: not started", name)
		}
		if got := recordOf(t, w); today == "" || got != today {
			t.Errorf("%s: the record is %q, not %q", name, got, today)
		}
		if len(logs) != 1 || !strings.Contains(logs[0], "no handoff note written") {
			t.Errorf("%s: logs %q", name, logs)
		}
	}
}

// A failed call is tried again SummaryRetryS later, and the fresh start waits for it meanwhile; the note it then writes is the record's.
func TestAFailedSummaryIsTriedAgainAndWritten(t *testing.T) {
	calls := 0
	w := summarized(t, func(context.Context, string, string) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("no account has room")
		}
		return "(4) next: none", nil
	})
	fromTranscript(t, w)
	w.now += SummaryRetryS - 1
	if !fromTranscript(t, w) || calls != 1 {
		t.Fatalf("before the backoff: calls %d, or the start did not wait", calls)
	}
	w.now++
	if !fromTranscript(t, w) || calls != 2 {
		t.Fatalf("after the backoff: calls %d", calls)
	}
	if got := recordOf(t, w); !strings.HasSuffix(got, SummaryMarker+":\n  (4) next: none") {
		t.Fatalf("record %q", got)
	}
	if fromTranscript(t, w) || calls != 2 {
		t.Errorf("tried after the note was in: %d calls", calls)
	}
}

func TestSummaryTriesStopAtTheCap(t *testing.T) {
	calls := 0
	w := summarized(t, func(context.Context, string, string) (string, error) { calls++; return "", errors.New("timeout") })
	var logs []string
	w.Summary.Log = func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }
	for i := 0; i < SummaryTries; i++ {
		if !fromTranscript(t, w) {
			t.Fatalf("try %d not started", i+1)
		}
		w.now += SummaryRetryS
	}
	if fromTranscript(t, w) || calls != SummaryTries {
		t.Fatalf("past the cap: %d calls", calls)
	}
	if last := logs[len(logs)-1]; !strings.Contains(last, fmt.Sprintf("after %d tries", SummaryTries)) {
		t.Errorf("the last log %q", last)
	}
}

// A newer session's record gets its own tries, whatever the last one used.
func TestANewSessionGetsItsOwnSummaryTries(t *testing.T) {
	w := summarized(t, func(context.Context, string, string) (string, error) { return "", errors.New("timeout") })
	if err := db.SetMeta(w.DB, summaryKey("a"), fmt.Sprintf("s0 %d %d", SummaryTries, w.now)); err != nil {
		t.Fatal(err)
	}
	if !fromTranscript(t, w) {
		t.Fatal("s1 was not tried")
	}
}

// A value from before the tries were counted is a session done.
func TestAnOldSummaryValueReadsAsDone(t *testing.T) {
	called := false
	w := summarized(t, func(context.Context, string, string) (string, error) { called = true; return "x", nil })
	if err := db.SetMeta(w.DB, summaryKey("a"), "s1"); err != nil {
		t.Fatal(err)
	}
	if fromTranscript(t, w) || called {
		t.Fatal("a done session was tried again")
	}
}

func TestANoteIsWrittenFromTheTranscriptWhenTheRecordHasNone(t *testing.T) {
	var prompts []string
	w := summarized(t, func(_ context.Context, agent, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		return "(1) none\n(4) next: none", nil
	})
	if !fromTranscript(t, w) {
		t.Fatal("not started")
	}
	got := recordOf(t, w)
	if len(prompts) != 1 || !strings.Contains(prompts[0], "carry on with T1") {
		t.Fatalf("prompts %q", prompts)
	}
	if !strings.Contains(got, "previous session: s1") || !strings.HasSuffix(got, "\n"+SummaryMarker+":\n  (1) none\n  (4) next: none") {
		t.Fatalf("record %q", got)
	}
	if fromTranscript(t, w) || len(prompts) != 1 {
		t.Errorf("summarized twice: %d calls", len(prompts))
	}
}

func TestARecordWithTheAgentsOwnNoteOrNoneIsNotSummarized(t *testing.T) {
	called := false
	run := func(context.Context, string, string) (string, error) { called = true; return "x", nil }
	w := summarized(t, run)
	db.SetHandoff(w.DB, "a", "previous session: s1, recorded at /t.jsonl\n"+agentNoteHeading+"\n  mine")
	if fromTranscript(t, w) {
		t.Error("an agent's own note was summarized")
	}
	db.ClearHandoff(w.DB, "a")
	if fromTranscript(t, w) || called {
		t.Error("no record was summarized")
	}
}

// A record that changed while the call ran (a wake cleared it, or a newer close replaced it) is not written over.
func TestANoteForARecordThatChangedIsDropped(t *testing.T) {
	var w *world
	w = summarized(t, func(context.Context, string, string) (string, error) {
		db.SetHandoff(w.DB, "a", "a newer record")
		return "note", nil
	})
	fromTranscript(t, w)
	if got := recordOf(t, w); got != "a newer record" {
		t.Fatalf("record %q", got)
	}
	if v, _, _ := db.Meta(w.DB, summaryKey("a")); v != "s1" {
		t.Fatalf("a taken record did not end the tries: %q", v)
	}
	w = summarized(t, func(context.Context, string, string) (string, error) {
		db.ClearHandoff(w.DB, "a")
		return "note", nil
	})
	fromTranscript(t, w)
	if got := recordOf(t, w); got != "" {
		t.Fatalf("a cleared record came back: %q", got)
	}
}

// A call the daemon's stop cut short is not counted as the session's one try.
func TestACallCutShortByStopIsTriedAgain(t *testing.T) {
	var w *world
	w = summarized(t, func(ctx context.Context, _, _ string) (string, error) {
		go w.Summary.Stop()
		<-ctx.Done()
		return "", ctx.Err()
	})
	fromTranscript(t, w)
	if tried, _, _ := db.Meta(w.DB, summaryKey("a")); tried != "" {
		t.Fatalf("marked tried: %q", tried)
	}
}

func TestAStoppedSummarizerStartsNothing(t *testing.T) {
	w := summarized(t, func(context.Context, string, string) (string, error) { return "x", nil })
	w.Summary.Stop()
	if w.NoteFromTranscript("a") || strings.Contains(recordOf(t, w), SummaryMarker) {
		t.Fatal("started after Stop")
	}
}

func recordOf(t *testing.T, w *world) string {
	t.Helper()
	var text string
	if err := w.DB.QueryRow("SELECT COALESCE(handoff, '') FROM agent_state WHERE agent = 'a'").Scan(&text); err != nil {
		t.Fatal(err)
	}
	return text
}

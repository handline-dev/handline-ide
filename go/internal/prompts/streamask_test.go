package prompts

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
)

type streamed struct {
	a   agent.AskAnswer
	ok  bool
	err error
}

// askStream asks as backend's Chat session in the background, and waits until its prompt is the open one.
func askStream(t *testing.T, ctx context.Context, e *Env, tool string, input map[string]any) (<-chan streamed, *Block) {
	t.Helper()
	out := make(chan streamed, 1)
	go func() {
		a, ok, err := e.AskStream(ctx, backendKey(e), tool, input)
		out <- streamed{a, ok, err}
	}()
	for i := 0; i < 200; i++ {
		if b, _ := e.OpenBlockOf(backendKey(e)); b != nil {
			return out, b
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no open prompt")
	return nil, nil
}

func got(t *testing.T, ch <-chan streamed) streamed {
	t.Helper()
	select {
	case s := <-ch:
		if s.err != nil {
			t.Fatal(s.err)
		}
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("the ask was not answered")
	}
	return streamed{}
}

func nextOpen(t *testing.T, e *Env, after int64) *Block {
	t.Helper()
	for i := 0; i < 300; i++ {
		if b, _ := e.OpenBlockOf(backendKey(e)); b != nil && b.ID != after {
			return b
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no next prompt")
	return nil
}

func TestAStreamAskIsAPermissionPromptAllowedDeniedOrKeptAsARule(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	agentKey := backendKey(e)
	ctx := context.Background()
	in := map[string]any{"command": "npm test"}

	call, b := askStream(t, ctx, e, "Bash", in)
	if p := openPrompt(t, e, agentKey); p["source"] != "stream" || p["kind"] != "permission" || !slices.Equal(optionKeys(p), []string{"allow", "deny"}) {
		t.Fatalf("%v", p)
	}
	if _, err := e.answer(asRon(), map[string]any{"agent": agentKey, "prompt": b.Fingerprint, "choice": "deny", "note": "run the script"}); err != nil {
		t.Fatal(err)
	}
	if s := got(t, call); !s.ok || s.a.Allow || s.a.Message != "The founder said no: run the script" {
		t.Fatalf("%+v", s)
	}

	call, b = askStream(t, ctx, e, "Bash", in) // asked before: Always allow is offered
	if k := optionKeys(openPrompt(t, e, agentKey)); !slices.Equal(k, []string{"allow", "always", "deny"}) {
		t.Fatal(k)
	}
	if _, err := e.answer(asOwner(), map[string]any{"agent": agentKey, "prompt": b.Fingerprint, "choice": "always"}); err != nil {
		t.Fatal(err)
	}
	if s := got(t, call); !s.a.Allow || !slices.Equal(s.a.Rules, []agent.Rule{{Tool: "Bash", Content: "npm test"}}) {
		t.Fatalf("%+v", s)
	}

	call, b = askStream(t, ctx, e, "Bash", map[string]any{"command": "ls"})
	answers(e, b.Fingerprint, "allow")
	if s := got(t, call); !s.a.Allow || s.a.Rules != nil || s.a.Input != nil {
		t.Fatalf("%+v", s)
	}
}

func TestAStreamAskWaitsUntilTheSessionGoesAndThenClosesItsPrompt(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	e.Refresh = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	e.DB.Exec("INSERT INTO events(ts, agent, kind) VALUES(?, ?, 'Stop')", time.Now().Unix()+5, backendKey(e))
	call, _ := askStream(t, ctx, e, "Bash", map[string]any{"command": "ls"})
	time.Sleep(100 * time.Millisecond)
	if left, _ := e.OpenOf(backendKey(e)); left == nil {
		t.Fatal("a turn event ended a stream ask: only its session's end does")
	}
	cancel()
	if s := got(t, call); s.ok {
		t.Fatalf("%+v", s)
	}
	if left, _ := e.OpenOf(backendKey(e)); left != nil {
		t.Fatal("the prompt of a gone session stays")
	}
}

func TestAQuestionIsAnsweredWithAnOptionSeveralOrText(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	agentKey := backendKey(e)
	in := map[string]any{"questions": []any{
		map[string]any{"question": "Pick a color", "options": []any{map[string]any{"label": "Red"}, map[string]any{"label": "Blue"}}},
		map[string]any{"question": "Which sizes?", "multiSelect": true, "options": []any{map[string]any{"label": "S"}, map[string]any{"label": "M"},
			map[string]any{"label": "L"}}},
		map[string]any{"question": "Anything else?", "options": []any{map[string]any{"label": "No"}}},
	}}
	call, b := askStream(t, context.Background(), e, "AskUserQuestion", in)
	if p := openPrompt(t, e, agentKey); p["kind"] != "question" || p["source"] != "stream" || p["multi"] != false ||
		!slices.Equal(optionKeys(p), []string{"1", "2", "esc"}) {
		t.Fatalf("%v", p)
	}
	refusal(t, mustErr(answers(e, b.Fingerprint, "3")), "bad_request")
	refusal(t, mustErr(answers(e, b.Fingerprint, []any{"1"})), "bad_request") // not a multi-select
	refusal(t, mustErr(e.answer(asRon(), map[string]any{"agent": agentKey, "prompt": b.Fingerprint, "choice": "esc", "note": "x"})), "bad_request")
	answers(e, b.Fingerprint, "2")
	b = nextOpen(t, e, b.ID)
	if p := openPrompt(t, e, agentKey); p["multi"] != true {
		t.Fatalf("%v", p)
	}
	// The row is in a database agents can write: a label put in it is not the answer sent.
	e.DB.Exec("UPDATE prompt_blocks SET detail = json_set(detail, '$.options[0].label', 'XL') WHERE id = ?", b.ID)
	answers(e, b.Fingerprint, []any{"1", "3", "1"})
	b = nextOpen(t, e, b.ID)
	answers(e, b.Fingerprint, map[string]any{"text": " Gift wrap "})
	s := got(t, call)
	want := map[string]any{"Pick a color": "Blue", "Which sizes?": "S, L", "Anything else?": "Gift wrap"}
	ans, _ := s.a.Input["answers"].(map[string]any)
	if !s.a.Allow || len(ans) != 3 || ans["Pick a color"] != want["Pick a color"] || ans["Which sizes?"] != want["Which sizes?"] ||
		ans["Anything else?"] != want["Anything else?"] || s.a.Input["questions"] == nil {
		t.Fatalf("%+v", s.a)
	}
	if _, has := in["answers"]; has {
		t.Fatal("the ask's own input was changed")
	}
	var label, note string
	e.DB.QueryRow("SELECT answer_label, answer_note FROM prompt_blocks WHERE id = ?", b.ID).Scan(&label, &note)
	if label != "Answered" || note != "Gift wrap" {
		t.Fatal(label, note)
	}

	call, b = askStream(t, context.Background(), e, "AskUserQuestion", in)
	answers(e, b.Fingerprint, "esc")
	if s := got(t, call); s.a.Allow || s.a.Message == "" {
		t.Fatalf("a cancelled question denies the tool: %+v", s.a)
	}
	if a, ok, _ := e.AskStream(context.Background(), agentKey, "AskUserQuestion", map[string]any{"questions": []any{map[string]any{}}}); !ok || a.Allow {
		t.Fatalf("a question with no text is allowed unasked: %+v", a)
	}
}

func TestAChatAgentsHookAsksNothing(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	register(t, e)
	e.Chat = func(a string) bool { return a == backendKey(e) }
	r, err := e.permissionAsk(asBackend(e), map[string]any{"tool": "Bash", "input": map[string]any{"command": "ls"}})
	if err != nil || r.(map[string]any)["behavior"] != nil {
		t.Fatal(r, err)
	}
	if b, _ := e.OpenOf(backendKey(e)); b != nil {
		t.Fatal("the hook raised a second prompt")
	}
}

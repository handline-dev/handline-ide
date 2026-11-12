package codex

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent"
	"github.com/handline/handline/go/agentctl/delivery"
)

type outcomes struct {
	mu        sync.Mutex
	typed     []string
	delivered []string
	failed    []string
	done      chan struct{}
}

func (o *outcomes) add(to *[]string, s string) {
	o.mu.Lock()
	*to = append(*to, s)
	o.mu.Unlock()
	o.done <- struct{}{}
}

// port is a codex Port over a session in config at cwd: typing appends what the session would record of the text when echo is true.
func port(t *testing.T, config, cwd, file string, echo bool) (*Port, *outcomes) {
	o := &outcomes{done: make(chan struct{}, 4)}
	env := agent.DeliverEnv{
		Where: func(string) (agent.Place, error) { return agent.Place{Config: config, Cwd: cwd}, nil },
		TypeIn: func(_, text string) error {
			o.mu.Lock()
			o.typed = append(o.typed, text)
			o.mu.Unlock()
			if echo {
				line := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}` + "\n"
				f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.WriteString(line)
				return err
			}
			return nil
		},
		OnDelivered: func(_, op string) { o.add(&o.delivered, op) },
		OnFailed: func(_, op, why string, written bool) {
			o.add(&o.failed, op+" "+why+" "+map[bool]string{true: "written"}[written])
		},
	}
	p := Driver{}.Deliver(env).(*Port)
	p.poll, p.wait = 5*time.Millisecond, 300*time.Millisecond
	return p, o
}

func wait(t *testing.T, o *outcomes) {
	t.Helper()
	select {
	case <-o.done:
	case <-time.After(5 * time.Second):
		t.Fatal("no outcome")
	}
}

// A typed message is delivered when the transcript records it after the typing; the earlier lines (the same text sent before) do not count.
func TestATypedMessageIsConfirmedByTheTranscript(t *testing.T) {
	config := t.TempDir()
	file := rollout(t, config, "/w", time.Now(), "s")
	p, o := port(t, config, "/w", file, true)
	r, err := p.Send("a1", "op1", "List three prime numbers, then count slowly from 1 to 30, one per line.", delivery.Now)
	if err != nil || r.State != delivery.Sent {
		t.Fatalf("%+v %v", r, err)
	}
	wait(t, o)
	if len(o.delivered) != 1 || o.delivered[0] != "op1" || len(o.failed) != 0 {
		t.Fatalf("%+v", o)
	}
	if again, _ := p.Send("a1", "op1", "x", delivery.Now); again != r || len(o.typed) != 1 {
		t.Fatalf("the same op is typed once: %+v %v", again, o.typed)
	}
}

func TestAMessageTheTranscriptNeverShowsIsWrittenNotConfirmed(t *testing.T) {
	config := t.TempDir()
	file := rollout(t, config, "/w", time.Now(), "s")
	p, o := port(t, config, "/w", file, false)
	if _, err := p.Send("a1", "op1", "hello", delivery.Now); err != nil {
		t.Fatal(err)
	}
	wait(t, o)
	if len(o.failed) != 1 || !strings.HasSuffix(o.failed[0], " written") || len(o.delivered) != 0 {
		t.Fatalf("%+v", o)
	}
}

// What compose's typing refuses (a dialog on screen, no terminal) is an error: nothing was typed, nothing is kept.
func TestARefusedTypingIsAnError(t *testing.T) {
	p, o := port(t, t.TempDir(), "/w", "", false)
	p.env.TypeIn = func(string, string) error { return errors.New("a dialog is open on its screen") }
	if _, err := p.Send("a1", "op1", "hello", delivery.Now); err == nil || !strings.Contains(err.Error(), "a dialog is open") {
		t.Fatalf("%v", err)
	}
	p.env.TypeIn = func(_, text string) error { o.typed = append(o.typed, text); return nil }
	if r, err := p.Send("a1", "op1", "hello", delivery.Now); err != nil || r.State != delivery.Sent || len(o.typed) != 1 {
		t.Fatalf("a retry types: %+v %v %v", r, err, o.typed)
	}
	if !p.Live("a1") || !p.Structured("a1") {
		t.Fatal("live")
	}
}

// A session that starts after the typing (a relaunch in the same worktree) has a newer file: the confirmation is read there.
func TestTheConfirmationFollowsANewerSession(t *testing.T) {
	config := t.TempDir()
	old := rollout(t, config, "/w", time.Now(), "old")
	p, o := port(t, config, "/w", old, false)
	p.env.TypeIn = func(_, text string) error {
		go func() {
			time.Sleep(20 * time.Millisecond)
			fresh := rollout(t, config, "/w", time.Now(), "new")
			later := time.Now().Add(time.Minute)
			os.Chtimes(fresh, later, later)
			f, _ := os.OpenFile(fresh, os.O_APPEND|os.O_WRONLY, 0)
			f.WriteString(`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}` + "\n")
			f.Close()
		}()
		return nil
	}
	if _, err := p.Send("a1", "op1", "ok", delivery.Now); err != nil {
		t.Fatal(err)
	}
	wait(t, o)
	if len(o.delivered) != 1 {
		t.Fatalf("%+v", o)
	}
}

// A short text is matched whole, never inside a longer message or codex's own context records.
func TestOnlyTheWholeTextConfirms(t *testing.T) {
	config := t.TempDir()
	file := rollout(t, config, "/w", time.Now(), "s")
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	defer f.Close()
	for _, text := range []string{"<environment_context>ok</environment_context>", "ok, and more"} {
		f.WriteString(`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}` + "\n")
	}
	if seen, _ := shows(file, 0, "ok"); seen {
		t.Fatal("matched a part")
	}
}

// The wait ends when the session does: the message is settled as written, not watched for the rest of the wait.
func TestTheWaitEndsWithTheSession(t *testing.T) {
	config := t.TempDir()
	p, o := port(t, config, "/w", rollout(t, config, "/w", time.Now(), "s"), false)
	p.wait = time.Hour
	calls := 0
	p.env.Where = func(string) (agent.Place, error) {
		if calls++; calls > 2 {
			return agent.Place{}, agent.ErrNoSession
		}
		return agent.Place{Config: config, Cwd: "/w"}, nil
	}
	if _, err := p.Send("a1", "op1", "hello", delivery.Now); err != nil {
		t.Fatal(err)
	}
	wait(t, o)
	if len(o.failed) != 1 {
		t.Fatalf("%+v", o)
	}
}

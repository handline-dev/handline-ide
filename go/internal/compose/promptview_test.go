package compose

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/prompts"
)

// A card's prompt opening and going rewrites the agent's view then, not at the next tick, and what the hooks and the session file said
// while it waited ends with the answer though they still say it.
func TestAnAgentsViewWaitsWhileItsPromptIsOpenAndRunsOnceItIsAnsweredWithNoTick(t *testing.T) {
	p, _, wt := termTickRig(t)
	p.notified = true
	bob := teamtest.Key(t, p.home, "bob")
	p.Tick(cfgOf(p), TickConfig{Terminals: true}) // builds the view; the loop is never started
	if err := dbpkg.SetAgentState(p.db, bob, "RUNNING", 0, ""); err != nil {
		t.Fatal(err)
	}
	const at = 1_900_000_000
	writeSession(t, p, "a1", os.Getpid(), `{"cwd":"`+wt+`","sessionId":"s1","status":"busy"}`)
	view := func() (string, map[string]any) {
		t.Helper()
		var raw string
		if err := p.db.QueryRow("SELECT view FROM agent_view WHERE agent = ?", bob).Scan(&raw); err != nil {
			t.Fatalf("no row: %v", err)
		}
		var v map[string]any
		json.Unmarshal([]byte(raw), &v)
		s, _ := v["state"].(string)
		return s, v
	}
	p.RefreshView(bob)
	if s, _ := view(); s != "running" {
		t.Fatalf("before the prompt: %s", s)
	}
	env := &prompts.Env{DB: p.db, Home: p.home, Now: func() time.Time { return time.Unix(at, 0) }, Refresh: 10 * time.Millisecond,
		Changed: p.RefreshView}
	// What claude does at the prompt besides the hook's question: its session file says waiting, the PermissionRequest hook leaves its
	// wait file and the Notification hook says nobody answered. None of them is taken back when the card is answered.
	writeSession(t, p, "a1", os.Getpid(), `{"cwd":"`+wt+`","sessionId":"s1","status":"waiting","waitingFor":"permission prompt","updatedAt":`+
		strconv.Itoa((at-1)*1000)+`}`)
	put := func(dir, body string) {
		os.MkdirAll(filepath.Join(p.home, dir), 0o755)
		if err := os.WriteFile(filepath.Join(p.home, dir, bob+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("waiting", `{"tool":"Bash","command":"cd `+wt+`; go test ./...","since":`+strconv.Itoa(at)+`}`)
	put("needs-you", `{"since":`+strconv.Itoa(at)+`}`)

	type result struct {
		d        prompts.Decision
		answered bool
		err      error
	}
	done := make(chan result, 1)
	go func() {
		d, answered, err := env.AskPermission(context.Background(), bob, "Bash", map[string]any{"command": "cd " + wt + "; go test ./..."})
		done <- result{d, answered, err}
	}()
	var id int64
	for deadline := time.Now().Add(5 * time.Second); ; {
		b, err := env.OpenBlockOf(bob)
		if err != nil {
			t.Fatal(err)
		}
		if b != nil {
			if s, _ := view(); s == "waiting" { // the open told the view, after the row was in
				id = b.ID
				break
			}
		}
		if time.Now().After(deadline) {
			s, v := view()
			t.Fatalf("the prompt never showed in the view: %s %v", s, v)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, v := view(); v["state_detail"] != "Bash: go test ./..." {
		t.Fatalf("the ask's line: %v", v["state_detail"])
	}
	if ok, said := env.Answer(id, prompts.Choice{Yes: true}); !ok {
		t.Fatal(said)
	}
	r := <-done
	if r.err != nil || !r.answered || !r.d.Allow {
		t.Fatalf("%+v", r)
	}
	// AskPermission has returned: the row is gone and the view written, with no tick between.
	if s, v := view(); s != "running" || v["needs_you"] != nil {
		t.Fatalf("after the answer: %s %v", s, v)
	}
	// A new prompt the session reports after the answer needs a person again.
	writeSession(t, p, "a1", os.Getpid(), `{"cwd":"`+wt+`","sessionId":"s1","status":"waiting","waitingFor":"permission prompt","updatedAt":`+
		strconv.Itoa((at+5)*1000)+`}`)
	p.RefreshView(bob)
	if s, _ := view(); s != "waiting" {
		t.Fatalf("a prompt raised after the answer: %s", s)
	}
}

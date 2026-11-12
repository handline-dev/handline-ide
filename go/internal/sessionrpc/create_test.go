package sessionrpc

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/protocol/operations"
)

type seen struct {
	mu  sync.Mutex
	got []map[string]any
}

func (s *seen) emit(_, typ string, data any, _ string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if typ == "operation.progress" {
		s.got = append(s.got, data.(map[string]any))
	}
}

func (s *seen) states() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, d := range s.got {
		out = append(out, d["state"].(string))
	}
	return out
}

func createRig(t *testing.T) (*rig, *seen, *conn) {
	for _, k := range []string{"HANDLINE_AGENT"} { // Register is the founder's: not from inside an agent's session
		t.Setenv(k, "")
	}
	r := newRig(t)
	sn := &seen{}
	r.e.Roster, r.e.Emit = team.NewRoster(r.e.Home), sn.emit
	return r, sn, r.dial(false)
}

// waitEnded waits until the operation's row has left running and its goroutine has returned.
func waitEnded(t *testing.T, r *rig, op string) {
	t.Helper()
	for i := 0; i < 250; i++ {
		var state string
		if err := r.e.DB.QueryRow("SELECT state FROM operations WHERE id = ?", op).Scan(&state); err == nil && state != "running" && !operations.Live(op) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the operation did not end")
}

func TestCreateRegistersAndStartsAndTellsEachStep(t *testing.T) {
	r, sn, c := createRig(t)
	repo := checkout(t)
	got := result(t, c.call("agent.create", map[string]any{"name": "ann.new", "role": "worker", "repo": repo, "parent": "ann", "effort": "high", "request_id": rid("c1")}))
	op, _ := got["operation"].(string)
	if got["accepted"] != true || op == "" || got["said"] != "done" || operations.Live(op) {
		t.Fatalf("answered once the agent is up, with the start's words: %v", got)
	}
	if len(sn.got) > 0 && sn.got[len(sn.got)-1]["state"] == "failed" {
		t.Fatalf("%v", sn.got[len(sn.got)-1])
	}
	if r.ctl.calls[len(r.ctl.calls)-1] != "start ann.new "+repo+"  true" {
		t.Fatalf("started through the session path: %v", r.ctl.calls)
	}
	cfg, err := team.NewRoster(r.e.Home).Read("ann.new")
	if err != nil || cfg["effort"] != "high" || cfg["repo"] != repo {
		t.Fatalf("registered with what was asked: %v %v", cfg, err)
	}
	if st := sn.states(); st[0] != "running" || st[len(st)-1] != "done" {
		t.Fatalf("%v", st)
	}
	last := sn.got[len(sn.got)-1]
	if res := last["result"].(map[string]any); res["agent"] != "ann.new" || res["registered"] != true || last["agent"] != "ann.new" {
		t.Fatalf("%v", last)
	}
	again := result(t, c.call("agent.create", map[string]any{"name": "ann.new", "role": "worker", "repo": repo, "parent": "ann", "effort": "high", "request_id": rid("c1")}))
	if again["replayed"] != true || again["operation"] != op || len(r.ctl.calls) != 1 {
		t.Fatalf("a retry starts nothing twice: %v %v", again, r.ctl.calls)
	}
}

// A start that fails is the call's refusal, with the operation and whether the agent was registered; nothing is cleaned up.
func TestCreateWhoseStartFailsIsRefusedAndLeavesTheAgentRegistered(t *testing.T) {
	r, sn, c := createRig(t)
	r.ctl.ok, r.ctl.said = false, "diverged"
	repo := checkout(t)
	m := c.call("agent.create", map[string]any{"name": "solo", "role": "worker", "repo": repo, "request_id": rid("c2")})
	e, _ := m["error"].(map[string]any)
	d, _ := e["data"].(map[string]any)
	if e["message"] != "solo is registered; starting it failed: diverged" || d["registered"] != true || d["operation"] == nil {
		t.Fatalf("refused with how far it got: %v", m)
	}
	if last := sn.got[len(sn.got)-1]; last["state"] != "failed" {
		t.Fatalf("the operations topic says the same: %v", last)
	}
	if _, err := team.NewRoster(r.e.Home).Read("solo"); err != nil {
		t.Fatalf("nothing is cleaned up: %v", err)
	}
	again := c.call("agent.create", map[string]any{"name": "solo", "role": "worker", "repo": repo, "request_id": rid("c2")})
	if e2, _ := again["error"].(map[string]any); e2 == nil || e2["message"] != e["message"] || len(r.ctl.calls) != 1 {
		t.Fatalf("a retry gets the same refusal and starts nothing: %v %v", again, r.ctl.calls)
	}
}

// The call answers only once the agent is up; a start still running at the limit is refused with what it reached, and goes on.
func TestCreateAnswersOnceStartedAndAtTheLimitSaysItGoesOn(t *testing.T) {
	r, _, c := createRig(t)
	r.ctl.hold = make(chan struct{})
	r.e.Wait = 50 * time.Millisecond
	m := c.call("agent.create", map[string]any{"name": "slow", "role": "worker", "repo": checkout(t), "request_id": rid("c3")})
	e, _ := m["error"].(map[string]any)
	d, _ := e["data"].(map[string]any)
	op, _ := d["operation"].(string)
	if msg, _ := e["message"].(string); op == "" || !strings.Contains(msg, "not finished in time (starting slow): nothing was undone, it goes on") {
		t.Fatalf("refused at the limit with what it reached: %v", m)
	}
	close(r.ctl.hold)
	waitEnded(t, r, op)
	if len(r.ctl.calls) != 1 {
		t.Fatalf("the start went on: %v", r.ctl.calls)
	}
}

func TestCreateRefusesWhatItCannotStart(t *testing.T) {
	r, _, c := createRig(t)
	repo := checkout(t)
	ok := map[string]any{"name": "x", "role": "worker", "repo": repo}
	with := func(k string, v any, id string) map[string]any {
		p := map[string]any{"request_id": rid(id)}
		for a, b := range ok {
			p[a] = b
		}
		p[k] = v
		return c.call("agent.create", p)
	}
	if g := refused(with("name", "ann", "u1")); g != "state" {
		t.Fatalf("a used name: %q", g)
	}
	if f := badField(with("parent", "ann", "u2")); f != "name" {
		t.Fatalf("a name outside its parent: %q", f)
	}
	if f := badField(with("effort", "extreme", "u3")); f != "effort" {
		t.Fatalf("%q", f)
	}
	if f := badField(with("account", "a1", "u4")); f != "account" {
		t.Fatalf("%q", f)
	}
	if f := badField(with("name", "a/b", "u5")); f != "name" {
		t.Fatalf("%q", f)
	}
	if m := with("repo", "/no/such/repo", "u6"); m["error"].(map[string]any)["data"].(map[string]any)["what"] != "repo" {
		t.Fatalf("%v", m)
	}
	if m := with("parent", "nobody", "u7"); m["error"].(map[string]any)["data"].(map[string]any)["what"] != "agent" {
		t.Fatalf("%v", m)
	}
	if len(r.ctl.calls) != 0 {
		t.Fatalf("nothing started: %v", r.ctl.calls)
	}
}

func TestCreateRefusesASecondOfTheNameWhileTheFirstRuns(t *testing.T) {
	r, _, c := createRig(t)
	if _, err := r.e.DB.Exec("INSERT INTO operations(id, kind, state, steps_done, agent, created_at, updated_at) VALUES('held', 'agent.create', 'running', 0, 'busy', 0, 0)"); err != nil {
		t.Fatal(err)
	}
	operations.Begin("held")
	defer operations.End("held")
	if g := refused(c.call("agent.create", map[string]any{"name": "busy", "role": "worker", "repo": checkout(t), "request_id": rid("b1")})); g != "state" {
		t.Fatalf("%q", g)
	}
}

func TestCreatePrunesOnlyItsOwnOldOperations(t *testing.T) {
	r, _, c := createRig(t)
	rows := [][2]string{{"send:ann:q", "queued"}, {"send:ann:s", "sent"}, {"send:ann:w", "withdrawn"}, {"send:ann:r", "delivered"}, {"old-create", "done"}}
	for _, row := range rows {
		kind := "send"
		if row[0] == "old-create" {
			kind = createKind
		}
		if _, err := r.e.DB.Exec("INSERT INTO operations(id, kind, state, steps_done, created_at, updated_at) VALUES(?, ?, ?, 0, 0, 0)", row[0], kind, row[1]); err != nil {
			t.Fatal(err)
		}
	}
	op := result(t, c.call("agent.create", map[string]any{"name": "fresh", "role": "worker", "repo": checkout(t), "request_id": rid("p1")}))["operation"].(string)
	waitEnded(t, r, op)
	var n int
	r.e.DB.QueryRow("SELECT count(*) FROM operations WHERE kind = 'send'").Scan(&n)
	if n != 4 {
		t.Fatalf("every send row stays, %d left", n)
	}
	r.e.DB.QueryRow("SELECT count(*) FROM operations WHERE id = 'old-create'").Scan(&n)
	if n != 0 {
		t.Fatal("an old create operation is pruned")
	}
}

func TestCreateChecksTheNameBeforeEffortAndAccount(t *testing.T) {
	_, _, c := createRig(t)
	got := c.call("agent.create", map[string]any{"name": "a/b", "role": "worker", "repo": "/x", "effort": "extreme", "account": "a1", "request_id": rid("n1")})
	if f := badField(got); f != "name" {
		t.Fatalf("field %q", f)
	}
}

// checkout is a new empty git checkout: a workspace's folder is one.
func checkout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	return dir
}

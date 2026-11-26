package sessionrpc

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/testfiles"
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

// startedByKey fails t unless the last start was agent name's, by its key, in repo, through the session path.
func startedByKey(t *testing.T, r *rig, name, repo string) {
	t.Helper()
	if key := paths.AgentKey(r.e.Home, name); key == "" || r.ctl.calls[len(r.ctl.calls)-1] != "start "+key+" "+repo+"  true" {
		t.Fatalf("%s started by its key in %s: %v", name, repo, r.ctl.calls)
	}
}

// annIn gives workspace ann a git checkout as its folder, the one its new agents work in.
func annIn(t *testing.T, r *rig) string {
	t.Helper()
	repo := testfiles.GitCheckout(t)
	if _, err := r.e.DB.Exec("UPDATE workspaces SET folder = ? WHERE name = 'ann'", repo); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestCreateRegistersAndStartsAndTellsEachStep(t *testing.T) {
	r, sn, c := createRig(t)
	repo := annIn(t, r)
	got := result(t, c.call("agent.create", map[string]any{"workspace": "ann", "name": "new", "role": "worker", "effort": "high", "request_id": rid("c1")}))
	op, _ := got["operation"].(string)
	if got["accepted"] != true || op == "" || got["said"] != "done" || operations.Live(op) {
		t.Fatalf("answered once the agent is up, with the start's words: %v", got)
	}
	if len(sn.got) > 0 && sn.got[len(sn.got)-1]["state"] == "failed" {
		t.Fatalf("%v", sn.got[len(sn.got)-1])
	}
	startedByKey(t, r, "ann/new", repo)
	cfg, err := team.NewRoster(r.e.Home).Read("ann/new")
	if err != nil || cfg["effort"] != "high" || cfg["repo"] != repo {
		t.Fatalf("registered with what was asked: %v %v", cfg, err)
	}
	if st := sn.states(); st[0] != "running" || st[len(st)-1] != "done" {
		t.Fatalf("%v", st)
	}
	last := sn.got[len(sn.got)-1]
	if res := last["result"].(map[string]any); res["agent"] != "ann/new" || res["registered"] != true || last["agent"] != "ann/new" {
		t.Fatalf("%v", last)
	}
	again := result(t, c.call("agent.create", map[string]any{"workspace": "ann", "name": "new", "role": "worker", "effort": "high", "request_id": rid("c1")}))
	if again["replayed"] != true || again["operation"] != op || len(r.ctl.calls) != 1 {
		t.Fatalf("a retry starts nothing twice: %v %v", again, r.ctl.calls)
	}
}

// A start that fails is the call's refusal, with the operation and whether the agent was registered; nothing is cleaned up.
func TestCreateWhoseStartFailsIsRefusedAndLeavesTheAgentRegistered(t *testing.T) {
	r, sn, c := createRig(t)
	r.ctl.ok, r.ctl.said = false, "diverged"
	annIn(t, r)
	m := c.call("agent.create", map[string]any{"workspace": "ann", "name": "solo", "role": "worker", "request_id": rid("c2")})
	e, _ := m["error"].(map[string]any)
	d, _ := e["data"].(map[string]any)
	if e["message"] != "ann/solo is registered; starting it failed: diverged" || d["registered"] != true || d["operation"] == nil {
		t.Fatalf("refused with how far it got: %v", m)
	}
	if last := sn.got[len(sn.got)-1]; last["state"] != "failed" {
		t.Fatalf("the operations topic says the same: %v", last)
	}
	if _, err := team.NewRoster(r.e.Home).Read("ann/solo"); err != nil {
		t.Fatalf("nothing is cleaned up: %v", err)
	}
	again := c.call("agent.create", map[string]any{"workspace": "ann", "name": "solo", "role": "worker", "request_id": rid("c2")})
	if e2, _ := again["error"].(map[string]any); e2 == nil || e2["message"] != e["message"] || len(r.ctl.calls) != 1 {
		t.Fatalf("a retry gets the same refusal and starts nothing: %v %v", again, r.ctl.calls)
	}
}

// The call answers only once the agent is up; a start still running at the limit is refused with what it reached, and goes on.
func TestCreateAnswersOnceStartedAndAtTheLimitSaysItGoesOn(t *testing.T) {
	r, _, c := createRig(t)
	r.ctl.hold = make(chan struct{})
	r.e.Wait = time.Second // past registering, which asks git for the folder's checkout
	annIn(t, r)
	m := c.call("agent.create", map[string]any{"workspace": "ann", "name": "slow", "role": "worker", "request_id": rid("c3")})
	e, _ := m["error"].(map[string]any)
	d, _ := e["data"].(map[string]any)
	op, _ := d["operation"].(string)
	if msg, _ := e["message"].(string); op == "" || !strings.Contains(msg, "not finished in time (starting ann/slow): nothing was undone, it goes on") {
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
	annIn(t, r)
	if _, err := r.e.Roster.Register("ann/taken", team.RegisterOpts{Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	ok := map[string]any{"workspace": "ann", "name": "x", "role": "worker"}
	with := func(k string, v any, id string) map[string]any {
		p := map[string]any{"request_id": rid(id)}
		for a, b := range ok {
			p[a] = b
		}
		p[k] = v
		return c.call("agent.create", p)
	}
	if g := refused(with("name", "taken", "u1")); g != "state" {
		t.Fatalf("a used name: %q", g)
	}
	if f := badField(with("name", "ann.x", "u2")); f != "name" {
		t.Fatalf("a name with a dot: %q", f)
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
	if m := with("workspace", "nowhere", "u6"); m["error"].(map[string]any)["data"].(map[string]any)["what"] != "workspace" {
		t.Fatalf("%v", m)
	}
	if f := badField(with("workspace", nil, "u7")); f != "workspace" {
		t.Fatalf("a workspace is needed: %q", f)
	}
	if len(r.ctl.calls) != 0 {
		t.Fatalf("nothing started: %v", r.ctl.calls)
	}
}

func TestCreateRefusesASecondOfTheNameWhileTheFirstRuns(t *testing.T) {
	r, _, c := createRig(t)
	if _, err := r.e.DB.Exec("INSERT INTO operations(id, kind, state, steps_done, agent, created_at, updated_at) VALUES('held', 'agent.create', 'running', 0, 'ann/busy', 0, 0)"); err != nil {
		t.Fatal(err)
	}
	operations.Begin("held")
	defer operations.End("held")
	annIn(t, r)
	if g := refused(c.call("agent.create", map[string]any{"workspace": "ann", "name": "busy", "role": "worker", "request_id": rid("b1")})); g != "state" {
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
	annIn(t, r)
	op := result(t, c.call("agent.create", map[string]any{"workspace": "ann", "name": "fresh", "role": "worker", "request_id": rid("p1")}))["operation"].(string)
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
	got := c.call("agent.create", map[string]any{"name": "a/b", "role": "worker", "workspace": "nowhere", "effort": "extreme", "account": "a1", "request_id": rid("n1")})
	if f := badField(got); f != "name" {
		t.Fatalf("field %q", f)
	}
}

// agent.create in a workspace: a plain name, working in the folder of it asked for, or its default.
func TestCreateInAWorkspaceWorksInTheFolderAskedFor(t *testing.T) {
	r, _, c := createRig(t)
	web, api := testfiles.GitCheckout(t), testfiles.GitCheckout(t)
	ws := paths.WorkspaceKey(r.e.Home, "ann")
	r.e.DB.Exec("UPDATE workspaces SET folder = ? WHERE id = ?", web, ws[1:])
	if err := r.e.Roster.AddFolder(ws, api); err != nil {
		t.Fatal(err)
	}
	result(t, c.call("agent.create", map[string]any{"workspace": "ann", "name": "server", "folder": api, "role": "worker", "request_id": rid("f1")}))
	startedByKey(t, r, "ann/server", api)
	result(t, c.call("agent.create", map[string]any{"workspace": ws, "name": "ui", "role": "worker", "request_id": rid("f2")}))
	startedByKey(t, r, "ann/ui", web)
	if r.e.Roster.RepoOf("ann/server") != api {
		t.Fatalf("it works in that folder from now on: %q", r.e.Roster.RepoOf("ann/server"))
	}
	for i, p := range []map[string]any{
		{"workspace": "ann", "name": "a.b", "role": "worker"},
		{"workspace": "ann", "name": "x", "folder": testfiles.GitCheckout(t), "role": "worker"},
	} {
		p["request_id"] = rid("f" + string(rune('3'+i)))
		if f := badField(c.call("agent.create", p)); f != []string{"name", "folder"}[i] {
			t.Errorf("%v: field %q", p, f)
		}
	}
}

// A name or role left out or blank gets the daemon's default: the first plain name free in the workspace, and the plain role.
func TestCreateWithNoNameOrRoleTakesTheFirstFreePlainNameAndThePlainRole(t *testing.T) {
	r, _, c := createRig(t)
	annIn(t, r)
	result(t, c.call("agent.create", map[string]any{"workspace": "ann", "name": " ", "request_id": rid("d1")}))
	result(t, c.call("agent.create", map[string]any{"workspace": "ann", "role": "", "request_id": rid("d2")}))
	for _, name := range []string{"ann/" + names.Plain[0], "ann/" + names.Plain[1]} {
		if paths.AgentKey(r.e.Home, name) == "" {
			t.Fatalf("%s was not made: %v", name, r.ctl.calls)
		}
		if role, err := team.AgentInstruction(r.e.DB, name, db.PartRole); err != nil || !strings.Contains(role, DefaultRole) {
			t.Fatalf("%s's role: %q %v", name, role, err)
		}
	}
}

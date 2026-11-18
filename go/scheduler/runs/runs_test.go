package runs

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/leases"
)

type auth struct{ agent string }

func (a auth) AgentOf(int) string         { return a.agent }
func (a auth) FounderKeyOK(k string) bool { return a.agent == "" && k == "founder" }
func (a auth) SessionsReadable() error    { return nil }

// runner calls one method on a server of its own as as (an agent key, "" for the founder); a write gets a request id of its own.
func runner(t *testing.T, d *sql.DB, view func() []leases.Pool) func(as, method, params string) map[string]any {
	n, book := 0, &Book{DB: d}
	return func(as, method, params string) map[string]any {
		n++
		if method != "run.list" {
			params += fmt.Sprintf(`,"request_id":"r%d"`, n)
		}
		s := rpc.NewServer(auth{as}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = d
		Register(s, book, view)
		return one(t, s, `{"jsonrpc":"2.0","id":2,"method":"`+method+`","params":{`+strings.TrimPrefix(params, ",")+`}}`)
	}
}

// one says hello with the founder's key (an agent's caller wins over it), sends line and is its reply.
func one(t *testing.T, s *rpc.Server, line string) map[string]any {
	t.Helper()
	cl, sv := net.Pipe()
	done := make(chan struct{})
	go func() { s.Serve(rpc.NewLineTransport(sv), 0); sv.Close(); close(done) }()
	defer func() { cl.Close(); <-done }()
	cl.SetDeadline(time.Now().Add(10 * time.Second))
	rd, reply := bufio.NewReader(cl), map[string]any{}
	for _, l := range []string{`{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"founder"}}`, line} {
		if _, err := cl.Write([]byte(l + "\n")); err != nil {
			t.Fatal(err)
		}
		b, err := rd.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		reply = map[string]any{}
		_ = json.Unmarshal(b, &reply)
	}
	return reply
}

func errText(r map[string]any) string {
	if e, ok := r["error"].(map[string]any); ok {
		return fmt.Sprint(e["message"])
	}
	return ""
}

func names(t *testing.T, r map[string]any) string {
	t.Helper()
	res, ok := r["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", r)
	}
	var out []string
	for _, x := range res["runs"].([]any) {
		m := x.(map[string]any)
		out = append(out, fmt.Sprintf("%s@%s", m["name"], m["scope"]))
	}
	return strings.Join(out, " ")
}

// An agent sets a run for This Mac or its own workspace, at once and with a why; another workspace's agents neither see nor take its
// workspace runs; the founder sets and removes any; a name means one run for every agent.
func TestAgentsAndTheFounderSetAndRemoveRunsInBothScopes(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker", "web", "web.dev")
	worker, dev := teamtest.Key(t, home, "gp.worker"), teamtest.Key(t, home, "web.dev")
	gp, web := WorkspaceOf(d, worker), WorkspaceOf(d, dev)
	if gp == 0 || web == 0 || gp == web {
		t.Fatalf("workspaces %d %d", gp, web)
	}
	view := func() []leases.Pool {
		return []leases.Pool{{Kind: fmt.Sprintf("w%d:xcode", gp), Held: []leases.Lease{{Agent: worker, Since: 1}}, Queue: []leases.Waiting{{Agent: "a9", Position: 1, Task: 4}}}}
	}
	run := runner(t, d, view)
	if got := errText(run(worker, "run.set", `"name":"xcode","scope":"workspace","cmds":["xcodebuild"]`)); !strings.Contains(got, "why") {
		t.Fatalf("no why: %q", got)
	}
	if got := errText(run(worker, "run.set", `"name":"xcode","scope":"workspace","cmds":["xcodebuild"],"why":"the fans"`)); got != "" {
		t.Fatalf("workspace run: %q", got)
	}
	if got := errText(run(worker, "run.set", `"name":"shots","scope":"mac","cmds":["@browser","@simulator"],"at_once":2,"why":"one Chrome"`)); got != "" {
		t.Fatalf("mac run: %q", got)
	}
	if got := errText(run(dev, "run.set", `"name":"xcode","scope":"mac","cmds":["xcodebuild"],"why":"x"`)); !strings.Contains(got, "has a run named xcode") {
		t.Fatalf("a Mac run named as a workspace's: %q", got)
	}
	if got := errText(run(dev, "run.set", `"name":"shots","scope":"workspace","cmds":["@browser"],"why":"x"`)); !strings.Contains(got, "This Mac has a run named shots") {
		t.Fatalf("a workspace run named as a Mac one: %q", got)
	}
	if got := errText(run(dev, "run.set", fmt.Sprintf(`"name":"e2e","scope":"workspace","workspace":"w%d","cmds":["pnpm e2e"],"why":"x"`, gp))); got != "" {
		t.Fatalf("an agent's workspace param is ignored, its own is used: %q", got)
	}
	if got := errText(run("", "run.set", fmt.Sprintf(`"name":"db","scope":"workspace","workspace":"w%d","cmds":["tools/seed.sh"]`, web))); got != "" {
		t.Fatalf("the founder's, with no why: %q", got)
	}
	if got := names(t, run(worker, "run.list", fmt.Sprintf(`"agent":%q`, worker))); got != fmt.Sprintf("shots@mac xcode@w%d", gp) {
		t.Fatalf("gp's agent sees %s", got)
	}
	if got := names(t, run(worker, "run.list", fmt.Sprintf(`"agent":%q`, dev))); got != fmt.Sprintf("db@w%d e2e@w%d shots@mac", web, web) {
		t.Fatalf("web's agent sees %s", got)
	}
	all := run(worker, "run.list", "")
	if got := names(t, all); got != fmt.Sprintf("db@w%d e2e@w%d shots@mac xcode@w%d", web, web, gp) {
		t.Fatalf("every run: %s", got)
	}
	xcode := all["result"].(map[string]any)["runs"].([]any)[3].(map[string]any)
	if xcode["at_once"] != float64(1) || xcode["set_by"] != worker || xcode["why"] != "the fans" || len(xcode["held"].([]any)) != 1 ||
		xcode["waiting"].([]any)[0].(map[string]any)["task"] != float64(4) {
		t.Fatalf("xcode: %v", xcode)
	}
	if got := errText(run(dev, "run.remove", `"name":"xcode","scope":"workspace"`)); !strings.Contains(got, "has no shared run xcode") {
		t.Fatalf("another workspace's run removed: %q", got)
	}
	if got := errText(run("", "run.remove", fmt.Sprintf(`"name":"xcode","scope":"workspace","workspace":"w%d"`, gp))); got != "" {
		t.Fatalf("the founder's remove: %q", got)
	}
	if got := errText(run(worker, "run.set", `"name":"shots","scope":"mac","cmds":["@browser"],"why":"one at a time"`)); got != "" {
		t.Fatalf("a change: %q", got)
	}
	if got := names(t, run(worker, "run.list", fmt.Sprintf(`"agent":%q`, worker))); got != "shots@mac" {
		t.Fatalf("after the remove: %s", got)
	}
	if rs, _ := All(d); len(rs) != 3 || rs[2].Name != "shots" || rs[2].AtOnce != 1 || len(rs[2].Cmds) != 1 {
		t.Fatalf("the change: %+v", rs)
	}
}

func TestARunIsRefusedAWordTooBroadAReservedNameOrNoPreset(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker")
	run := runner(t, d, nil)
	for params, want := range map[string]string{
		`"name":"Xcode","scope":"mac","cmds":["x"],"why":"w"`:                   "lowercase",
		`"name":"port","scope":"mac","cmds":["x"],"why":"w"`:                    "keeps for itself",
		`"name":"b","scope":"mac","cmds":["git"],"why":"w"`:                     "nearly every command",
		`"name":"b","scope":"mac","cmds":["go"],"why":"w"`:                      "go test",
		`"name":"b","scope":"mac","cmds":["@gpu"],"why":"w"`:                    "no preset",
		`"name":"b","scope":"mac","cmds":[],"why":"w"`:                          "cmds is 1 to",
		`"name":"b","scope":"mac","cmds":["x"],"at_once":0,"why":"w"`:           "at_once",
		`"name":"b","scope":"everywhere","cmds":["x"],"why":"w"`:                "scope is mac or workspace",
		`"name":"b","scope":"workspace","cmds":["x"]`:                           "workspace is the workspace's key",
		`"name":"b","scope":"workspace","workspace":"a3","cmds":["x"],"why":""`: "workspace is the workspace's key",
	} {
		if got := errText(run("", "run.set", params)); !strings.Contains(got, want) {
			t.Errorf("%s: %q, want %q", params, got, want)
		}
	}
	if rs, _ := All(d); len(rs) != 0 {
		t.Fatalf("kept %+v", rs)
	}
}

// The lease manager reads the runs as kinds with their limits, and a name as the agent asking knows it.
func TestLeasesSeesEachRunAsAKindAndResolvesANamePerAgent(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker", "web", "web.dev")
	worker, dev := teamtest.Key(t, home, "gp.worker"), teamtest.Key(t, home, "web.dev")
	gp := WorkspaceOf(d, worker)
	for _, r := range []Run{{Name: "xcode", Workspace: gp, Cmds: []string{"xcodebuild"}, AtOnce: 1}, {Name: "shots", Cmds: []string{"@browser"}, AtOnce: 2}} {
		if err := Set(d, r); err != nil {
			t.Fatal(err)
		}
	}
	b := &Book{DB: d}
	shared, named := b.Shared, b.Named
	if got := fmt.Sprint(shared()); got != fmt.Sprintf("[{mac:shots 2} {w%d:xcode 1}]", gp) {
		t.Fatalf("shared %s", got)
	}
	if k, ok := named(worker, "xcode"); !ok || k != fmt.Sprintf("w%d:xcode", gp) {
		t.Fatalf("worker's xcode: %q %v", k, ok)
	}
	if _, ok := named(dev, "xcode"); ok {
		t.Fatal("another workspace's run resolved")
	}
	if k, ok := named(dev, "shots"); !ok || k != "mac:shots" {
		t.Fatalf("dev's shots: %q %v", k, ok)
	}
}

type failing struct {
	*sql.DB
	fail bool
}

func (f *failing) Query(q string, args ...any) (*sql.Rows, error) {
	if f.fail {
		return nil, fmt.Errorf("database is locked")
	}
	return f.DB.Query(q, args...)
}

// A read that fails keeps the runs last read, so a busy database lifts no limit; a write is read at once, and the rest after bookTTL.
func TestTheBookKeepsTheLastGoodReadAndReadsAgainAfterAWrite(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker")
	now := time.Unix(1000, 0)
	f := &failing{DB: d}
	b := &Book{DB: f, Now: func() time.Time { return now }}
	if err := Set(d, Run{Name: "x", Cmds: []string{"xcodebuild"}, AtOnce: 1}); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(b.Shared()); got != "[{mac:x 1}]" {
		t.Fatalf("first read %s", got)
	}
	if err := Set(d, Run{Name: "y", Cmds: []string{"tools/check.sh"}, AtOnce: 1}); err != nil {
		t.Fatal(err)
	}
	if got := len(b.All()); got != 1 {
		t.Fatalf("read again before bookTTL: %d", got)
	}
	b.Changed()
	if got := len(b.All()); got != 2 {
		t.Fatalf("after Changed: %d", got)
	}
	f.fail, now = true, now.Add(bookTTL)
	if got := fmt.Sprint(b.Shared()); got != "[{mac:x 1} {mac:y 1}]" {
		t.Fatalf("a failed read: %s", got)
	}
}

// A run the founder set is his: an agent's change or remove of it is refused, while agents change and remove each other's and the
// founder changes any.
func TestOnlyTheFounderChangesOrRemovesARunHeSet(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp.worker", "gp.dev")
	worker, dev := teamtest.Key(t, home, "gp.worker"), teamtest.Key(t, home, "gp.dev")
	gp := WorkspaceOf(d, worker)
	run := runner(t, d, func() []leases.Pool { return nil })
	ok := func(as, method, params, what string) {
		t.Helper()
		if got := errText(run(as, method, params)); got != "" {
			t.Fatalf("%s: %q", what, got)
		}
	}
	ok("", "run.set", `"name":"sim","scope":"mac","cmds":["@simulator"]`, "the founder's run")
	for _, c := range []struct{ method, params string }{
		{"run.set", `"name":"sim","scope":"mac","cmds":["@simulator"],"at_once":2,"why":"my build"`},
		{"run.remove", `"name":"sim","scope":"mac"`},
	} {
		if got := errText(run(worker, c.method, c.params)); got != "set by the founder; ask him on your task" {
			t.Fatalf("an agent's %s of the founder's run: %q", c.method, got)
		}
	}
	ok(worker, "run.set", `"name":"e2e","scope":"workspace","cmds":["pnpm e2e"],"why":"one browser"`, "an agent's run")
	ok(dev, "run.set", `"name":"e2e","scope":"workspace","cmds":["pnpm e2e"],"at_once":2,"why":"two fit"`, "another agent's change")
	ok("", "run.set", fmt.Sprintf(`"name":"e2e","scope":"workspace","workspace":"w%d","cmds":["pnpm e2e"],"why":"x"`, gp),
		"the founder's change of an agent's run")
	if got := errText(run(dev, "run.remove", `"name":"e2e","scope":"workspace"`)); got != "set by the founder; ask him on your task" {
		t.Fatalf("the run is the founder's once he changed it: %q", got)
	}
	ok("", "run.set", `"name":"sim","scope":"mac","cmds":["@simulator"],"at_once":2`, "the founder's change of his own")
	ok("", "run.remove", `"name":"sim","scope":"mac"`, "the founder's remove")
	if rs, _ := All(d); len(rs) != 1 || rs[0].Name != "e2e" || rs[0].AtOnce != 1 {
		t.Fatalf("left: %+v", rs)
	}
}

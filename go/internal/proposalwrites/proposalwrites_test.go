package proposalwrites

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/internal/reads"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/kit/yamlcompat"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/loadgate"
	"github.com/handline/handline/go/taskboard/supervisor"
)

type auth struct{}

func (auth) AgentOf(int) string         { return "" }
func (auth) FounderKeyOK(k string) bool { return k == "founder" }

type rig struct {
	t       *testing.T
	home    string
	db      *sql.DB
	e       *Env
	device  string // when set, calls arrive as this paired device
	srv     *rpc.Server
	live    map[string]bool // written through setLive once a call may be waiting beside the test
	liveMu  sync.Mutex
	started *starts
}

func (r *rig) setLive(agent string, on bool) {
	r.liveMu.Lock()
	r.live[r.who(agent)] = on
	r.liveMu.Unlock()
}

// starts records the agents the telling layer started or woke for work waiting.
type starts struct {
	mu    sync.Mutex
	names []string
}

func (s *starts) StartForWork(_ dbpkg.Q, agent string) string { s.add(agent); return "" }
func (s *starts) WakeAgent(_ dbpkg.Q, agent string) (bool, string) {
	s.add(agent)
	return true, ""
}
func (s *starts) AccountWait(dbpkg.Q, string) (string, error) { return "", nil }
func (s *starts) add(agent string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = append(s.names, agent)
}
func (s *starts) list() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.names)
}

func newRig(t *testing.T) *rig {
	t.Helper()
	home := t.TempDir()
	d, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	r := &rig{t: t, home: home, db: d, live: map[string]bool{}, started: &starts{}}
	say := &supervisor.Env{DB: d, Home: home}
	say.Lifecycle = r.started
	t.Cleanup(say.Wait) // before the database closes: a proposer's outcome is told off the caller
	r.e = &Env{DB: d, Home: home, Say: say, ProposalView: (&reads.Env{DB: d, Home: home}).ProposalView, Live: func() (map[string]bool, bool) {
		r.liveMu.Lock()
		defer r.liveMu.Unlock()
		return maps.Clone(r.live), true
	}}
	for _, a := range []string{"alice", "bob"} {
		if _, err := team.NewRoster(home).Register(a, team.RegisterOpts{Role: "r"}); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { store.ForgetShared(filepath.Join(home, "db.sqlite")) })
	s := rpc.NewServer(auth{}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	s.DB, s.Who = d, r.who
	Register(s, r.e)
	r.srv = s
	return r
}

// who is a word's key, as the daemon's server resolves it.
func (r *rig) who(word string) string { return paths.WhoKey(r.home, word) }

func (r *rig) call(method, params string) map[string]any {
	return r.callHello(`{"client":"t","protocol":1,"key":"founder"}`, method, params)
}

type agentAuth struct{ name string }

func (a agentAuth) AgentOf(int) string     { return a.name }
func (agentAuth) SessionsReadable() error  { return nil }
func (agentAuth) FounderKeyOK(string) bool { return false }

// callAs is call from the session of agent: its hello carries no key.
func (r *rig) callAs(agent, method, params string) map[string]any {
	r.t.Helper()
	was := r.srv
	defer func() { r.srv = was }()
	s := rpc.NewServer(agentAuth{r.who(agent)}, "x", store.LatestVersion(), "local", func(string, ...any) {})
	s.DB, s.Who = r.db, r.who
	Register(s, r.e)
	r.srv = s
	return r.callHello(`{"client":"t","protocol":1}`, method, params)
}

// phone is the paired device id: every hello on the network server logs in as it.
type phone struct{ id string }

func (p phone) Know(string) bool { return true }
func (p phone) Login(string, string, func()) (string, string, string, func(), bool) {
	return p.id, "phone", "", func() {}, true
}
func (phone) Live(string) bool { return true }

func (r *rig) callHello(hello, method, params string) map[string]any {
	r.t.Helper()
	cl, sv := net.Pipe()
	done := make(chan struct{})
	go func() {
		if r.device != "" {
			s := rpc.NewServer(auth{}, "x", store.LatestVersion(), "network", func(string, ...any) {})
			s.DB, s.Net, s.Who = r.db, phone{r.device}, r.who
			Register(s, r.e)
			s.ServeNet(rpc.NewLineTransport(sv), "127.0.0.1", func() { sv.Close() })
		} else {
			r.srv.Serve(rpc.NewLineTransport(sv), 0)
		}
		sv.Close()
		close(done)
	}()
	defer func() { cl.Close(); <-done }()
	rd := bufio.NewReader(cl)
	cl.SetDeadline(time.Now().Add(20 * time.Second))
	if r.device != "" { // the network gate lets in only a hello that carries a device token
		hello = `{"token":"t",` + strings.TrimPrefix(hello, "{")
	}
	var m map[string]any
	for _, line := range []string{fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"hello","params":%s}`, hello),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":%q,"params":%s}`, method, params)} {
		cl.Write([]byte(line + "\n"))
		b, err := rd.ReadBytes('\n')
		if err != nil {
			r.t.Fatal(err)
		}
		m = nil
		json.Unmarshal(b, &m)
	}
	return m
}

// behind sends a founder call that may wait on a held change and returns once the call has answered or ready says it is waiting, so the
// test can carry the held change on. answer is what the call answered; the connection closes when the test ends, which ends a call still
// waiting.
func (r *rig) behind(method, params string, ready func() bool) (answer func() map[string]any) {
	r.t.Helper()
	cl, sv := net.Pipe()
	done := make(chan struct{})
	go func() { r.srv.Serve(rpc.NewLineTransport(sv), 0); sv.Close(); close(done) }()
	r.t.Cleanup(func() { cl.Close(); <-done })
	rd := bufio.NewReader(cl)
	cl.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"founder"}}` + "\n"))
	if _, err := rd.ReadBytes('\n'); err != nil {
		r.t.Fatal(err)
	}
	cl.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":%q,"params":%s}`, method, params) + "\n"))
	got := make(chan map[string]any, 1)
	go func() {
		var m map[string]any
		if b, err := rd.ReadBytes('\n'); err == nil {
			json.Unmarshal(b, &m)
		}
		got <- m
	}()
	var m map[string]any
	answered := false
	for deadline := time.Now().Add(10 * time.Second); !answered && !ready(); {
		select {
		case m = <-got:
			answered = true
		case <-time.After(5 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			r.t.Fatal("neither answered nor held")
		}
	}
	return func() map[string]any {
		r.t.Helper()
		if answered {
			return m
		}
		select {
		case m = <-got:
			answered = true
			return m
		case <-time.After(10 * time.Second):
			r.t.Fatal("the call never answered")
			return nil
		}
	}
}

func (r *rig) anyHeld() bool {
	for _, p := range r.e.listing() {
		if p.str("state") == applying {
			return true
		}
	}
	return false
}

// applyNowHeld is proposal.apply_now sent behind: it waits once the proposal carries the apply_now mark.
func (r *rig) applyNowHeld(id int64, pid, rid string) func() map[string]any {
	return r.behind("proposal.apply_now", fmt.Sprintf(`{"proposal":%d,"request_id":%q}`, id, rid), func() bool {
		p, _ := r.e.load(pid)
		return p != nil && p.str("apply_now") != ""
	})
}

// decideHeld is decide sent behind (see behind).
func (r *rig) decideHeld(id int64, subject, rid string) func() map[string]any {
	return r.behind("proposal.decide", fmt.Sprintf(`{"proposal":%d,"decision":"approve","request_id":%q,"subject":%q}`, id, rid, subject), r.anyHeld)
}

// propose writes a proposal file as hr.py would and returns it with its wire id.
func (r *rig) propose(id string, fields ...string) (*Prop, int64) {
	r.t.Helper()
	p := &Prop{}
	p.set("id", id)
	for i := 0; i < len(fields); i += 2 {
		v := fields[i+1]
		// a proposal names its agents by key: its agent, a bundle item's, and a link's other end
		if k := fields[i]; k == "agent" || strings.HasSuffix(k, ".agent") || strings.HasSuffix(k, ".to") && r.who(v) != v {
			v = r.who(v)
		}
		p.set(fields[i], v)
	}
	p.set("requested_by", r.who("alice"))
	p.set("why", "because")
	p.set("created", int64(1000))
	p.set("state", "proposed")
	if err := r.e.save(p); err != nil {
		r.t.Fatal(err)
	}
	n, _ := hexNum(id)
	return p, n
}

func (r *rig) decide(id int64, decision string, subject string, rid string) map[string]any {
	params := fmt.Sprintf(`{"proposal":%d,"decision":%q,"request_id":%q`, id, decision, rid)
	if subject != "" {
		params += fmt.Sprintf(`,"subject":%q`, subject)
	}
	return r.call("proposal.decide", params+"}")
}

func (r *rig) file(parts ...string) string {
	b, _ := os.ReadFile(filepath.Join(append([]string{r.home}, parts...)...))
	return string(b)
}

// agentFile is a file in the agent's folder, "" when it is not there.
func (r *rig) agentFile(agent, name string) string {
	b, _ := os.ReadFile(filepath.Join(paths.AgentDir(r.home, agent), name))
	return string(b)
}

// strayFolders are the agent folders no agent row has, as a rolled-back add would leave.
func (r *rig) strayFolders() []string {
	entries, _ := os.ReadDir(paths.AgentsDir(r.home))
	var out []string
	for _, e := range entries {
		var n int
		if r.db.QueryRow("SELECT COUNT(*) FROM agents WHERE 'a' || id = ?", e.Name()).Scan(&n); n == 0 && e.Name() != "archive" {
			out = append(out, e.Name())
		}
	}
	return out
}

// rosterText is the agents, one "  <name>: {}" line each, so a check reads who is registered.
func (r *rig) rosterText() string {
	rows, err := r.db.Query("SELECT " + paths.LegacyNameSQL + " FROM agents a JOIN workspaces w ON w.id = a.workspace ORDER BY a.id")
	if err != nil {
		r.t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var n string
		rows.Scan(&n)
		b.WriteString("  " + n + ": {}\n")
	}
	return b.String()
}

// settingsText is the agent's settings, one "<key>: <value>" line each in key order; "" for an agent with no row.
func (r *rig) settingsText(agent string) string {
	cfg, err := team.NewRoster(r.home).Read(agent)
	if err != nil {
		return ""
	}
	var lines []string
	for k, v := range cfg {
		lines = append(lines, fmt.Sprintf("%s: %v", k, v))
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n") + "\n"
}

func (r *rig) state(id string) string {
	p, _ := r.e.load(id)
	return p.str("state")
}

func gate(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	d, _ := e["data"].(map[string]any)
	g, _ := d["gate"].(string)
	return g
}

// proposal.list reads back the founder's note on a decision, and no note when none was given.
func TestTheFoundersNoteOnADecisionIsListed(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	_, noted := r.propose("aaaa0001", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	_, bare := r.propose("aaaa0002", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	r.call("proposal.decide", fmt.Sprintf(`{"proposal":%d,"decision":"reject","note":"not before the redesign","request_id":"d1"}`, noted))
	r.decide(bare, "reject", "", "d2")
	if v := r.e.ProposalView("aaaa0001"); v["note"] != "not before the redesign" {
		t.Fatalf("%v", v)
	}
	if v := r.e.ProposalView("aaaa0002"); v["state"] != "rejected" || v["note"] != nil {
		t.Fatalf("%v", v)
	}
}

func TestRejectIsRecordedOnceAndAFixedProposalIsRefusedAgain(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	_, id := r.propose("aaaa0001", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	res := r.decide(id, "reject", "", "d1")["result"].(map[string]any)
	if res["said"] != "proposal aaaa0001 rejected" || r.state("aaaa0001") != "rejected" {
		t.Fatalf("%v %s", res, r.state("aaaa0001"))
	}
	if got := r.e.decisionOf("aaaa0001"); got["call"] != "rejected" {
		t.Fatalf("the decision is on record: %v", got)
	}
	if again := r.decide(id, "reject", "", "d1")["result"].(map[string]any); again["replayed"] != true {
		t.Fatalf("a retry repeats nothing: %v", again)
	}
	settled := r.decide(id, "reject", "", "d2")
	if gate(settled) != "proposal_settled" {
		t.Fatalf("a settled proposal is refused: %v", settled)
	}
	if e := settled["error"].(map[string]any)["data"].(map[string]any); e["proposal"] == nil {
		t.Fatal("the refusal carries the proposal as it is")
	}
}

func TestApproveNeedsTheSubjectTheFounderReadAndAppliesIt(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa0002", "kind", "add", "agent", "carol")
	if m := r.call("proposal.decide", fmt.Sprintf(`{"proposal":%d,"decision":"approve","request_id":"a0"}`, id)); m["error"] == nil {
		t.Fatal("an approve names its subject")
	}
	if gate(r.decide(id, "approve", "not-the-digest", "a1")) != "proposal_changed" {
		t.Fatal("an edit after the card was drawn is refused")
	}
	if r.state("aaaa0002") != "proposed" || strings.Contains(r.rosterText(), "carol") {
		t.Fatal("nothing changed")
	}
	res := r.decide(id, "approve", p.subject(), "a2")["result"].(map[string]any)
	if !strings.HasPrefix(res["said"].(string), "carol is an agent.") {
		t.Fatalf("%v", res)
	}
	if !strings.Contains(r.rosterText(), "  carol: {}") || r.state("aaaa0002") != "applied" {
		t.Fatalf("carol is registered and the proposal applied:\n%s", r.rosterText())
	}
	if gate(r.decide(id, "approve", p.subject(), "a3")) != "proposal_settled" {
		t.Fatal("applied is settled")
	}
}

func TestAddRegistersTheAgentInItsWorkspace(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	repo := t.TempDir()
	os.Mkdir(filepath.Join(repo, ".git"), 0o755)
	p, id := r.propose("aaaa0003", "kind", "add", "agent", "eng.carol", "team", "eng", "repo", repo)
	res := r.decide(id, "approve", p.subject(), "b1")["result"].(map[string]any)
	said := res["said"].(string)
	if !strings.HasPrefix(said, "eng.carol is an agent, team eng.") || !strings.HasSuffix(said, "is a git repository.") {
		t.Fatalf("%q", said)
	}
	if !strings.Contains(r.rosterText(), "  eng.carol: {}") || team.NewRoster(r.home).RepoOf("eng.carol") != repo {
		t.Fatalf("%s", r.rosterText())
	}
	if _, err := os.Stat(filepath.Join(paths.AgentDir(r.home, "eng.carol"), "ROLE.md")); err != nil {
		t.Fatal("registered")
	}
	// the team named is the workspace its name says
	p, id = r.propose("aaaa0013", "kind", "add", "agent", "dave", "team", "eng")
	if m := r.decide(id, "approve", p.subject(), "b2"); !strings.Contains(fmt.Sprint(m), "would be in workspace dave, not eng") {
		t.Fatalf("%v", m)
	}
}

// approveBundle raises changes as one bundle from alice and approves it; the error reply, or nil.
func (r *rig) approveBundle(changes string) any {
	r.t.Helper()
	m := r.callAs("alice", "proposal.propose", `{"kind":"bundle","args":{"name":"changes","changes":[`+changes+`]},"why":"w","request_id":"p1"}`)
	n, ok := m["result"].(map[string]any)["proposal"].(float64)
	if !ok {
		r.t.Fatalf("proposed: %v", m)
	}
	p, err := r.e.load(fmt.Sprintf("%08x", int64(n)))
	if err != nil {
		r.t.Fatal(err)
	}
	return r.decide(int64(n), "approve", p.subject(), "d1")["error"]
}

func TestADiffThatFailsHalfwayPutsEverythingBack(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	before := r.rosterText()
	p, id := r.propose("aaaa0004", "kind", "bundle", "agent", "2 changes",
		"i1.kind", "add", "i1.agent", "dave",
		"i2.kind", "rename", "i2.agent", "dave", "i2.to", "dave.x")
	res := r.decide(id, "approve", p.subject(), "c1")
	msg := res["error"].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "step 2 of 2 (rename dave) failed") || !strings.Contains(msg, "Nothing was changed") {
		t.Fatalf("%v", res)
	}
	if r.rosterText() != before {
		t.Fatalf("the roster put back:\n%s", r.rosterText())
	}
	if stray := r.strayFolders(); len(stray) != 0 {
		t.Fatalf("the folder the first step made is gone: %v", stray)
	}
	if r.state("aaaa0004") != "approved" {
		t.Fatalf("decided but not applied, so approving again retries: %s", r.state("aaaa0004"))
	}
	got, _ := r.e.load("aaaa0004")
	if got.str("error") == "" {
		t.Fatal("the error is kept on the proposal")
	}
}

func TestRetireArchivesTheFolderAndRenameMovesTheNameAndItsRows(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.db.Exec("INSERT INTO agent_state(agent, state, updated) VALUES(?, 'idle', 1)", r.who("bob"))
	if _, err := dbpkg.AddRequest(r.db, dbpkg.NewRequest{Requester: r.who("alice"), Target: r.who("bob"), Body: "hi", Kind: "ask"}); err != nil {
		t.Fatal(err)
	}
	p, id := r.propose("aaaa0005", "kind", "rename", "agent", "bob", "to", "robert")
	r.setLive("bob", true)
	answer := r.decideHeld(id, p.subject(), "e1")
	if r.state("aaaa0005") != "applying" || !strings.Contains(r.rosterText(), "  bob: {}") {
		t.Fatalf("nothing changes while bob's turn runs: %s", r.state("aaaa0005"))
	}
	if m := r.decide(id, "approve", p.subject(), "e2"); gate(m) != "proposal_settled" {
		t.Fatalf("an applying proposal is not decided again: %v", m)
	}
	// a sandboxed agent's caches, with a read-only folder as Go's module cache keeps them
	cache := func(name string) string {
		ro := filepath.Join(paths.CacheDir(r.home, name), "go-mod", "m@v1")
		os.MkdirAll(ro, 0o755)
		os.WriteFile(filepath.Join(ro, "go.mod"), nil, 0o444)
		os.Chmod(ro, 0o555)
		return paths.CacheDir(r.home, name)
	}
	bobs, bobDir := cache("bob"), paths.AgentDir(r.home, "bob")
	idle, closed, started := false, []string{}, []string{}
	ports := HeldPorts{Alive: func(a string) bool { return r.live[a] }, Idle: func(string) bool { return idle },
		Close: func(a string) (bool, string) { closed = append(closed, a); r.setLive(a, false); return true, "" },
		Start: func(a string) (bool, string) { started = append(started, a); return true, "" }}
	r.e.SweepHeld(ports) // mid-turn: not closed
	if len(closed) != 0 || r.state("aaaa0005") != "applying" {
		t.Fatalf("a turn is never cut: %v %s", closed, r.state("aaaa0005"))
	}
	idle = true
	r.e.SweepHeld(ports) // at its prompt: closed, applied on the next look
	if !reflect.DeepEqual(closed, []string{r.who("bob")}) || r.state("aaaa0005") != "applying" {
		t.Fatalf("closed at its prompt: %v %s", closed, r.state("aaaa0005"))
	}
	r.e.SweepHeld(ports)
	if r.state("aaaa0005") != "applied" || !reflect.DeepEqual(started, []string{r.who("robert")}) {
		t.Fatalf("applied, and the agent started under its new name: %s %v", r.state("aaaa0005"), started)
	}
	if m := answer(); m["result"] == nil || !strings.Contains(m["result"].(map[string]any)["said"].(string), "robert") {
		t.Fatalf("the approve answers once the rename is applied: %v", m)
	}
	var target, agent string
	r.db.QueryRow("SELECT target FROM requests WHERE body = 'hi'").Scan(&target)
	r.db.QueryRow("SELECT agent FROM agent_state").Scan(&agent)
	yml := r.rosterText()
	if target != r.who("robert") || agent != r.who("robert") || !strings.Contains(yml, "  robert: {}") || strings.Contains(yml, "bob") {
		t.Fatalf("%s %s\n%s", target, agent, yml)
	}
	pairs, _, _ := yamlcompat.ParsePairs(r.settingsText("robert"))
	if len(pairs) == 0 {
		t.Fatal("the settings moved with the name")
	}
	if paths.AgentDir(r.home, "robert") != bobDir || paths.CacheDir(r.home, "robert") != bobs {
		t.Fatal("a rename keeps the folder and the caches, which are by key")
	}
	if _, err := os.Stat(bobs); err != nil {
		t.Fatal("a rename leaves the caches where they are")
	}
	roberts := bobs
	p2, id2 := r.propose("aaaa0006", "kind", "retire", "agent", "robert")
	res := r.decide(id2, "approve", p2.subject(), "e3")["result"].(map[string]any)
	if !strings.Contains(res["said"].(string), "robert is retired; its folder is at") {
		t.Fatalf("%v", res)
	}
	if _, err := os.Stat(bobDir); err == nil {
		t.Fatal("archived")
	}
	if _, err := os.Stat(roberts); err == nil {
		t.Fatal("a retired agent's caches are left behind")
	}
	matches, _ := filepath.Glob(filepath.Join(r.home, "agents", "archive", "*-robert"))
	if len(matches) != 1 || strings.Contains(r.rosterText(), "robert") {
		t.Fatalf("%v", matches)
	}
}

func TestContextScheduleGlobalAndSettingsAreAppliedAsProposed(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	ctx, _ := json.Marshal("You are alice.\n")
	p, id := r.propose("aaaa0007", "kind", "context", "agent", "alice", "set.ROLE.md", string(ctx))
	r.decide(id, "approve", p.subject(), "f1")
	if r.agentFile("alice", "ROLE.md") != "You are alice.\n" {
		t.Fatalf("%q", r.agentFile("alice", "ROLE.md"))
	}
	p, id = r.propose("aaaa0008", "kind", "schedule", "agent", "alice", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "morning")
	if res := r.decide(id, "approve", p.subject(), "f2"); !strings.Contains(r.settingsText("alice"), "morning") {
		t.Fatalf("scheduled: %v\n%s", res, r.settingsText("alice"))
	}
	p, id = r.propose("aaaa0009", "kind", "global", "agent", "idle-timeout", "set.value", "1800")
	r.decide(id, "approve", p.subject(), "f3")
	if !strings.Contains(r.file("config.yaml"), "park_timeout_s: 1800") {
		t.Fatalf("%q", r.file("config.yaml"))
	}
	p, id = r.propose("aaaa000a", "kind", "settings", "agent", "alice", "set.effort", "high")
	res := r.decide(id, "approve", p.subject(), "f4")
	if res["result"] == nil || !strings.Contains(r.settingsText("alice"), "effort: high") {
		t.Fatalf("%v\n%s", res, r.settingsText("alice"))
	}
}

func (auth) SessionsReadable() error { return nil }

func TestAContextFileHasNoSizeLimit(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	role := "# alice\n\n" + strings.Repeat("x", 3072) + "\n"
	ctx, _ := json.Marshal(role)
	p, id := r.propose("aaaa0010", "kind", "context", "agent", "alice", "set.ROLE.md", string(ctx))
	if res := r.decide(id, "approve", p.subject(), "g1"); r.agentFile("alice", "ROLE.md") != role {
		t.Fatalf("a 3KB ROLE.md was not written: %v", res)
	}
}

func TestAgentSetTakesEveryFieldOfTheContractForTheFounder(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	for _, c := range []struct{ key, val, want string }{
		{"model", "opus", "model: opus"},
		{"effort", "high", "effort: high"},
		{"weight", "heavy", "weight: heavy"},
		{"browser", "chrome", "chrome: true"},
		{"context_k", "120", "120"},
		{"idle_close", "30", "park_timeout_s: 1800"},
		{"handoff_note", "off", "handoff_note"},
		{"sandbox", "on", "sandbox"},
		{"mode", "plan", "plan"},
	} {
		res := r.call("agent.set", fmt.Sprintf(`{"agent":"alice","values":{%q:%q},"request_id":"as-%s"}`, c.key, c.val, c.key))
		if got, _ := res["result"].(map[string]any); got["accepted"] != true {
			t.Fatalf("%s=%s: %v", c.key, c.val, res)
		}
		if all := r.settingsText("alice") + r.rosterText(); !strings.Contains(all, c.want) {
			t.Errorf("%s=%s not written (wanted %q):\n%s", c.key, c.val, c.want, all)
		}
	}
	res := r.call("agent.set", `{"agent":"alice","values":{"browser":"playwright","browser_sites":"https://a.example https://b.example"},"request_id":"as-sites"}`)
	if res["result"] == nil || !strings.Contains(r.settingsText("alice")+r.agentFile("alice", "mcp.json"), "b.example") {
		t.Fatalf("sites: %v", res)
	}
	res = r.call("agent.set", `{"agent":"bob","values":{"effort":"low","context_k":"90"},"request_id":"as-two"}`)
	if res["result"] == nil || !strings.Contains(r.settingsText("bob"), "effort: low") {
		t.Fatalf("%v\n%s", res, r.settingsText("bob"))
	}
}

func TestAgentSetRefusesWhatCannotBeSaved(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	for name, params := range map[string]string{
		"a bad value":    `{"agent":"alice","values":{"effort":"extreme"},"request_id":"b1"}`,
		"an unknown key": `{"agent":"alice","values":{"nope":"x"},"request_id":"b2"}`,
		"skip prompts":   `{"agent":"alice","values":{"skip_permissions":"on"},"request_id":"b4"}`,
		"no agent":       `{"agent":"ghost","values":{"effort":"high"},"request_id":"b3"}`,
	} {
		if res := r.call("agent.set", params); res["error"] == nil {
			t.Errorf("%s was saved: %v", name, res)
		}
	}
	for name, params := range map[string]string{
		"no values":     `{"agent":"alice","values":{},"request_id":"b4"}`,
		"a number":      `{"agent":"alice","values":{"context_k":150},"request_id":"b5"}`,
		"a missing one": `{"values":{"effort":"high"},"request_id":"b6"}`,
	} {
		res := r.call("agent.set", params)
		if e, _ := res["error"].(map[string]any); e == nil || e["code"] == nil {
			t.Errorf("%s: %v", name, res)
		}
	}
}

func TestAgentSetLeavesTheCustomBrowserToATerminal(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if res := r.call("agent.set", `{"agent":"alice","values":{"browser":"custom"},"request_id":"c1"}`); res["error"] == nil {
		t.Fatalf("custom was saved: %v", res)
	}
}

func TestAgentSetIsRefusedToAPairedDevice(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	c := &rpc.Conn{Caller: rpc.Caller{Role: "member", Kind: "device", Info: map[string]any{"kind": "device", "name": "phone", "device": "d1"}}}
	before := r.settingsText("alice")
	_, err := r.e.agentSetMethod(c, map[string]any{"agent": "alice", "values": map[string]any{"sandbox": "off"}})
	var re *rpc.Error
	if !errors.As(err, &re) || re.Data["gate"] != "role" {
		t.Fatalf("a device was not refused at the role gate: %v", err)
	}
	if r.settingsText("alice") != before {
		t.Fatal("a refused device changed the agent")
	}
}

func errOf(t *testing.T, res map[string]any) map[string]any {
	t.Helper()
	e, _ := res["error"].(map[string]any)
	if e == nil {
		t.Fatalf("not refused: %v", res)
	}
	d, _ := e["data"].(map[string]any)
	return d
}

func TestAnAgentSetsTheOrdinaryKeysOfItselfAndNothingElse(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	set := func(agent string, values string, rid string) map[string]any {
		return r.callAs("alice", "agent.set", fmt.Sprintf(`{"agent":%q,"values":%s,"request_id":%q}`, agent, values, rid))
	}
	if res := set("alice", `{"effort":"high","context_k":"120"}`, "ag1"); res["result"] == nil {
		t.Fatalf("an agent's own ordinary keys: %v", res)
	}
	if !strings.Contains(r.settingsText("alice"), "effort: high") {
		t.Fatalf("not written:\n%s", r.settingsText("alice"))
	}
	bob := r.settingsText("bob")
	if d := errOf(t, set("bob", `{"effort":"low"}`, "ag2")); d["code"] != "refused" || d["gate"] != "not_self" {
		t.Fatalf("another agent's settings: %v", d)
	}
	if r.settingsText("bob") != bob {
		t.Fatal("a refused call changed another agent")
	}
	alice := r.settingsText("alice")
	for i, key := range []string{"sandbox", "mode"} { // the founder's alone
		if d := errOf(t, set("alice", fmt.Sprintf(`{%q:"on"}`, key), fmt.Sprintf("ag-f%d", i))); d["code"] != "refused" {
			t.Errorf("%s: an agent set a founder-only key: %v", key, d)
		}
	}
	if r.settingsText("alice") != alice {
		t.Fatalf("a founder-only key was written:\n%s", r.settingsText("alice"))
	}
}

func TestAgentSetNamesWhatWentWrong(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	call := func(params string) map[string]any {
		return errOf(t, r.call("agent.set", params))
	}
	if d := call(`{"agent":"ghost","values":{"effort":"high"},"request_id":"n1"}`); d["code"] != "not_found" {
		t.Errorf("an agent that is not there: %v", d)
	}
	if d := call(`{"agent":"alice","values":{"effort":"extreme"},"request_id":"n2"}`); d["code"] != "refused" {
		t.Errorf("a bad value: %v", d)
	}
	if d := call(`{"agent":"alice","values":{"nope":"x"},"request_id":"n3"}`); d["code"] != "refused" {
		t.Errorf("an unknown key: %v", d)
	}
}

func TestARetriedAgentSetIsAnsweredAndWritesNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	call := `{"agent":"alice","values":{"effort":"high"},"request_id":"re1"}`
	first, _ := r.call("agent.set", call)["result"].(map[string]any)
	if first["accepted"] != true || first["replayed"] != nil {
		t.Fatalf("first: %v", first)
	}
	// the setting changes behind the retry: a rewrite would put "high" back
	teamtest.Set(t, r.home, "alice", "effort", "low")
	again, _ := r.call("agent.set", call)["result"].(map[string]any)
	if again["replayed"] != true || again["accepted"] != true {
		t.Fatalf("retry: %v", again)
	}
	if strings.Contains(r.settingsText("alice"), "effort: high") {
		t.Fatal("the retry wrote the setting again")
	}
}

func TestWithdrawTakesBackAnUndecidedProposalForItsAuthorOrTheFounderAndNoOtherAgent(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	_, a := r.propose("aaaa0010", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	_, b := r.propose("aaaa0011", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	_, c := r.propose("aaaa0012", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	call := func(who string, id int64, rid string) map[string]any {
		params := fmt.Sprintf(`{"proposal":%d,"request_id":%q}`, id, rid)
		if who == "" {
			return r.call("proposal.withdraw", params)
		}
		return r.callAs(who, "proposal.withdraw", params)
	}
	if m := call("bob", a, "w1"); gate(m) != "role" || r.state("aaaa0010") != "proposed" {
		t.Fatalf("another agent withdrew it: %v %s", m, r.state("aaaa0010"))
	}
	res := call("alice", a, "w2")["result"].(map[string]any)
	if res["said"] != "proposal aaaa0010 withdrawn" || r.state("aaaa0010") != "withdrawn" {
		t.Fatalf("%v %s", res, r.state("aaaa0010"))
	}
	if p, _ := r.e.load("aaaa0010"); p.get("decided") == nil {
		t.Fatal("the withdrawal is not dated")
	}
	if again := call("alice", a, "w2")["result"].(map[string]any); again["replayed"] != true {
		t.Fatalf("a retry repeats nothing: %v", again)
	}
	// the founder takes back any, even one approved and not applied
	if m := call("", b, "w3"); m["result"] == nil || r.state("aaaa0011") != "withdrawn" {
		t.Fatalf("%v", m)
	}
	p, _ := r.e.load("aaaa0012")
	p.set("state", "approved")
	r.e.save(p)
	if m := call("", c, "w4"); m["result"] == nil || r.state("aaaa0012") != "withdrawn" {
		t.Fatalf("%v", m)
	}
}

func TestWithdrawOfASettledOrUnknownProposalIsRefusedWithTheProposalAsItIs(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	_, id := r.propose("aaaa0020", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	r.decide(id, "reject", "", "d1")
	m := r.callAs("alice", "proposal.withdraw", fmt.Sprintf(`{"proposal":%d,"request_id":"w1"}`, id))
	e, _ := m["error"].(map[string]any)
	d, _ := e["data"].(map[string]any)
	if d["gate"] != "proposal_settled" || d["proposal"] == nil || r.state("aaaa0020") != "rejected" {
		t.Fatalf("%v", m)
	}
	if m := r.callAs("alice", "proposal.withdraw", `{"proposal":999999,"request_id":"w2"}`); m["error"].(map[string]any)["data"].(map[string]any)["code"] != "not_found" {
		t.Fatalf("%v", m)
	}
	if m := r.callAs("alice", "proposal.withdraw", `{"proposal":"x","request_id":"w3"}`); m["error"].(map[string]any)["data"].(map[string]any)["code"] != "bad_request" {
		t.Fatalf("%v", m)
	}
}

func TestAPairedDeviceDecidesAndAnAgentSessionIsRefused(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	_, id := r.propose("aaaa0003", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	refused := r.callAs("alice", "proposal.decide", fmt.Sprintf(`{"proposal":%d,"decision":"reject","request_id":"g1"}`, id))
	if gate(refused) != "role" || r.state("aaaa0003") != "proposed" {
		t.Fatalf("an agent does not decide: %v %s", refused, r.state("aaaa0003"))
	}
	r.device = "dev-1"
	res := r.decide(id, "reject", "", "g2")["result"].(map[string]any)
	if res["said"] != "proposal aaaa0003 rejected" || r.state("aaaa0003") != "rejected" {
		t.Fatalf("a paired device decides: %v %s", res, r.state("aaaa0003"))
	}
}

func (r *rig) told(who string) []string {
	r.t.Helper()
	rows, err := r.db.Query("SELECT body FROM requests WHERE target = ? AND requester = ? ORDER BY id", r.who(who), dbpkg.NoticeRequester)
	if err != nil {
		r.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var b string
		rows.Scan(&b)
		out = append(out, b)
	}
	return out
}

func TestTheProposerIsToldHowEachDecisionWentInTheWordsTheOldCLIUsed(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	_, id := r.propose("aaaa0011", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	r.call("proposal.decide", fmt.Sprintf(`{"proposal":%d,"decision":"reject","note":"not now","request_id":"t1"}`, id))
	_, id = r.propose("aaaa0012", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	r.decide(id, "reject", "", "t2")
	p, id := r.propose("aaaa0013", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	r.decide(id, "approve", p.subject(), "t3")
	p, id = r.propose("aaaa0014", "kind", "bundle", "agent", "2 changes",
		"i1.kind", "add", "i1.agent", "dave",
		"i2.kind", "rename", "i2.agent", "dave", "i2.to", "dave.x")
	r.decide(id, "approve", p.subject(), "t4")
	got := r.told("alice")
	if len(got) != 4 || got[0] != "proposal aaaa0011 was rejected: not now" || got[1] != "proposal aaaa0012 was rejected." ||
		!strings.HasPrefix(got[2], "proposal aaaa0013 applied: ") ||
		!strings.HasPrefix(got[3], "proposal aaaa0014 was approved but could not be applied: step 2 of 2 (rename dave) failed") {
		t.Fatalf("%q", got)
	}
}

func TestABatchOfDecisionsWaitsForTheProposersDigestAndNothingIsFromTheFounder(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	for i := 0; i < 5; i++ {
		p, id := r.propose(fmt.Sprintf("aaaa002%d", i), "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
		r.decide(id, ifThen(i%2 == 0, "approve", "reject"), p.subject(), fmt.Sprintf("b%d", i))
	}
	if pending, err := dbpkg.PendingNotices(r.db, r.who("bob")); err != nil || len(pending) != 5 {
		t.Fatalf("bob: five outcomes wait for one digest: %d %v", len(pending), err)
	}
	var fromFounder int
	r.db.QueryRow("SELECT COUNT(*) FROM requests WHERE target = ? AND requester = 'p1'", r.who("alice")).Scan(&fromFounder)
	if fromFounder != 0 {
		t.Fatalf("%d outcomes were said as the founder", fromFounder)
	}
}

func TestAnOutcomeReachesTheAgentsABundleNamesOnceEach(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	if _, err := team.NewRoster(r.home).Register("carol", team.RegisterOpts{Role: "r"}); err != nil {
		t.Fatal(err)
	}
	_, id := r.propose("aaaa0031", "kind", "bundle", "agent", "2 changes",
		"i1.kind", "link", "i1.agent", "carol", "i1.to", "bob", "i2.kind", "link", "i2.agent", "alice", "i2.to", "alice")
	r.decide(id, "reject", "", "n1")
	for who, want := range map[string]int{"alice": 1, "bob": 1, "carol": 1} {
		if got := len(r.told(who)); got != want {
			t.Errorf("%s was told %d times, want %d", who, got, want)
		}
	}
}

func TestAnOutcomeWakesTheIdleProposerAndOnlyNoticesTheOthersNamed(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa0041", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	r.decide(id, "approve", p.subject(), "w1")
	r.e.Say.Wait()
	if got := r.started.list(); !slices.Equal(got, []string{r.who("alice")}) {
		t.Fatalf("the closed proposer is started at once, nobody else: %q", got)
	}
	owed, err := dbpkg.UndeliveredFor(r.db, r.who("alice"))
	if err != nil || len(owed) != 1 || owed[0].Notice.Int64 != 0 || !strings.HasPrefix(owed[0].Body, "proposal aaaa0041 applied") {
		t.Fatalf("the proposer is owed the outcome as a message the tick tells, not a digest line: %v %v", owed, err)
	}
	if pending, err := dbpkg.PendingNotices(r.db, r.who("bob")); err != nil || len(pending) != 1 {
		t.Fatalf("the agent the change names gets a notice for its digest: %d %v", len(pending), err)
	}
}

func ifThen(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

func TestEachOutcomeMessageNamesTheProposalItIsAbout(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	_, rejected := r.propose("aaaa0015", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	r.decide(rejected, "reject", "", "o1")
	p, applied := r.propose("aaaa0016", "kind", "schedule", "agent", "bob", "set.action", "add", "set.cron", "0 9 * * *", "set.message", "m")
	r.decide(applied, "approve", p.subject(), "o2")
	rows, err := r.db.Query("SELECT json_extract(e.payload, '$.proposal') FROM requests q LEFT JOIN events e ON e.dedup_key = 'proposal_outcome:' || q.id "+
		"WHERE q.target = ? AND q.requester = ? ORDER BY q.id", r.who("alice"), dbpkg.NoticeRequester)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int64
	for rows.Next() {
		var n sql.NullInt64
		rows.Scan(&n)
		got = append(got, n.Int64)
	}
	if fmt.Sprint(got) != fmt.Sprint([]int64{rejected, applied}) {
		t.Fatalf("each told message links its proposal %d and %d: %v", rejected, applied, got)
	}
}

func TestACacheThatCannotBeRemovedDoesNotUndoARetire(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	caches := filepath.Dir(paths.CacheDir(r.home, "bob"))
	os.MkdirAll(filepath.Join(paths.CacheDir(r.home, "bob"), "go-build"), 0o755)
	os.Chmod(caches, 0o555) // bob's folder cannot leave it
	defer os.Chmod(caches, 0o755)
	p, id := r.propose("aaaa0009", "kind", "retire", "agent", "bob")
	res := r.decide(id, "approve", p.subject(), "g1")["result"].(map[string]any)
	if !strings.Contains(res["said"].(string), "bob is retired") || strings.Contains(r.rosterText(), "bob:") {
		t.Fatalf("the retire stopped half done: %v", res)
	}
}

func TestProposeWritesTheProposalFileAndRefusesWhatTheOldCLIRefused(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	call := func(rid, params string) map[string]any {
		return r.callAs("alice", "proposal.propose", fmt.Sprintf(`{%s,"request_id":%q}`, params, rid))
	}
	res := call("p1", `"kind":"retire","args":{"name":"bob"},"why":"idle"`)["result"].(map[string]any)
	n, ok := res["proposal"].(float64)
	if !ok || !strings.HasPrefix(res["said"].(string), "proposal ") {
		t.Fatalf("%v", res)
	}
	p, err := r.e.load(fmt.Sprintf("%08x", int64(n)))
	if err != nil || p == nil || p.str("kind") != "retire" || p.str("agent") != r.who("bob") || p.str("requested_by") != r.who("alice") ||
		p.str("state") != "proposed" || p.str("why") != "idle" {
		t.Fatalf("%v %+v", err, p)
	}
	ctx := call("p2", `"kind":"context","args":{"name":"bob","sets":{"ROLE.md":"new role"}},"why":"w"`)["result"].(map[string]any)
	cp, _ := r.e.load(fmt.Sprintf("%08x", int64(ctx["proposal"].(float64))))
	if cp == nil || cp.str("set.ROLE.md") != `"new role"` {
		t.Fatalf("a context text is kept as JSON on one line: %+v", cp)
	}
	for _, c := range []struct{ params, want string }{
		{`"kind":"add","args":{"name":"alice"},"why":"w"`, "alice is already an agent"},
		{`"kind":"retire","args":{"name":"ghost"},"why":"w"`, "ghost is not an agent"},
		{`"kind":"add","args":{"name":"carol"},"why":"w"`, "carol needs a repo"},
		{`"kind":"retire","args":{"name":"bob"},"why":" "`, "say why"},
		{`"kind":"link","args":{"name":"alice","to":"bob"},"why":"w"`, `kind "link" is not raised this way`},
		{`"kind":"context","args":{"name":"bob","sets":{"x.md":"t"}},"why":"w"`, "only ROLE.md and base.md"},
	} {
		refusedWith(t, call(c.want[:3], c.params), c.want)
	}
}

// gitDir is a fresh folder that reads as a git checkout.
// refusedWith fails t unless the call m was refused with a message that holds want.
func refusedWith(t *testing.T, m map[string]any, want string) {
	t.Helper()
	if msg, _ := m["error"].(map[string]any)["message"].(string); !strings.Contains(msg, want) {
		t.Errorf("%s: %v", want, m)
	}
}

func gitDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	if err := os.Mkdir(filepath.Join(d, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestProposeRenameTeamAndBundleAreDraftedAsTheOldCLIDraftsThem(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	rr, ss := gitDir(t), gitDir(t)
	paths := strings.NewReplacer(`"/r"`, fmt.Sprintf("%q", rr), `"/s"`, fmt.Sprintf("%q", ss))
	call := func(rid, params string) map[string]any {
		return r.callAs("alice", "proposal.propose", fmt.Sprintf(`{%s,"request_id":%q}`, paths.Replace(params), rid))
	}
	loaded := func(res map[string]any) *Prop {
		t.Helper()
		out, _ := res["result"].(map[string]any)
		n, ok := out["proposal"].(float64)
		if !ok {
			t.Fatalf("not raised: %v", res)
		}
		p, err := r.e.load(fmt.Sprintf("%08x", int64(n)))
		if err != nil || p == nil {
			t.Fatalf("%v", err)
		}
		return p
	}
	rn := loaded(call("r1", `"kind":"rename","args":{"name":"bob","to":"robert"},"why":"w"`))
	if rn.kind() != "rename" || rn.str("agent") != r.who("bob") || rn.str("to") != "robert" {
		t.Fatalf("%+v", rn)
	}
	tm := loaded(call("t1", `"kind":"team","args":{"name":"zed","repo":"/r","members":["sea","zed.air"],"repos":{"zed.sea":"/r"}},"why":"w"`))
	if tm.str("members") != "zed.sea,zed.air" || tm.str("repo.zed.sea") != rr || tm.str("repo.zed.air") != rr {
		t.Fatalf("%+v", tm)
	}
	b := loaded(call("b1", `"kind":"bundle","args":{"name":"2 changes","changes":[{"kind":"rename","name":"carol","to":"cy"},{"kind":"add","name":"carol","repo":"/r"}]},"why":"w"`))
	if b.kind() != "bundle" || b.str("agent") != "2 changes" || len(b.items()) != 2 {
		t.Fatalf("%+v", b)
	}
	// ordered: the add comes before the rename in the apply order, so carol exists when the rename is judged
	if it := ordered(b.items()); it[0].kind() != "add" || it[1].kind() != "rename" {
		t.Fatalf("%v %v", it[0].kind(), it[1].kind())
	}
	for _, c := range []struct{ params, want string }{
		{`"kind":"rename","args":{"name":"bob"},"why":"w"`, "a rename needs a new name"},
		{`"kind":"rename","args":{"name":"bob","to":"alice"},"why":"w"`, "alice is already an agent"},
		{`"kind":"team","args":{"name":"zed","repo":"/r"},"why":"w"`, "a team needs at least one member"},
		{`"kind":"team","args":{"name":"zed","members":["sea"]},"why":"w"`, "needs --repo"},
		{`"kind":"team","args":{"name":"alice","repo":"/r","members":["sea"]},"why":"w"`, "alice already an agent"},
		{`"kind":"bundle","args":{"name":"0 changes","changes":[]},"why":"w"`, "needs at least one change"},
		{`"kind":"bundle","args":{"name":"1 changes","changes":[{"kind":"retire","name":"ghost"}]},"why":"w"`, "change 1 (retire ghost): ghost is not an agent"},
		{`"kind":"bundle","args":{"name":"1 changes","changes":[{"kind":"retire","name":"bob","bogus":1}]},"why":"w"`, "not an argument: bogus"},
	} {
		refusedWith(t, call(c.want[:5], c.params), c.want)
	}
}

func TestAPairedDeviceSetsAnotherAgentsModelAndEffort(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.device = "dev-1"
	res := r.call("agent.set", `{"agent":"alice","values":{"model":"opus","effort":"high"},"request_id":"dev-set"}`)
	if got, _ := res["result"].(map[string]any); got["accepted"] != true {
		t.Fatalf("a paired device set model and effort: %v", res)
	}
	if all := r.settingsText("alice") + r.rosterText(); !strings.Contains(all, "opus") || !strings.Contains(all, "effort: high") {
		t.Errorf("not written:\n%s", all)
	}
}

// The founder's own proposal add is applied at once, its approval on record as any other's, and proposal.list shows it.
// The repo is checked again when a proposal is applied: one raised before the check (a relative path) or whose checkout is gone since is
// refused then, with nothing registered.
func TestAnApproveChecksTheRepoAgain(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	gone := gitDir(t)
	os.RemoveAll(gone)
	for i, c := range []struct{ kind, repo, want string }{
		{"add", "handline/handline", "repo handline/handline is not an absolute path"},
		{"add", gone, "is not a folder"},
		{"team", gone, "is not a folder"},
	} {
		id := fmt.Sprintf("aaab000%d", i)
		fields := []string{"kind", c.kind, "agent", "carol", "repo", c.repo}
		if c.kind == "team" {
			fields = append(fields, "members", "carol.a", "repo.carol.a", gitDir(t))
		}
		p, n := r.propose(id, fields...)
		res := r.decide(n, "approve", p.subject(), "d"+id)
		msg, _ := res["error"].(map[string]any)["message"].(string)
		if !strings.Contains(msg, c.want) {
			t.Fatalf("%s %s: %v", c.kind, c.repo, res)
		}
		if stray := r.strayFolders(); len(stray) != 0 || paths.AgentKey(r.home, "carol") != "" {
			t.Fatalf("%s %s: carol is registered: %v", c.kind, c.repo, stray)
		}
	}
}

func TestTheFoundersOwnAddIsAppliedAtOnce(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	repo := t.TempDir()
	os.Mkdir(filepath.Join(repo, ".git"), 0o755)
	m := r.call("proposal.propose", fmt.Sprintf(`{"kind":"add","args":{"name":"carol","repo":%q},"why":"we need her","request_id":"f1"}`, repo))
	res, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("%v", m)
	}
	id := fmt.Sprintf("%08x", int64(res["proposal"].(float64)))
	if said := res["said"].(string); !strings.HasPrefix(said, "proposal "+id+" applied: carol is an agent.") {
		t.Fatalf("%q", said)
	}
	if st := r.state(id); st != "applied" {
		t.Fatalf("state %s", st)
	}
	if got := r.e.decisionOf(id); got == nil || got["call"] != "approved" {
		t.Fatalf("the approval is on record: %v", got)
	}
	if !slices.Contains(paths.ListAgents(r.home), r.who("carol")) {
		t.Fatal("registered")
	}
	if v := r.e.ProposalView(id); v == nil || v["state"] != "approved" || v["requested_by"] != "p1" || v["note"] != nil {
		t.Fatalf("proposal.list shows it, with no note: %v", v)
	}
}

// sweepEnded sweeps once after bob's turn ended and his session closed, starting agents with start.
func (r *rig) sweepEnded(start func(string) (bool, string)) {
	r.setLive("bob", false)
	r.e.SweepHeld(HeldPorts{Alive: func(a string) bool { return r.live[a] }, Idle: func(string) bool { return true },
		Close: func(string) (bool, string) { return true, "" }, Start: start})
}

// A held change is checked against the founder's approval again before it is applied: a draft changed while it waited is not applied.
func TestAHeldChangeWhoseDraftChangedWhileItWaitedIsNotApplied(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa000a", "kind", "rename", "agent", "bob", "to", "robert")
	r.setLive("bob", true)
	answer := r.decideHeld(id, p.subject(), "h1")
	if r.state("aaaa000a") != "applying" {
		t.Fatalf("held: %s", r.state("aaaa000a"))
	}
	held, err := r.e.load("aaaa000a")
	if err != nil {
		t.Fatal(err)
	}
	held.set("to", "mallory")
	if err := r.e.save(held); err != nil {
		t.Fatal(err)
	}
	r.sweepEnded(func(string) (bool, string) { return true, "" })
	if st := r.state("aaaa000a"); st != "approved" || !strings.Contains(r.file("proposals", "aaaa000a.yaml"), "was changed after the founder approved it") {
		t.Fatalf("apply_failed, with why: %s\n%s", st, r.file("proposals", "aaaa000a.yaml"))
	}
	if m := answer(); gate(m) != "state" || !strings.Contains(m["error"].(map[string]any)["message"].(string), "was changed after the founder approved it") {
		t.Fatalf("the approve is refused with why: %v", m)
	}
	if yml := r.rosterText(); !strings.Contains(yml, "  bob: {}") || strings.Contains(yml, "mallory") {
		t.Fatalf("nothing renamed:\n%s", yml)
	}
}

// followUp is proposal.list's follow_up of proposal pid (nil when none).
func (r *rig) followUp(pid string) map[string]any {
	v := r.e.ProposalView(pid)
	f, _ := v["follow_up"].(map[string]any)
	return f
}

// A renamed agent whose start is held for machine load: the change is applied (approved on the wire), the follow-up waits, and the
// sweep starts the agent by itself once the load allows, with nothing left on the proposal.
func TestAHeldRenameWhoseStartIsHeldIsAppliedAndStartsByItself(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa000b", "kind", "rename", "agent", "bob", "to", "robert")
	r.setLive("bob", true)
	answer := r.decideHeld(id, p.subject(), "h2")
	defer answer()
	r.sweepEnded(func(string) (bool, string) {
		return false, loadgate.HeldPrefix + "the computer is busy right now."
	})
	if v := r.e.ProposalView("aaaa000b"); v["state"] != "approved" || v["error"] != nil {
		t.Fatalf("applied, no error: %v", v)
	}
	f := r.followUp("aaaa000b")
	if f["agent"] != r.who("robert") || f["state"] != "waiting" || !strings.Contains(f["text"].(string), "the computer is busy right now") {
		t.Fatalf("follow-up waits: %v", f)
	}
	if !strings.Contains(r.rosterText(), "  robert: {}") {
		t.Fatal("the rename itself stands")
	}
	still := func(string) (bool, string) {
		return false, loadgate.HeldPrefix + "the computer is busy right now."
	}
	r.sweepEnded(still)
	if r.followUp("aaaa000b")["state"] != "waiting" {
		t.Fatal("still waiting while held")
	}
	var started []string
	r.sweepEnded(func(a string) (bool, string) { started = append(started, a); return true, "" })
	if !slices.Equal(started, []string{r.who("robert")}) || r.followUp("aaaa000b") != nil {
		t.Fatalf("started %v, follow-up %v", started, r.followUp("aaaa000b"))
	}
}

// A start that fails for another reason is a failed follow-up, not an apply failure, and the sweep does not retry it.
func TestAHeldRenameWhoseStartFailsIsAppliedWithAFailedFollowUp(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa000c", "kind", "rename", "agent", "bob", "to", "robert")
	r.setLive("bob", true)
	answer := r.decideHeld(id, p.subject(), "h3")
	defer answer()
	r.sweepEnded(func(string) (bool, string) { return false, "no account free" })
	if v := r.e.ProposalView("aaaa000c"); v["state"] != "approved" {
		t.Fatalf("applied: %v", v)
	}
	if f := r.followUp("aaaa000c"); f["state"] != "failed" || f["text"] != "no account free" {
		t.Fatalf("failed follow-up: %v", f)
	}
	calls := 0
	r.sweepEnded(func(string) (bool, string) { calls++; return true, "" })
	if calls != 0 {
		t.Fatal("a failed start is not retried by the sweep")
	}
}

// A failed follow-up ends when the agent is running, as after the founder's Start now; the sweep never starts it itself.
func TestAFailedFollowUpEndsWhenTheAgentIsRunning(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa000f", "kind", "rename", "agent", "bob", "to", "robert")
	r.setLive("bob", true)
	answer := r.decideHeld(id, p.subject(), "h6")
	defer answer()
	r.sweepEnded(func(string) (bool, string) { return false, "no account free" })
	if r.followUp("aaaa000f")["state"] != "failed" {
		t.Fatal("failed first")
	}
	r.setLive("robert", true)
	started := 0
	r.sweepEnded(func(string) (bool, string) { started++; return true, "" })
	if r.followUp("aaaa000f") != nil || started != 0 {
		t.Fatalf("follow-up gone, no start by the sweep: %v %d", r.followUp("aaaa000f"), started)
	}
}

// A rename whose agent starts at once leaves no follow-up.
func TestAHeldRenameThatStartsLeavesNoFollowUp(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa000d", "kind", "rename", "agent", "bob", "to", "robert")
	r.setLive("bob", true)
	answer := r.decideHeld(id, p.subject(), "h4")
	defer answer()
	r.sweepEnded(func(string) (bool, string) { return true, "" })
	if v := r.e.ProposalView("aaaa000d"); v["state"] != "approved" || r.followUp("aaaa000d") != nil {
		t.Fatalf("applied, no follow-up: %v", v)
	}
}

// A founder's proposal left waiting (raised before their own were applied at once) is listed and approved like any other.
func TestAFoundersProposalLeftWaitingIsListedAndApprovable(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa0009", "kind", "retire", "agent", "bob")
	p.set("requested_by", "p1")
	if err := r.e.save(p); err != nil {
		t.Fatal(err)
	}
	if v := r.e.ProposalView(p.id()); v == nil || v["state"] != "open" {
		t.Fatalf("listed as waiting: %v", v)
	}
	if m := r.decide(id, "approve", p.subject(), "w1"); m["error"] != nil {
		t.Fatalf("%v", m)
	}
	if st := r.state(p.id()); st != "applied" {
		t.Fatalf("state %s", st)
	}
}

// A relative --repo (and a member's SUFFIX=REPO) is taken from the proposer's own repo and kept absolute; a path that is not a git checkout,
// or a relative one from a proposer with no repo (the founder), is refused before anything is written.
func TestAProposedRepoIsKeptAbsoluteAndMustBeAGitCheckout(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	base := gitDir(t)
	for _, sub := range []string{"app", "lib"} {
		if err := os.MkdirAll(filepath.Join(base, sub, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.Mkdir(filepath.Join(base, "plain"), 0o755)
	if _, err := team.NewRoster(r.home).Register("alice", team.RegisterOpts{Role: "r", Repo: base}); err != nil {
		t.Fatal(err)
	}
	propose := func(as, rid, params string) map[string]any {
		if as == "" {
			return r.call("proposal.propose", fmt.Sprintf(`{%s,"request_id":%q}`, params, rid))
		}
		return r.callAs(as, "proposal.propose", fmt.Sprintf(`{%s,"request_id":%q}`, params, rid))
	}
	stored := func(res map[string]any) *Prop {
		t.Helper()
		out, ok := res["result"].(map[string]any)
		if !ok {
			t.Fatalf("not raised: %v", res)
		}
		p, _ := r.e.load(fmt.Sprintf("%08x", int64(out["proposal"].(float64))))
		return p
	}
	if p := stored(propose("alice", "a1", `"kind":"add","args":{"name":"carol","repo":"app"},"why":"w"`)); p.str("repo") != filepath.Join(base, "app") {
		t.Fatalf("the relative repo is alice's own repo's app: %q", p.str("repo"))
	}
	tm := stored(propose("alice", "t1", `"kind":"team","args":{"name":"zed","repo":"app","members":["sea"],"repos":{"zed.sea":"app"}},"why":"w"`))
	if tm.str("repo") != filepath.Join(base, "app") || tm.str("repo.zed.sea") != filepath.Join(base, "app") {
		t.Fatalf("%q %q", tm.str("repo"), tm.str("repo.zed.sea"))
	}
	for _, c := range []struct{ as, params, want string }{
		{"alice", `"kind":"add","args":{"name":"dan","repo":"plain"},"why":"w"`, "repo plain is not a git checkout (" + filepath.Join(base, "plain") + ")"},
		{"alice", `"kind":"add","args":{"name":"dan","repo":"handline/handline"},"why":"w"`, "repo handline/handline is not a folder"},
		{"alice", `"kind":"team","args":{"name":"zed","repo":"app","members":["sea"],"repos":{"zed.sea":"plain"}},"why":"w"`, "repo plain is not a git checkout"},
		{"alice", `"kind":"team","args":{"name":"zed","repo":"app","members":["sea"],"repos":{"zed.sea":"lib"}},"why":"w"`, "zed.sea would work in " + filepath.Join(base, "lib") + ", but a workspace has one folder"},
		{"", `"kind":"add","args":{"name":"dan","repo":"app"},"why":"w"`, "repo app is a relative path and p1 has no repo to take it from"},
	} {
		m := propose(c.as, "e"+c.want[:12], c.params)
		msg, _ := m["error"].(map[string]any)["message"].(string)
		if !strings.HasPrefix(msg, c.want) {
			t.Errorf("%s: %v", c.want, m)
		}
	}
	if stray := r.strayFolders(); len(stray) != 0 {
		t.Fatalf("a refused add registers nothing: %v", stray)
	}
}

// proposal.apply_now on a proposal waiting for a busy turn: the sweep interrupts that turn, and once the session is idle closes it and applies
// the change; the view shows since when it waited. Only an applying proposal is accepted.
func TestApplyNowInterruptsTheBusyTurnAndAppliesTheChange(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa000e", "kind", "retire", "agent", "bob")
	if m := r.call("proposal.apply_now", fmt.Sprintf(`{"proposal":%d,"request_id":"an0"}`, id)); gate(m) != "proposal_settled" {
		t.Fatalf("an open proposal is refused: %v", m)
	}
	r.setLive("bob", true)
	decided := r.decideHeld(id, p.subject(), "h5")
	if v := r.e.ProposalView("aaaa000e"); v["state"] != "applying" || v["waiting_since"] == nil {
		t.Fatalf("waits, with since when: %v", v)
	}
	busy := true
	interrupted := 0
	ports := HeldPorts{Alive: func(a string) bool { return r.live[a] }, Idle: func(string) bool { return !busy },
		Interrupt: func(string) (bool, string) { interrupted++; busy = false; return true, "" },
		Close:     func(string) (bool, string) { r.setLive("bob", false); return true, "" },
		Start:     func(string) (bool, string) { return true, "" }}
	r.e.SweepHeld(ports)
	if interrupted != 0 || r.state("aaaa000e") != "applying" {
		t.Fatalf("without apply_now the busy turn is left alone: %d %s", interrupted, r.state("aaaa000e"))
	}
	applied := r.applyNowHeld(id, "aaaa000e", "an1")
	for range 3 { // interrupt, close, apply: one step a tick
		r.e.SweepHeld(ports)
	}
	if interrupted != 1 || r.state("aaaa000e") != "applied" {
		t.Fatalf("interrupted once, then applied: %d %s", interrupted, r.state("aaaa000e"))
	}
	for _, m := range []map[string]any{applied(), decided()} {
		if res, _ := m["result"].(map[string]any); res == nil || !strings.Contains(res["said"].(string), "bob is retired") {
			t.Fatalf("apply_now and the approve both answer once it is applied: %v", m)
		}
	}
	if v := r.e.ProposalView("aaaa000e"); v["waiting_since"] != nil {
		t.Fatalf("no wait left: %v", v)
	}
}

// A rename approved a second time, after it already took effect (a card left approved-but-failed), settles as applied.
func TestApprovingARenameThatIsAlreadyInEffectSettlesItAsApplied(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa0011", "kind", "rename", "agent", "bob", "to", "robert")
	if m := r.decide(id, "approve", p.subject(), "h8"); m["result"] == nil || r.state("aaaa0011") != "applied" {
		t.Fatalf("first approve: %v %s", m, r.state("aaaa0011"))
	}
	again, err := r.e.load("aaaa0011")
	if err != nil {
		t.Fatal(err)
	}
	again.set("state", "approved")
	again.set("error", "gone wrong before")
	if err := r.e.save(again); err != nil {
		t.Fatal(err)
	}
	m := r.decide(id, "approve", again.subject(), "h9")
	if m["result"] == nil || r.state("aaaa0011") != "applied" {
		t.Fatalf("second approve settles it: %v %s", m, r.state("aaaa0011"))
	}
	if v := r.e.ProposalView("aaaa0011"); v["error"] != nil {
		t.Fatalf("no error left: %v", v)
	}
}

// A turn that is slow to end after the Esc is not pressed again on the next tick, only after the pause.
func TestApplyNowPressesEscOnceThenWaits(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	p, id := r.propose("aaaa0010", "kind", "retire", "agent", "bob")
	r.setLive("bob", true)
	r.decideHeld(id, p.subject(), "h7")
	r.applyNowHeld(id, "aaaa0010", "an2")
	interrupted := 0
	ports := HeldPorts{Alive: func(a string) bool { return r.live[a] }, Idle: func(string) bool { return false },
		Interrupt: func(string) (bool, string) { interrupted++; return true, "" },
		Close:     func(string) (bool, string) { return true, "" }, Start: func(string) (bool, string) { return true, "" }}
	for range 3 {
		r.e.SweepHeld(ports)
	}
	if interrupted != 1 {
		t.Fatalf("one Esc while the turn is slow to end: %d", interrupted)
	}
	defer func(was time.Duration) { interruptPause = was }(interruptPause)
	interruptPause = 0
	r.e.SweepHeld(ports)
	if interrupted != 2 {
		t.Fatalf("pressed again after the pause: %d", interrupted)
	}
}

// An agent proposes its own recurring work: add, edit and remove are written down for the founder, guarded, listed, and applied through the
// schedule store only when the founder approves.
func TestAnAgentProposesAScheduleAndItIsAppliedOnlyOnApproval(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	propose := func(rid, args string) map[string]any {
		return r.callAs("alice", "proposal.propose", fmt.Sprintf(`{"kind":"schedule","args":%s,"why":"a weekly review","request_id":%q}`, args, rid))
	}
	refusal := func(m map[string]any) string { msg, _ := m["error"].(map[string]any)["message"].(string); return msg }
	res := propose("s1", `{"name":"bob","action":"add","cron":"0 9 * * 1","message":"review   the open PRs"}`)["result"].(map[string]any)
	pid := fmt.Sprintf("%08x", int64(res["proposal"].(float64)))
	p, err := r.e.load(pid)
	if err != nil || p == nil || p.str("kind") != "schedule" || p.str("state") != "proposed" || p.str("requested_by") != r.who("alice") ||
		p.sets()["cron"] != "0 9 * * 1" || p.sets()["message"] != "review the open PRs" {
		t.Fatalf("%v %+v", err, p)
	}
	if strings.Contains(r.settingsText("bob"), "review") {
		t.Fatal("applied before the founder decided")
	}
	// one proposal per agent and entry
	if msg := refusal(propose("s2", `{"name":"bob","action":"add","cron":"0 9 * * 1","message":"review the open PRs"}`)); !strings.Contains(msg, "already asks for that schedule") {
		t.Fatalf("%q", msg)
	}
	// replay of the same request id answers the same proposal and writes no second one
	if again := propose("s1", `{"name":"bob","action":"add","cron":"0 9 * * 1","message":"review   the open PRs"}`)["result"].(map[string]any); again["proposal"] != res["proposal"] {
		t.Fatalf("replay: %v", again)
	}
	r.decide(int64(res["proposal"].(float64)), "approve", p.subject(), "d1")
	if got := r.settingsText("bob"); !strings.Contains(got, "review the open PRs") || !strings.Contains(got, "0 9 * * 1") {
		t.Fatalf("not applied:\n%s", got)
	}
	// edit and remove name the existing entry by its cron
	edit := propose("s3", `{"name":"bob","action":"edit","was_cron":"0 9 * * 1","cron":"0 8 * * 1"}`)["result"].(map[string]any)
	ep, _ := r.e.load(fmt.Sprintf("%08x", int64(edit["proposal"].(float64))))
	if ep.sets()["message"] != "review the open PRs" || ep.sets()["message_was"] != "review the open PRs" || ep.sets()["cron_was"] != "0 9 * * 1" {
		t.Fatalf("%+v", ep.sets())
	}
	r.decide(int64(edit["proposal"].(float64)), "approve", ep.subject(), "d2")
	if got := r.settingsText("bob"); !strings.Contains(got, "0 8 * * 1") || strings.Contains(got, "0 9 * * 1") {
		t.Fatalf("not edited:\n%s", got)
	}
	rm := propose("s4", `{"name":"bob","action":"remove","was_cron":"0 8 * * 1"}`)["result"].(map[string]any)
	rp, _ := r.e.load(fmt.Sprintf("%08x", int64(rm["proposal"].(float64))))
	r.decide(int64(rm["proposal"].(float64)), "reject", rp.subject(), "d3")
	if !strings.Contains(r.settingsText("bob"), "0 8 * * 1") {
		t.Fatal("a rejected removal removed it")
	}
	// a withdrawn proposal is taken back by its author
	wd := propose("s5", `{"name":"bob","action":"remove","was_cron":"0 8 * * 1"}`)["result"].(map[string]any)
	if m := r.callAs("alice", "proposal.withdraw", fmt.Sprintf(`{"proposal":%d,"request_id":"w1"}`, int64(wd["proposal"].(float64)))); m["error"] != nil {
		t.Fatalf("%v", m)
	}
	// neither an add nor an edit may leave two identical entries
	if msg := refusal(propose("s6", `{"name":"bob","action":"add","cron":"0 8 * * 1","message":"review the open PRs"}`)); !strings.Contains(msg, "has this schedule already") {
		t.Fatalf("add: %q", msg)
	}
	other := propose("s7", `{"name":"bob","action":"add","cron":"0 7 * * 1","message":"review the open PRs"}`)["result"].(map[string]any)
	op, _ := r.e.load(fmt.Sprintf("%08x", int64(other["proposal"].(float64))))
	r.decide(int64(other["proposal"].(float64)), "approve", op.subject(), "d4")
	if msg := refusal(propose("s8", `{"name":"bob","action":"edit","was_cron":"0 7 * * 1","cron":"0 8 * * 1"}`)); !strings.Contains(msg, "has this schedule already") {
		t.Fatalf("edit: %q", msg)
	}
}

func TestAScheduleProposalIsRefusedWhenItIsTooOftenTooLongOrNamesNothing(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	call := func(rid, args string) string {
		m := r.callAs("alice", "proposal.propose", fmt.Sprintf(`{"kind":"schedule","args":%s,"why":"w","request_id":%q}`, args, rid))
		msg, _ := m["error"].(map[string]any)["message"].(string)
		return msg
	}
	for i, c := range []struct{ args, want string }{
		{`{"name":"bob","action":"add","cron":"*/5 * * * *","message":"m"}`, "at most once an hour"},
		{`{"name":"bob","action":"add","cron":"0,30 * * * *","message":"m"}`, "at most once an hour"},
		{`{"name":"bob","action":"add","cron":"nope","message":"m"}`, "five fields"},
		{`{"name":"bob","action":"add","cron":"0 9 * * *"}`, "needs --message"},
		{`{"name":"bob","action":"add","cron":"0 9 * * *","message":"` + strings.Repeat("x", ScheduleMessageMax+1) + `"}`, "at most 1000"},
		{`{"name":"bob","action":"remove","was_cron":"0 9 * * *"}`, "has no schedule with cron"},
		{`{"name":"bob","action":"edit","cron":"0 9 * * *"}`, "say which schedule"},
		{`{"name":"bob","action":"swap","cron":"0 9 * * *","message":"m"}`, "add, edit or remove"},
		{`{"name":"ghost","action":"add","cron":"0 9 * * *","message":"m"}`, "ghost is not an agent"},
	} {
		if msg := call(fmt.Sprintf("g%d", i), c.args); !strings.Contains(msg, c.want) {
			t.Errorf("%s: %q, want %q", c.args, msg, c.want)
		}
	}
}

// TestMain sets the process environment once (storetest.Main), so the tests can run in parallel.
func TestMain(m *testing.M) {
	storetest.Main(m, []string{"HANDLINE_AGENT", "HANDLINE_TERMINAL"}, nil)
}

// An approve, and an apply_now, whose agent's turn outlasts the limit are refused: the held change is dropped (withdrawn), nothing is
// changed, and a later sweep does not apply it.
func TestAHeldChangeWhoseTurnOutlastsTheLimitIsRefusedAndDropped(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.e.Wait = 50 * time.Millisecond
	p, id := r.propose("aaaa0012", "kind", "rename", "agent", "bob", "to", "robert")
	r.setLive("bob", true)
	decided := r.decideHeld(id, p.subject(), "lim1")
	if msg := refusalOf(decided()); !strings.Contains(msg, "proposal aaaa0012 was not applied: "+r.who("bob")+" did not end the turn in time") {
		t.Fatalf("the approve is refused at the limit: %q", msg)
	}
	if st := r.state("aaaa0012"); st != "withdrawn" {
		t.Fatalf("dropped, not left held: %s", st)
	}
	r.sweepEnded(func(string) (bool, string) { t.Error("started"); return true, "" })
	if yml := r.rosterText(); !strings.Contains(yml, "  bob: {}") || strings.Contains(yml, "robert") || r.state("aaaa0012") != "withdrawn" {
		t.Fatalf("a later sweep applied it: %s\n%s", r.state("aaaa0012"), yml)
	}
}

// refusalOf is the message of the error m answers, "" for a call that was not refused.
func refusalOf(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	msg, _ := e["message"].(string)
	return msg
}

// agent.set keeps who made the change, for the restart that applies it: the founder, or the agent on itself.
func TestAgentSetKeepsWhoMadeTheChange(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	by := func() string {
		v, _, err := dbpkg.Meta(r.db, dbpkg.SettingsByMeta(r.who("alice")))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if res := r.call("agent.set", `{"agent":"alice","values":{"model":"opus"},"request_id":"b1"}`); res["result"] == nil || by() != actor.OwnerKey {
		t.Fatalf("%v %q", res, by())
	}
	if res := r.callAs("alice", "agent.set", `{"agent":"alice","values":{"effort":"high"},"request_id":"b2"}`); res["result"] == nil || by() != r.who("alice") {
		t.Fatalf("%v %q", res, by())
	}
}

// agent.set keeps how the restart for its change goes: fresh when asked, and resumed (empty) for any other set, so a later change made
// without the mode does not inherit it.
func TestAgentSetKeepsTheRestartModeOfItsChange(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	mode := func() string {
		v, _, err := dbpkg.Meta(r.db, dbpkg.SettingsRestartMeta(r.who("alice")))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if res := r.call("agent.set", `{"agent":"alice","values":{"model":"opus"},"restart":"later","request_id":"m0"}`); res["error"] == nil || mode() != "" {
		t.Fatalf("an unknown mode was taken: %v %q", res, mode())
	}
	if res := r.call("agent.set", `{"agent":"alice","values":{"model":"opus"},"restart":"fresh","request_id":"m1"}`); res["result"] == nil || mode() != "fresh" {
		t.Fatalf("%v %q", res, mode())
	}
	if res := r.call("agent.set", `{"agent":"alice","values":{"model":"sonnet"},"request_id":"m2"}`); res["result"] == nil || mode() != "" {
		t.Fatalf("a set with no mode resumes: %v %q", res, mode())
	}
}

package proposalwrites

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/handline/handline/go/internal/cmdruns"
	"github.com/handline/handline/go/protocol/rpc"
)

// fakeRuns opens no window: a start records the run as termchan does, under handle %9; fail makes the next start fail.
type fakeRuns struct {
	r       *rig
	started []string
	closed  []string
	fail    error
}

func (f *fakeRuns) StartRun(agent, cwd, command string, b cmdruns.Block) (string, error) {
	if err := f.fail; err != nil {
		f.fail = nil
		return "", err
	}
	f.started = append(f.started, agent+" "+cwd+" "+b.String()+" "+command)
	_, err := cmdruns.Start(f.r.db, b, "%9", agent, command, cmdruns.Now())
	return "%9", err
}

func (f *fakeRuns) CloseRun(handle string) error { f.closed = append(f.closed, handle); return nil }

func runRig(t *testing.T) (*rig, *fakeRuns, string) {
	r := newRig(t)
	f := &fakeRuns{r: r}
	r.e.Runs = f
	return r, f, t.TempDir()
}

// proposeRun is alice's proposal.propose of a run; it answers the reply and the new proposal's store id ("" when refused).
func (r *rig) proposeRun(as, name, command, cwd string) (map[string]any, string) {
	r.t.Helper()
	m := r.callAs(as, "proposal.propose", fmt.Sprintf(`{"kind":"run","args":{"name":%q,"command":%q,"cwd":%q},"why":"disk cleanup","request_id":"p-%s-%d"}`,
		name, command, cwd, as, len(r.e.listing())))
	res, _ := m["result"].(map[string]any)
	n, _ := res["proposal"].(float64)
	if res == nil {
		return m, ""
	}
	return m, fmt.Sprintf("%08x", int64(n))
}

func TestAnAgentProposesACommandAndTheCardShowsItWithItsMarks(t *testing.T) {
	t.Parallel()
	r, f, dir := runRig(t)
	_, pid := r.proposeRun("alice", "alice", "rm -r build && echo done", dir)
	if pid == "" {
		t.Fatal("not raised")
	}
	v := r.e.ProposalView(pid)
	run, _ := v["run"].(map[string]any)
	if v["kind"] != "run" || v["state"] != "open" || run["command"] != "rm -r build && echo done" || run["cwd"] != dir ||
		!slices.Equal(run["risks"].([]string), []string{cmdruns.RiskDelete}) || run["state"] != nil || len(f.started) != 0 {
		t.Fatalf("%v", v)
	}
	if !strings.Contains(v["diff"].(string), "run in "+dir) || !strings.Contains(v["diff"].(string), "rm -r build") {
		t.Fatalf("diff: %q", v["diff"])
	}
}

func TestARunProposalIsCheckedWhenRaised(t *testing.T) {
	t.Parallel()
	r, _, dir := runRig(t)
	for _, c := range []struct{ name, command, cwd, want string }{
		{"bob", "ls", dir, "proposes its own commands"},
		{"alice", "", dir, "needs a command"},
		{"alice", strings.Repeat("x", 4001), dir, "needs a command"},
		{"alice", "ls", "", "folder it runs in"},
		{"alice", "ls", filepath.Join(dir, "nope"), "is not a folder"},
		{"alice", "ls", "relative", "has no worktree"},
	} {
		m, pid := r.proposeRun("alice", c.name, c.command, c.cwd)
		if e, _ := m["error"].(map[string]any); pid != "" || !strings.Contains(fmt.Sprint(e["message"]), c.want) {
			t.Errorf("%+v: %v", c, m)
		}
	}
	for i := range maxOpenRuns {
		if _, pid := r.proposeRun("alice", "alice", fmt.Sprintf("echo %d", i), dir); pid == "" {
			t.Fatalf("run %d refused", i)
		}
	}
	if m, pid := r.proposeRun("alice", "alice", "echo six", dir); pid != "" || !strings.Contains(fmt.Sprint(m), "5 run proposals waiting") {
		t.Fatalf("a sixth open run was raised: %v", m)
	}
	if _, pid := r.proposeRun("bob", "bob", "echo bob", dir); pid == "" {
		t.Fatal("the limit is per agent")
	}
	m := r.callAs("alice", "proposal.propose", fmt.Sprintf(`{"kind":"bundle","args":{"name":"1 changes","changes":[{"kind":"run","name":"alice","command":"ls","cwd":%q}]},"why":"x","request_id":"b1"}`, dir))
	if m["error"] == nil {
		t.Fatalf("a run went into a bundle: %v", m)
	}
}

// The founder's approve starts the command once, beside its agent, who is told it runs; an agent can neither decide nor stop it.
func TestTheFoundersApproveRunsTheCommandOnce(t *testing.T) {
	t.Parallel()
	r, f, dir := runRig(t)
	_, pid := r.proposeRun("alice", "alice", "rm -r build", dir)
	n, _ := hexNum(pid)
	subject := r.e.ProposalView(pid)["subject"].(string)
	for _, method := range []string{"proposal.decide", "proposal.stop"} {
		m := r.callAs("alice", method, fmt.Sprintf(`{"proposal":%d,"decision":"approve","subject":%q,"request_id":"a-%s"}`, n, subject, method))
		if gate(m) != "role" || len(f.started) != 0 {
			t.Fatalf("an agent called %s on its own run: %v", method, m)
		}
	}
	if m := r.decide(n, "approve", subject, "d1"); m["result"] == nil {
		t.Fatalf("%v", m)
	}
	if want := r.who("alice") + " " + dir + " p" + fmt.Sprint(n) + ":0 rm -r build"; !slices.Equal(f.started, []string{want}) || r.state(pid) != "applied" {
		t.Fatalf("%q %s", f.started, r.state(pid))
	}
	if told := r.told("alice"); len(told) == 0 || !strings.Contains(told[len(told)-1], "its command runs in window %9") {
		t.Fatalf("alice: %q", told)
	}
	if run := r.e.ProposalView(pid)["run"].(map[string]any); run["state"] != "running" || run["terminal"] != "%9" {
		t.Fatalf("%v", run)
	}
	if m := r.decide(n, "approve", subject, "d2"); gate(m) != "proposal_settled" || len(f.started) != 1 {
		t.Fatalf("approved twice: %v %q", m, f.started)
	}
	if m := r.call("proposal.stop", fmt.Sprintf(`{"proposal":%d,"request_id":"s1"}`, n)); m["result"] == nil || !slices.Equal(f.closed, []string{"%9"}) {
		t.Fatalf("stop: %v %q", m, f.closed)
	}
}

// A start that failed leaves it approved and the founder may approve it again; a start that ran never runs again, even if it failed.
func TestAFailedStartIsRetriedAndARunCommandIsNever(t *testing.T) {
	t.Parallel()
	r, f, dir := runRig(t)
	_, pid := r.proposeRun("alice", "alice", "make clean", dir)
	n, _ := hexNum(pid)
	subject := r.e.ProposalView(pid)["subject"].(string)
	f.fail = rpc.Refused("bob has 8 shells open; close one first", "state", "")
	r.decide(n, "approve", subject, "d1")
	if v := r.e.ProposalView(pid); v["state"] != "apply_failed" || !strings.Contains(fmt.Sprint(v["error"]), "8 shells open") || len(f.started) != 0 {
		t.Fatalf("%v", v)
	}
	r.decide(n, "approve", subject, "d2")
	if len(f.started) != 1 || r.state(pid) != "applied" {
		t.Fatalf("not retried: %q %s", f.started, r.state(pid))
	}
	p, _ := r.e.load(pid) // as if its record failed after the window opened: approved, with the run there
	p.set("state", "approved")
	r.e.save(p)
	if m := r.decide(n, "approve", subject, "d3"); !strings.Contains(fmt.Sprint(m["error"]), "runs once") || len(f.started) != 1 {
		t.Fatalf("a started command ran again: %v %q", m, f.started)
	}
}

// What the founder approved is what runs: a command edited in the file after the approve does not start.
func TestACommandEditedAfterTheApproveDoesNotRun(t *testing.T) {
	t.Parallel()
	r, f, dir := runRig(t)
	_, pid := r.proposeRun("alice", "alice", "ls", dir)
	n, _ := hexNum(pid)
	f.fail = errors.New("no host")
	r.decide(n, "approve", r.e.ProposalView(pid)["subject"].(string), "d1") // the approval is on record; the start failed
	path := filepath.Join(r.home, "proposals", pid+".yaml")
	b, _ := os.ReadFile(path)
	edited := strings.Replace(string(b), `set.command: "\"ls\""`, `set.command: "\"rm -rf ~\""`, 1)
	if edited == string(b) {
		t.Fatalf("no command to edit in:\n%s", b)
	}
	os.WriteFile(path, []byte(edited), 0o644)
	if ok, said, _ := r.e.apply(pid, false); ok || !strings.Contains(said, "changed after the founder approved it") || len(f.started) != 0 {
		t.Fatalf("an edited command ran: %v %s %q", ok, said, f.started)
	}
}

func TestARejectedRunNeverStarts(t *testing.T) {
	t.Parallel()
	r, f, dir := runRig(t)
	_, pid := r.proposeRun("alice", "alice", "ls", dir)
	n, _ := hexNum(pid)
	r.decide(n, "reject", "", "d1")
	if r.state(pid) != "rejected" || len(f.started) != 0 {
		t.Fatalf("%s %q", r.state(pid), f.started)
	}
	if m := r.call("proposal.stop", fmt.Sprintf(`{"proposal":%d,"request_id":"s1"}`, n)); gate(m) != "state" {
		t.Fatalf("stop of a command never started: %v", m)
	}
}

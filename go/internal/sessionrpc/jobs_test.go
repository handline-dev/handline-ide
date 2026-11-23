package sessionrpc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/agent/claudecode"
	"github.com/handline/handline/go/agentctl/term"
)

// pidHost is the test terminal whose pane runs this test's process, so the shells a test starts are in its tree.
type pidHost struct{ *host }

func (pidHost) PanePid(string) int { return os.Getpid() }

func jobsRig(t *testing.T, jobs []RunningJob) (*rig, *host, *conn) {
	r, h, _ := typingRig(t)
	r.e.Term = term.New(pidHost{h}, claudecode.ScreenFor)
	r.e.Running = func(agent string) []RunningJob {
		if agent != r.ann {
			t.Errorf("Running(%q), want ann's key", agent)
		}
		return jobs
	}
	return r, h, r.dial(false)
}

// shell starts `sh -c command` under this test's process in a group of its own, and ends it when the test does.
func shell(t *testing.T, command string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); cmd.Wait() })
	return cmd
}

// started records a background shell of ann's that writes out.
func started(r *rig, id, out string) {
	r.e.DB.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(50, ?, 'SessionStart', '{}')", r.ann)
	r.e.DB.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(60, ?, 'background_started', ?)", r.ann, fmt.Sprintf(`{"job":%q,"output":%q}`, id, out))
}

func TestAgentJobsListsWhatTheLastStopSaidWithAShellsLastLines(t *testing.T) {
	out := filepath.Join(t.TempDir(), "b1.output")
	os.WriteFile(out, []byte("one\n\ntwo\nthree\nfour\nfive\n"+strings.Repeat("x", 300)+"\n"), 0o644)
	r, _, c := jobsRig(t, []RunningJob{{ID: "b1", Type: "shell", Name: "shell: build"}, {ID: "a1", Type: "agent", Name: "agent: review"}})
	started(r, "b1", out)
	shell(t, fmt.Sprintf("sleep 300 >>%q", out)) // something still writes it
	got := result(t, c.call("agent.jobs", map[string]any{"agent": "ann"}))["jobs"].([]any)
	shellJob, other := got[0].(map[string]any), got[1].(map[string]any)
	lines := shellJob["output"].([]any)
	if len(got) != 2 || shellJob["id"] != "b1" || shellJob["started"] != float64(60) || len(lines) != OutputLines || lines[0] != "two" ||
		len(lines[4].(string)) != OutputWidth {
		t.Fatalf("shell: %v", shellJob)
	}
	if other["output"] != nil || other["started"] != nil || other["type"] != "agent" {
		t.Fatalf("a job that is no shell: %v", other)
	}
}

// The job's group is the nearest one of its own, under the agent's CLI, among the processes holding its output: never the CLI's,
// which reads that file too, never one shared with what is above it.
func TestTheJobsGroupIsItsOwnBelowTheCLI(t *testing.T) {
	procs := []proc{{1, 0, 1}, {10, 1, 10}, {20, 10, 20}, {21, 20, 20}, {30, 10, 10}, {40, 99, 40}}
	for _, c := range []struct {
		open []int
		want int
	}{{[]int{10, 20, 21}, 20}, {[]int{21, 10}, 0}, {[]int{10}, 0}, {[]int{30}, 0}, {[]int{40}, 0}, {nil, 0}} {
		got, err := jobGroup(procs, 1, c.open)
		if got != c.want || (err != nil) != (c.want == 0) {
			t.Errorf("open %v: %d %v, want %d", c.open, got, err, c.want)
		}
	}
}

// A shell stopped ends; the agent's CLI above it, which waits on it, goes on; agent.jobs no longer lists it.
func TestStopOneShellEndsItAndNothingAbove(t *testing.T) {
	out := filepath.Join(t.TempDir(), "b1.output")
	os.WriteFile(out, nil, 0o644)
	cli := shell(t, fmt.Sprintf("set -m; sleep 300 >>%q & wait; sleep 300", out)) // a group of its own, as Claude Code's tool shell is
	r, _, c := jobsRig(t, []RunningJob{{ID: "b1", Type: "shell", Name: "shell: sleeper"}, {ID: "a1", Type: "agent", Name: "agent: review"}})
	started(r, "b1", out)
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) { // until the job holds its output
		if open, _ := holders([]string{out}); len(open[out]) > 0 {
			break
		}
		if time.Now().After(end) {
			t.Fatal("the job never started")
		}
	}
	result(t, c.call("agent.job.stop", map[string]any{"agent": "ann", "job": "b1", "request_id": rid("s1")}))
	for end := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		jobs := result(t, c.call("agent.jobs", map[string]any{"agent": "ann"}))["jobs"].([]any)
		if len(jobs) == 1 {
			break
		}
		if time.Now().After(end) {
			t.Fatalf("still listed: %v", jobs)
		}
	}
	if err := cli.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("the CLI above it was stopped too: %v", err)
	}
	if res := c.call("agent.job.stop", map[string]any{"agent": "ann", "job": "b1", "request_id": rid("s2")}); res["error"] == nil {
		t.Fatalf("an ended job: %v", res)
	}
	if g := refused(c.call("agent.job.stop", map[string]any{"agent": "ann", "job": "a1", "request_id": rid("s3")})); g != "type" {
		t.Fatalf("no shell: gate %q", g)
	}
}

func TestStopAllPressesCtrlXCtrlKTwiceOnlyWhileSomethingRuns(t *testing.T) {
	r, h, c := jobsRig(t, nil)
	if g := refused(c.call("agent.jobs.stop", map[string]any{"agent": "ann", "request_id": rid("none")})); g != "state" || len(h.sent) > 0 {
		t.Fatalf("with nothing running: gate %q, sent %q", g, h.sent)
	}
	r.e.Running = func(string) []RunningJob { return []RunningJob{{ID: "a1", Type: "agent", Name: "agent: review"}} }
	h.rows = []string{"", "────────────────────────────────", "❯ ", "────────────────────────────────", "  ? for shortcuts"} // its prompt
	result(t, c.call("agent.jobs.stop", map[string]any{"agent": "ann", "request_id": rid("all")}))
	if !slices.Equal(h.sent, []string{stopAllKeys, stopAllKeys}) {
		t.Fatalf("sent %q", h.sent)
	}
}

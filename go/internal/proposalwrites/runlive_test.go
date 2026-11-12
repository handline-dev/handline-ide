package proposalwrites

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/data/agenthost"
	"github.com/handline/handline/go/internal/cmdruns"
	"github.com/handline/handline/go/internal/termchan"
	"github.com/handline/handline/go/kit/testkill"
)

// End to end on a real tmux server: an agent proposes `rm -r` of a folder, the founder approves, the window runs it, the folder is gone and
// the agent is told the exit and its window's rows. A command edited in the file after the approve, and a rejected one, run nothing.
func TestARunProposalRunsInARealWindowAndItsAgentIsToldHowItEnded(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	sock := "hlrun" + time.Now().Format("150405.000")
	t.Setenv("HANDLINE_TMUX_SOCKET", sock)
	t.Setenv("SHELL", "/bin/sh")
	if out, err := exec.Command("tmux", "-L", sock, "new-session", "-d", "-s", "handline", "-x", "80", "-y", "20", "cat").CombinedOutput(); err != nil {
		t.Skipf("tmux cannot run here: %v %s", err, out)
	}
	file, _ := exec.Command("tmux", "-L", sock, "display-message", "-p", "#{socket_path}").Output()
	testkill.OnEnd(t, func() {
		exec.Command("tmux", "-L", sock, "kill-server").Run()
		os.Remove(strings.TrimSpace(string(file)))
	})
	r := newRig(t)
	tm := agenthost.Tmux("")
	tc := termchan.New(r.db, r.home, func() term.Backend { return tm })
	r.e.Runs = tc
	work := t.TempDir()
	junk := filepath.Join(work, "junk")
	for _, d := range []string{junk, filepath.Join(work, "keep")} {
		if err := os.MkdirAll(filepath.Join(d, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	settle := func(pid string) cmdruns.Run {
		t.Helper()
		n, _ := hexNum(pid)
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if err := cmdruns.Settle(r.db, tc.PaneRuns(), cmdruns.Now(), cmdruns.Telling(tc.Tail)); err != nil {
				t.Fatal(err)
			}
			if run, ok, _ := cmdruns.OfProposalRun(r.db, n); ok && run.State != "running" {
				return run
			}
		}
		t.Fatal("the command did not end")
		return cmdruns.Run{}
	}

	_, pid := r.proposeRun("alice", "alice", "rm -r junk && echo removed", work)
	n, _ := hexNum(pid)
	if m := r.decide(n, "approve", r.e.ProposalView(pid)["subject"].(string), "d1"); m["result"] == nil {
		t.Fatalf("%v", m)
	}
	if run := settle(pid); run.State != "done" || *run.Exit != 0 {
		t.Fatalf("%+v", run)
	}
	if _, err := os.Stat(junk); !os.IsNotExist(err) {
		t.Fatalf("the folder is still there: %v", err)
	}
	told := r.told("alice")
	if len(told) == 0 || !strings.HasPrefix(told[len(told)-1], fmt.Sprintf("The run of your proposed command (proposal %s) ended with exit 0", pid)) ||
		!strings.Contains(told[len(told)-1], "removed") {
		t.Fatalf("alice: %q", told)
	}
	if v := r.e.ProposalView(pid)["run"].(map[string]any); v["state"] != "done" || v["exit"] != 0 {
		t.Fatalf("%v", v)
	}

	windows := func() int { return len(tm.ListTerminals()) }
	before := windows()
	_, edited := r.proposeRun("alice", "alice", "true", work)
	path := filepath.Join(r.home, "proposals", edited+".yaml")
	r.e.Runs = nil // the first start fails, so the approval is on record and the apply can be tried again
	en, _ := hexNum(edited)
	r.decide(en, "approve", r.e.ProposalView(edited)["subject"].(string), "d2")
	r.e.Runs = tc
	b, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(b), `\"true\"`, `\"rm -r keep\"`, 1)), 0o644)
	if ok, said, _ := r.e.apply(edited, false); ok || !strings.Contains(said, "changed after the founder approved it") {
		t.Fatalf("%v %s", ok, said)
	}
	_, rejected := r.proposeRun("alice", "alice", "rm -r keep", work)
	rn, _ := hexNum(rejected)
	r.decide(rn, "reject", "", "d3")
	if _, err := os.Stat(filepath.Join(work, "keep")); err != nil || windows() != before {
		t.Fatalf("an edited or rejected command ran: %v, %d windows, %d before", err, windows(), before)
	}
}

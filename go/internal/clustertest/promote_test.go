package clustertest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/cluster"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/protocol/rpc"
)

// MoveWait bounds each wait of a move: a runner's heartbeat goes every 15 s, and the old master is dialled as often.
const MoveWait = 90 * time.Second

// PollEvery is how often a wait asks.
const PollEvery = 3 * time.Second

// waitFor asks ok every PollEvery until it holds: each ask is a founder's hello, which the master records, and a runner's copy is
// said to hold all of the board only after a pull finds nothing new.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(MoveWait); !ok(); time.Sleep(PollEvery) {
		if time.Now().After(end) {
			t.Fatalf("no %s after %s", what, MoveWait)
		}
	}
}

// row is node's row in n's node.status, nil when n lists none or does not answer.
func row(n *cluster.Node, node string) map[string]any {
	got, err := n.Call("node.status", nil)
	if err != nil {
		return nil
	}
	list, _ := got["nodes"].([]any)
	for _, v := range list {
		if r, _ := v.(map[string]any); r["id"] == node {
			return r
		}
	}
	return nil
}

// isMaster is whether n answers as master node with number.
func isMaster(n *cluster.Node, node string, number int64) bool {
	got, err := n.Call("node.status", nil)
	if err != nil {
		return false
	}
	m, _ := got["master"].(map[string]any)
	return m["id"] == node && fmt.Sprint(m["number"]) == fmt.Sprint(number)
}

// ready waits until runner r can be made the master and runner other can find it: r holds the board's copy, and other was told r's
// door.
func ready(t *testing.T, c *cluster.Cluster, r, other *cluster.Node) {
	t.Helper()
	node := joined.NodeIDOf(r.Home)
	waitFor(t, r.Name+"'s copy of the board", func() bool { rw := row(c.Master, node); return rw != nil && rw["copy"] != nil })
	waitFor(t, other.Name+" told of "+r.Name+"'s door", func() bool {
		b, _ := os.ReadFile(runner.DoorsFile(other.Home))
		return strings.Contains(string(b), node)
	})
}

// result is what a promote run in the background printed.
type result struct {
	out string
	err error
}

// asked waits until runner r asked the master to hand over.
func asked(t *testing.T, r *cluster.Node) {
	t.Helper()
	waitFor(t, r.Name+" asking the handover", func() bool {
		b, _ := os.ReadFile(r.Log())
		return strings.Contains(string(b), "runner: asking the master to hand over")
	})
}

// joinedAsRunner waits until old runs as a runner of master, linked to it, with aside boards set aside.
func joinedAsRunner(t *testing.T, master, old *cluster.Node, aside int) {
	t.Helper()
	node := joined.NodeIDOf(old.Home)
	waitFor(t, old.Name+" linked to "+master.Name+" as a runner", func() bool { rw := row(master, node); return rw != nil && rw["online"] == true })
	m, ok := old.Joined()
	if !ok || m.Node != joined.NodeIDOf(master.Home) {
		t.Fatalf("%s joined %+v", old.Name, m)
	}
	if got, _ := filepath.Glob(filepath.Join(old.Home, "db.set-aside-*.sqlite")); len(got) != aside {
		t.Fatalf("%s set aside %v, not %d", old.Name, got, aside)
	}
}

// A planned move loses no write: writes go to the master while runner1 is made the master; every write the master answered is on
// runner1 after, the master joins it as a runner with its board set aside, and runner2 finds it.
func TestAPlannedMoveLosesNoWrite(t *testing.T) {
	c := Start(t, Opts{Runners: 2})
	r1, r2 := c.Runners[0], c.Runners[1]
	ready(t, c, r1, r2)
	var mu sync.Mutex
	var kept []string
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			name := fmt.Sprintf("w%d", i)
			if _, err := c.Master.Call("run.set", map[string]any{"name": name, "scope": "mac", "cmds": []any{"make " + name},
				"request_id": "move-" + name}); err == nil {
				mu.Lock()
				kept = append(kept, name)
				mu.Unlock()
			}
		}
	}()
	waitFor(t, "writes before the move", func() bool { mu.Lock(); defer mu.Unlock(); return len(kept) >= 5 })
	out, err := r1.Hl("node", "promote")
	close(stop)
	wg.Wait()
	if err != nil || !strings.Contains(out, "handed over") || !strings.Contains(out, "(number 2)") {
		t.Fatalf("promote: %v: %s", err, out)
	}
	node1 := joined.NodeIDOf(r1.Home)
	waitFor(t, "runner1 as master number 2", func() bool { return isMaster(r1, node1, 2) })
	got, err := r1.Call("run.list", nil)
	if err != nil {
		t.Fatal(err)
	}
	have := fmt.Sprint(got)
	for _, name := range kept {
		if !strings.Contains(have, "make "+name+"]") && !strings.Contains(have, "make "+name+" ") {
			t.Fatalf("the write %s the master answered is not on the new master (%d answered): %s", name, len(kept), have)
		}
	}
	joinedAsRunner(t, r1, c.Master, 1)
	waitFor(t, "runner2 linked to runner1", func() bool { rw := row(r1, joined.NodeIDOf(r2.Home)); return rw != nil && rw["online"] == true })
}

// The master killed: runner1 made the master takes over from its copy; runner2 and a phone paired with the old master reach it with
// their tokens; the old master started again becomes its runner, its board set aside.
func TestAKilledMasterIsTakenOverAndJoinsAsARunnerWhenItIsBack(t *testing.T) {
	c := Start(t, Opts{Runners: 2})
	r1, r2 := c.Runners[0], c.Runners[1]
	phone := pairPhone(t, c.Master)
	ready(t, c, r1, r2)
	if err := c.Master.Kill(); err != nil {
		t.Fatal(err)
	}
	out, err := r1.Hl("node", "promote")
	if err != nil || !strings.Contains(out, "(number 2), from its copy") {
		t.Fatalf("promote: %v: %s", err, out)
	}
	node1 := joined.NodeIDOf(r1.Home)
	waitFor(t, "runner1 as master number 2", func() bool { return isMaster(r1, node1, 2) })
	waitFor(t, "runner2 linked to runner1", func() bool { rw := row(r1, joined.NodeIDOf(r2.Home)); return rw != nil && rw["online"] == true })
	if says, err := hlclient.Ask(r1.Addr, "", phone, "phone", nil); err != nil || says.Number != 2 {
		t.Fatalf("the phone's token on the new master: %+v %v", says, err)
	}
	if err := c.Master.Restart(); err != nil {
		t.Fatal(err)
	}
	joinedAsRunner(t, r1, c.Master, 1)
}

// A master paused through the move (a laptop asleep) is made a runner once it runs again: the new master dials it until it answers.
func TestAMasterPausedThroughTheMoveBecomesARunnerOnceItRuns(t *testing.T) {
	c := Start(t, Opts{Runners: 2})
	r1, r2 := c.Runners[0], c.Runners[1]
	ready(t, c, r1, r2)
	if err := c.Master.Pause(); err != nil {
		t.Fatal(err)
	}
	out, err := r1.Hl("node", "promote")
	if err != nil || !strings.Contains(out, "(number 2), from its copy") {
		c.Master.Resume()
		t.Fatalf("promote: %v: %s", err, out)
	}
	node1 := joined.NodeIDOf(r1.Home)
	waitFor(t, "runner1 as master number 2", func() bool { return isMaster(r1, node1, 2) })
	if err := c.Master.Resume(); err != nil {
		t.Fatal(err)
	}
	joinedAsRunner(t, r1, c.Master, 1)
	waitFor(t, "runner2 linked to runner1", func() bool { rw := row(r1, joined.NodeIDOf(r2.Home)); return rw != nil && rw["online"] == true })
}

// pairPhone pairs a phone with master through its network door and answers the phone's device token.
func pairPhone(t *testing.T, master *cluster.Node) string {
	t.Helper()
	got, err := master.Call("device.pair", map[string]any{"name": "phone", "request_id": "pair-phone"})
	if err != nil {
		t.Fatal(err)
	}
	code, _ := got["code"].(string)
	ws, err := hlclient.DialMaster(master.Addr, "")
	if err != nil {
		t.Fatal(err)
	}
	l := rpc.NewLink(rpc.NewWSTransport(ws), nil)
	ended := make(chan error, 1)
	go func() { ended <- l.Run(time.Minute) }()
	defer func() { l.Close(); <-ended }()
	res, err := l.Call("device.redeem", map[string]any{"code": code, "name": "phone", "kind": "ios", "request_id": "redeem-phone"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := res["token"].(string)
	if token == "" {
		t.Fatalf("redeem: %v", res)
	}
	return token
}

// The old master started again while runner1's promote still waits on its handover starts as master; runner1 takes over from its
// copy with the higher number and the old master joins it as a runner.
func TestAMasterRestartedDuringTheHandoverBecomesARunner(t *testing.T) {
	c := Start(t, Opts{Runners: 2})
	r1, r2 := c.Runners[0], c.Runners[1]
	ready(t, c, r1, r2)
	if err := c.Master.Pause(); err != nil {
		t.Fatal(err)
	}
	promoted := make(chan result, 1)
	go func() { out, err := r1.Hl("node", "promote"); promoted <- result{out, err} }()
	asked(t, r1)
	if err := c.Master.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := c.Master.Restart(); err != nil {
		t.Fatal(err)
	}
	got := <-promoted
	if got.err != nil || !strings.Contains(got.out, "(number 2), from its copy") {
		t.Fatalf("promote: %v: %s", got.err, got.out)
	}
	node1 := joined.NodeIDOf(r1.Home)
	waitFor(t, "runner1 as master number 2", func() bool { return isMaster(r1, node1, 2) })
	joinedAsRunner(t, r1, c.Master, 1)
}

// The same after a first move: runner1, master number 2, started again while runner2's promote waits on its handover joins runner2,
// master number 3.
func TestASecondMasterRestartedDuringTheHandoverBecomesARunner(t *testing.T) {
	c := Start(t, Opts{Runners: 2})
	r1, r2 := c.Runners[0], c.Runners[1]
	ready(t, c, r1, r2)
	if out, err := r1.Hl("node", "promote"); err != nil || !strings.Contains(out, "(number 2)") {
		t.Fatalf("promote: %v: %s", err, out)
	}
	node1, node2 := joined.NodeIDOf(r1.Home), joined.NodeIDOf(r2.Home)
	waitFor(t, "runner1 as master number 2", func() bool { return isMaster(r1, node1, 2) })
	waitFor(t, "runner2 linked to runner1", func() bool { rw := row(r1, node2); return rw != nil && rw["online"] == true })
	waitFor(t, "runner2's copy of runner1's board", func() bool { rw := row(r1, node2); return rw != nil && rw["copy"] != nil })
	if err := r1.Pause(); err != nil {
		t.Fatal(err)
	}
	promoted := make(chan result, 1)
	go func() { out, err := r2.Hl("node", "promote"); promoted <- result{out, err} }()
	asked(t, r2)
	if err := r1.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := r1.Restart(); err != nil {
		t.Fatal(err)
	}
	if got := <-promoted; got.err != nil || !strings.Contains(got.out, "(number 3), from its copy") {
		t.Fatalf("promote: %v: %s", got.err, got.out)
	}
	waitFor(t, "runner2 as master number 3", func() bool { return isMaster(r2, node2, 3) })
	joinedAsRunner(t, r2, r1, 2)
}

// Make master pressed on the master (device.make_master): the runner answers that it takes over, the master hands over to it and
// joins it as a runner. Pressed on the runner it is node.promote, as in the tests above.
func TestMakeMasterOnTheMasterMovesItToThatDevice(t *testing.T) {
	c := Start(t, Opts{Runners: 2})
	r1, r2 := c.Runners[0], c.Runners[1]
	ready(t, c, r1, r2)
	node1 := joined.NodeIDOf(r1.Home)
	device, _ := row(c.Master, node1)["device"].(string)
	if device == "" {
		t.Fatalf("no device on %s's row: %v", r1.Name, row(c.Master, node1))
	}
	if _, err := c.Master.Call("device.make_master", map[string]any{"device": device, "request_id": "make-master"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "runner1 as master number 2", func() bool { return isMaster(r1, node1, 2) })
	b, _ := os.ReadFile(r1.Log())
	if !strings.Contains(string(b), "runner: asking the master to hand over") {
		t.Fatalf("%s took over without asking the master", r1.Name)
	}
	joinedAsRunner(t, r1, c.Master, 1)
	waitFor(t, "runner2 linked to runner1", func() bool { rw := row(r1, joined.NodeIDOf(r2.Home)); return rw != nil && rw["online"] == true })
}

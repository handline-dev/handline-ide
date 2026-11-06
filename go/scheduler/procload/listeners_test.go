package procload

import (
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

func TestParseLsof(t *testing.T) {
	out := "p100\nPTCP\nf5\ntIPv4\nn127.0.0.1:3000\nf6\ntIPv6\nn[::1]:3000\np200\nf7\ntIPv6\nn*:8080\nf8\ntIPv4\nn*:8080\np300\nf9\ntIPv4\nn10.0.0.2:99999\n"
	want := []Listener{{100, "127.0.0.1", 3000}, {100, "::1", 3000}, {200, "::", 8080}, {200, "0.0.0.0", 8080}}
	if got := parseLsof(out); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestServers(t *testing.T) {
	rows := []Raw{
		{PID: 100, PPID: 1, Start: 1, Cmd: "-zsh"},                  // alice's pane shell
		{PID: 101, PPID: 100, Start: 2, Cmd: "/usr/bin/claude --x"}, // her session
		{PID: 102, PPID: 101, Start: 3, Cmd: "node mcp-server.js"},  // an MCP server it spawned
		{PID: 103, PPID: 101, Start: 4, Cmd: "/bin/zsh -c eval"},    // the Bash tool's shell
		{PID: 104, PPID: 103, Start: 5, Cmd: "node dev-server.js"},  // a server she started
		{PID: 105, PPID: 104, Start: 6, Cmd: "node worker.js"},      // its child
		{PID: 200, PPID: 1, Start: 1, Cmd: "unrelated"},             // nobody's
		{PID: 300, PPID: 100, Start: 7, Cmd: "node proxy.js"},       // another process under her pane
	}
	listen := []Listener{{101, "127.0.0.1", 7000}, {102, "127.0.0.1", 7001}, {104, "::1", 5173}, {104, "127.0.0.1", 5173}, {104, "::", 5174},
		{105, "0.0.0.0", 9000}, {200, "0.0.0.0", 80}, {999, "0.0.0.0", 81}, {100, "0.0.0.0", 22}, {300, "0.0.0.0", 1234}}
	tr := tracker(&rows, map[int]string{}, map[string]int{"alice": 100})
	tr.Listen = func() ([]Listener, error) { return listen, nil }
	if err := tr.Sample(1000); err != nil {
		t.Fatal(err)
	}
	got := tr.ServersOf("alice")
	want := []Server{{300, 1234, "0.0.0.0", "node proxy.js", 7}, {104, 5173, "127.0.0.1", "node dev-server.js", 5},
		{104, 5174, "::", "node dev-server.js", 5}, {105, 9000, "0.0.0.0", "node worker.js", 6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if n := len(tr.ServersOf("bob")); n != 0 {
		t.Fatalf("bob has %d", n)
	}
}

func TestListenersReadEvery15s(t *testing.T) {
	rows := []Raw{{PID: 100, PPID: 1, Start: 1, Cmd: "-zsh"}, {PID: 104, PPID: 100, Start: 5, Cmd: "node a.js"}}
	reads := 0
	tr := tracker(&rows, map[int]string{}, map[string]int{"alice": 100})
	tr.Listen = func() ([]Listener, error) { reads++; return []Listener{{104, "127.0.0.1", 5173}}, nil }
	for _, at := range []int64{1000, 1005, 1010, 1015, 1020} {
		if err := tr.Sample(at); err != nil {
			t.Fatal(err)
		}
	}
	if reads != 2 {
		t.Fatalf("read %d times, want 2 (at 1000 and 1015)", reads)
	}
	rows = rows[:1] // the server ends before the next read: gone at once, the cached sockets notwithstanding
	if err := tr.Sample(1025); err != nil {
		t.Fatal(err)
	}
	if n := len(tr.ServersOf("alice")); n != 0 {
		t.Fatalf("an ended process still serves: %d", n)
	}
}

func TestServersLeavesOutTheDaemon(t *testing.T) {
	procs := []Proc{{PID: 5, Agent: "alice", Cmd: "daemon", Start: 1}, {PID: 6, Agent: "alice", Cmd: "node", Start: 1}}
	got := serversOf([]Listener{{5, "127.0.0.1", 1}, {6, "127.0.0.1", 2}}, procs, nil, 5)
	if len(got["alice"]) != 1 || got["alice"][0].Port != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestFailedReadKeepsTheLastServers(t *testing.T) {
	rows := []Raw{{PID: 100, PPID: 1, Start: 1, Cmd: "-zsh"}, {PID: 104, PPID: 100, Start: 5, Cmd: "node a.js"}}
	fail := false
	tr := tracker(&rows, map[int]string{}, map[string]int{"alice": 100})
	tr.Listen = func() ([]Listener, error) {
		if fail {
			return nil, errors.New("timed out")
		}
		return []Listener{{104, "127.0.0.1", 5173}}, nil
	}
	tr.Sample(1000)
	fail = true
	tr.Sample(1015)
	if n := len(tr.ServersOf("alice")); n != 1 {
		t.Fatalf("a failed read dropped the servers: %d", n)
	}
}

func TestAnEndedProcessIsNotAServerWhateverTheCachedSocketsSay(t *testing.T) {
	procs := []Proc{{PID: 6, Agent: "alice", Cmd: "node", Start: 1, Gone: true}}
	if got := serversOf([]Listener{{6, "127.0.0.1", 2}, {7, "127.0.0.1", 3}}, procs, nil, 1); len(got) != 0 {
		t.Fatalf("a gone process and one out of the table serve: %+v", got)
	}
	// through Sample: the process leaves the table, and its last row (Gone, kept for the view) is not a server either
	rows := []Raw{{PID: 100, PPID: 1, Start: 1, Cmd: "-zsh"}, {PID: 104, PPID: 100, Start: 5, Cmd: "node a.js"}}
	tr := tracker(&rows, map[int]string{}, map[string]int{"alice": 100})
	tr.Listen = func() ([]Listener, error) { return []Listener{{104, "127.0.0.1", 5173}}, nil }
	tr.Sample(1000)
	rows = rows[:1]
	tr.Sample(1005)
	if n := len(tr.ServersOf("alice")); n != 0 {
		t.Fatalf("%d", n)
	}
}

func TestTheSessionRuleIsPerAgentAndLeavesShellsAlone(t *testing.T) {
	procs := []Proc{
		{PID: 1, Agent: "alice", Cmd: "/bin/claude", Start: 1},
		{PID: 2, PPID: 1, Agent: "bob", Cmd: "node bob-server", Start: 1}, // bob's, though its parent is alice's claude
		{PID: 3, PPID: 1, Agent: "alice", Cmd: "/bin/zsh -c eval", Start: 1},
		{PID: 4, PPID: 1, Agent: "alice", Cmd: "node mcp", Start: 1},
		{PID: 5, Agent: "", Cmd: "stranger", Start: 1},
	}
	ls := []Listener{{2, "127.0.0.1", 12}, {3, "127.0.0.1", 13}, {4, "127.0.0.1", 14}, {5, "127.0.0.1", 15}}
	got := serversOf(ls, procs, nil, 0)
	if len(got["bob"]) != 1 || len(got["alice"]) != 1 || got["alice"][0].PID != 3 || len(got) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestOneAddressPerPortAndNoPortZero(t *testing.T) {
	procs := []Proc{{PID: 6, Agent: "alice", Cmd: "node", Start: 1}}
	got := serversOf([]Listener{{6, "10.0.0.2", 2}, {6, "10.0.0.1", 2}, {6, "::", 3}, {6, "0.0.0.0", 3}}, procs, nil, 0)["alice"]
	if len(got) != 2 || got[0].Addr != "10.0.0.1" || got[1].Addr != "0.0.0.0" {
		t.Fatalf("%+v", got)
	}
	if ls := parseLsof("p6\ntIPv4\nn*:0\nn*:70000\nn127.0.0.1:1\n"); len(ls) != 1 || ls[0].Port != 1 {
		t.Fatalf("%+v", ls)
	}
}

func TestLsofExitsAreToldApart(t *testing.T) {
	exitErr := func(code string) error { return exec.Command("sh", "-c", "exit "+code).Run() }
	if ls, err := lsofResult(nil, exitErr("1")); err != nil || ls == nil || len(ls) != 0 {
		t.Fatalf("exit 1 with no output is an empty read: %v %v", ls, err)
	}
	if ls, err := lsofResult(nil, exitErr("2")); err == nil || ls != nil {
		t.Fatalf("exit 2 is a failure: %v %v", ls, err)
	}
	if _, err := lsofResult([]byte("p1\n"), exitErr("1")); err == nil {
		t.Fatal("exit 1 with output is a failure")
	}
}

func TestAnEmptyReadClearsAndAFailedReadWaitsToo(t *testing.T) {
	rows := []Raw{{PID: 100, PPID: 1, Start: 1, Cmd: "-zsh"}, {PID: 104, PPID: 100, Start: 5, Cmd: "node a.js"}}
	tr := tracker(&rows, map[int]string{}, map[string]int{"alice": 100})
	reads, mode := 0, "some"
	tr.Listen = func() ([]Listener, error) {
		reads++
		switch mode {
		case "fail":
			return nil, errors.New("x")
		case "none":
			return nil, nil
		}
		return []Listener{{104, "127.0.0.1", 5173}}, nil
	}
	tr.Sample(1000)
	mode = "none"
	tr.Sample(1015)
	if n := len(tr.ServersOf("alice")); n != 0 {
		t.Fatalf("a read of nothing keeps %d", n)
	}
	mode = "fail"
	tr.Sample(1030)
	for _, at := range []int64{1035, 1040} {
		tr.Sample(at)
	}
	if reads != 3 {
		t.Fatalf("a failed read ran again before ListenEvery: %d reads", reads)
	}
}

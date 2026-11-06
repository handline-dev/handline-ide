package procload

import (
	"errors"
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

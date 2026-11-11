package sessionrpc

import (
	"bufio"
	"errors"
	"net"
	"testing"

	"github.com/handline/handline/go/protocol/rpc"
)

// dialAs is a connection of process pid on transport.
func (r *rig) dialAs(transport string, pid int) *conn {
	srv := rpc.NewServer(auth{}, "sha", 1, transport, func(string, ...any) {})
	srv.DB, srv.Who = r.e.DB, r.who
	Register(srv, r.e)
	cl, sv := net.Pipe()
	go func() { srv.Serve(rpc.NewLineTransport(sv), pid); sv.Close() }()
	r.t.Cleanup(func() { cl.Close() })
	c := &conn{c: cl, r: bufio.NewReader(cl), t: r.t}
	result(r.t, c.call("hello", map[string]any{"client": "t/1", "protocol": 1}))
	return c
}

func TestALaunchIsPreparedForAProcessOnTheLocalSocketWithItsOwnPid(t *testing.T) {
	r := newRig(t)
	var got LaunchRequest
	r.e.Launch = func(req LaunchRequest) (LaunchResult, error) {
		got = req
		return LaunchResult{Argv: []string{"/bin/claude"}, Env: []string{"A=b"}, Warnings: []string{"careful"}}, nil
	}
	params := map[string]any{"agent": "ann", "extra": []any{"--resume", "s1"}, "force": true, "account": "main", "cwd": "/w",
		"env": map[string]any{"PATH": "/bin"}, "binary": "/bin/claude", "handle": "%3", "pid": 1}
	res := result(t, r.dialAs("local", 4242).call("session.launch_prepare", params))
	if got.Agent != r.ann || got.PID != 4242 || !got.Force || got.Cwd != "/w" || got.Env["PATH"] != "/bin" || len(got.Extra) != 2 || got.Handle != "%3" {
		t.Fatalf("request %+v", got)
	}
	if argv, _ := res["argv"].([]any); len(argv) != 1 || argv[0] != "/bin/claude" || res["warnings"].([]any)[0] != "careful" {
		t.Fatalf("result %v", res)
	}
}

func TestALaunchIsNotPreparedOffTheLocalSocketForAStrangerOrWithBadInput(t *testing.T) {
	r := newRig(t)
	r.e.Launch = func(LaunchRequest) (LaunchResult, error) { t.Error("prepared"); return LaunchResult{}, nil }
	ok := map[string]any{"agent": "ann", "cwd": "/w"}
	if g := refused(r.dialAs("network", 4242).call("session.launch_prepare", ok)); g != "role" {
		t.Errorf("network: gate %q", g)
	}
	c := r.dialAs("local", 4242)
	if c.call("session.launch_prepare", map[string]any{"agent": "nobody", "cwd": "/w"})["error"] == nil {
		t.Error("an agent off the roster is served")
	}
	if f := badField(c.call("session.launch_prepare", map[string]any{"agent": "ann", "cwd": "w"})); f != "cwd" {
		t.Errorf("relative cwd: field %q", f)
	}
	if f := badField(c.call("session.launch_prepare", map[string]any{"agent": "ann", "cwd": "/w", "extra": "x"})); f != "extra" {
		t.Errorf("extra: field %q", f)
	}
}

func TestARefusedLaunchKeepsItsWords(t *testing.T) {
	r := newRig(t)
	r.e.Launch = func(LaunchRequest) (LaunchResult, error) {
		return LaunchResult{}, &LaunchRefusal{Msg: "account 'a' is not logged in."}
	}
	m := r.dialAs("local", 4242).call("session.launch_prepare", map[string]any{"agent": "ann", "cwd": "/w"})
	if msg, _ := m["error"].(map[string]any)["message"].(string); msg != "account 'a' is not logged in." || refused(m) != "state" {
		t.Fatalf("%v", m)
	}
	r.e.Launch = func(LaunchRequest) (LaunchResult, error) { return LaunchResult{}, errors.New("boom") }
	if m := r.dialAs("local", 4242).call("session.launch_prepare", map[string]any{"agent": "ann", "cwd": "/w"}); m["error"] == nil || refused(m) != "" {
		t.Fatalf("%v", m)
	}
}

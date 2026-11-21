package termchan

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// linkRelay is a runner's link played in one process: the master's Relay calls on one side, the runner's Streams on the other.
type linkRelay struct {
	runner *Streams

	mu    sync.Mutex
	next  int
	subs  map[string][2]func(map[string]any) // on, ended (as func(nil))
	calls []string
}

func (r *linkRelay) log(s string) { r.mu.Lock(); r.calls = append(r.calls, s); r.mu.Unlock() }

func (r *linkRelay) Stream(node, method string, params map[string]any, on func(map[string]any), ended func()) (map[string]any, string, func(), error) {
	r.mu.Lock()
	r.next++
	id := fmt.Sprintf("s%d", r.next)
	r.subs[id] = [2]func(map[string]any){on, func(map[string]any) { ended() }}
	r.mu.Unlock()
	r.log(method + " " + id + " " + params["handle"].(string))
	notify := func(method string, p map[string]any) bool {
		r.mu.Lock()
		s, ok := r.subs[p["stream"].(string)]
		r.mu.Unlock()
		if ok && method == "runner.output" {
			s[0](p)
		}
		return true
	}
	reply, err := r.runner.Attach(id, params["handle"].(string), params["history"].(int), notify)
	if err != nil {
		return nil, "", nil, err
	}
	stop := func() { // after the link ended there is no runner to tell
		r.mu.Lock()
		_, open := r.subs[id]
		delete(r.subs, id)
		r.mu.Unlock()
		if open {
			r.log("runner.detach " + id)
			r.runner.Detach(id)
		}
	}
	return reply, id, stop, nil
}

func (r *linkRelay) Call(node, method string, params map[string]any) (map[string]any, error) {
	r.log(method + " " + fmt.Sprint(params["stream"]))
	if method == "runner.resync" {
		return map[string]any{}, r.runner.Resync(params["stream"].(string), params["reason"].(string))
	}
	return nil, fmt.Errorf("no %s", method)
}

// drop is the link ending: each master stream ends, and the runner ends its own.
func (r *linkRelay) drop() {
	r.mu.Lock()
	subs := r.subs
	r.subs = map[string][2]func(map[string]any){}
	r.mu.Unlock()
	for _, s := range subs {
		s[1](nil)
	}
	r.runner.DetachAll()
}

func decoded(t *testing.T, m map[string]any) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(m["params"].(map[string]any)["data"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Two app clients of a runner's terminal each get a stream from the runner and the runner's snapshot; the pane's output reaches both
// under the master's handle with their own seq; a client's resync is the runner's snapshot in its stream; the last client to leave
// ends the stream on the runner, and a link that drops hangs up the app clients still attached.
func TestARunnersTerminalIsRelayedToEachAppClient(t *testing.T) {
	p := newPipeProc()
	l := linkOn(p)
	rl := &linkRelay{runner: NewStreams(func() *ControlLink { return l }), subs: map[string][2]func(map[string]any){}}
	hub := NewBytesHub()
	ch := hub.relayChannel("box:%1", rl, "box", "%1")

	attach := func(in *inbox, closed *bool) *BytesClient {
		t.Helper()
		c := NewBytesClient(in.send, func() { *closed = true })
		got := make(chan map[string]any, 1)
		go func() {
			reply, err := ch.attachRelayed(c, 0)
			if err != nil {
				t.Error(err)
			}
			got <- reply
		}()
		if cmd := p.line(t); !strings.Contains(cmd, "-t %1") { // the runner captures its own pane
			t.Fatalf("captured %q", cmd)
		}
		p.reply(modesLine(0, 10, 1, 0))
		p.reply("hello")
		reply := <-got
		if b, _ := base64.StdEncoding.DecodeString(reply["data"].(string)); !strings.Contains(string(b), "hello") {
			t.Fatalf("snapshot %q", b)
		}
		ch.Begin(c)
		return c
	}
	in1, in2 := &inbox{}, &inbox{}
	var closed1, closed2 bool
	c1 := attach(in1, &closed1)
	p.output("%1", "abc")
	if m := in1.wait(t, 1)[0]; m["method"] != "output" || decoded(t, m) != "abc" ||
		m["params"].(map[string]any)["terminal"] != "box:%1" || m["params"].(map[string]any)["seq"] != 1 {
		t.Fatalf("first output %v", m)
	}
	c2 := attach(in2, &closed2)
	p.output("%1", "xyz")
	if m := in1.wait(t, 2)[1]; decoded(t, m) != "xyz" {
		t.Fatalf("c1 got %v", m)
	}
	if m := in2.wait(t, 1)[0]; decoded(t, m) != "xyz" || m["params"].(map[string]any)["seq"] != 1 {
		t.Fatalf("c2 got %v", m)
	}

	// A marker on the master takes the client's next number; a resync is the runner's snapshot, numbered by the master.
	hub.Notify("box:%1", "marker", map[string]any{"kind": "control"})
	if m := in1.wait(t, 3)[2]; m["method"] != "marker" || m["params"].(map[string]any)["seq"] != 3 {
		t.Fatalf("marker %v", m)
	}
	ch.resync(c1, "slow_client")
	p.line(t)
	p.reply(modesLine(0, 10, 1, 0))
	p.reply("fresh")
	m := in1.wait(t, 4)[3]
	prm := m["params"].(map[string]any)
	if m["method"] != "snapshot" || prm["reason"] != "slow_client" || prm["terminal"] != "box:%1" || prm["seq"] != 4 || !strings.Contains(decoded(t, m), "fresh") {
		t.Fatalf("resync %v", m)
	}
	p.output("%1", "after")
	if m := in1.wait(t, 5)[4]; decoded(t, m) != "after" {
		t.Fatalf("output after the resync %v", m)
	}

	ch.Detach(c2)
	rl.drop()
	if !closed1 || closed2 {
		t.Fatalf("the link's drop hangs up the clients still attached: c1 %v c2 %v", closed1, closed2)
	}
	ch.Detach(c1)
	want := []string{"runner.attach s1 %1", "runner.attach s2 %1", "runner.resync s1", "runner.detach s2"}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if strings.Join(rl.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("the link carried %q, want %q", rl.calls, want)
	}
}

// A resync whose snapshot the runner cannot take leaves the master's client taking output again, as a local one that fails does.
func TestARelayedResyncThatFailsOnTheRunnerLetsOutputThrough(t *testing.T) {
	p := newPipeProc()
	l := linkOn(p)
	rl := &linkRelay{runner: NewStreams(func() *ControlLink { return l }), subs: map[string][2]func(map[string]any){}}
	ch := NewBytesHub().relayChannel("box:%1", rl, "box", "%1")
	in := &inbox{}
	c := NewBytesClient(in.send, func() { t.Error("hung up") })
	done := make(chan error, 1)
	go func() { _, err := ch.attachRelayed(c, 0); done <- err }()
	p.line(t)
	p.reply(modesLine(0, 10, 1, 0))
	p.reply("hello")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ch.Begin(c)
	ch.resync(c, "slow_client")
	p.line(t)
	p.fail() // the modes could not be read: tmux runs none of the line
	for end := time.Now().Add(3 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		c.mu.Lock()
		waiting := c.resyncing
		c.mu.Unlock()
		if !waiting {
			break
		}
		if time.Now().After(end) {
			t.Fatal("the client still waits on a snapshot that will not come")
		}
	}
	p.output("%1", "on")
	if m := in.wait(t, 1)[0]; decoded(t, m) != "on" {
		t.Fatalf("%v", m)
	}
}

// A detach the runner gets before the attach it ends is kept: that attach is refused, so no stream goes up the link unread.
func TestADetachBeforeItsAttachRefusesTheAttach(t *testing.T) {
	s := NewStreams(func() *ControlLink { return linkOn(newPipeProc()) })
	s.Detach("s9")
	if _, err := s.Attach("s9", "%1", 0, func(string, map[string]any) bool { return true }); err == nil {
		t.Fatal("an attach its master gave up on was kept")
	}
}

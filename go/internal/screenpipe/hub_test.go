package screenpipe

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/protocol/rpc"
)

type auth struct{ agent string }

func (a auth) AgentOf(int) string         { return a.agent }
func (a auth) FounderKeyOK(k string) bool { return a.agent == "" && k == "founder" }
func (a auth) SessionsReadable() error    { return nil }

// fake is a source that sends what the test gives it.
type fake struct {
	mu                         sync.Mutex
	started, stopped, released int
	frame                      func(Frame)
	ended                      func()
	inputs                     []Input
	releaseErr                 error
	gate                       chan struct{} // when set, Start waits for it
}

func (f *fake) Start(frame func(Frame), ended func()) error {
	f.mu.Lock()
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	f.mu.Lock()
	f.started++
	f.frame, f.ended = frame, ended
	f.mu.Unlock()
	go frame(Frame{JPEG: []byte("first"), Width: 1280, Height: 657})
	return nil
}
func (f *fake) Stop() { f.mu.Lock(); f.stopped++; f.mu.Unlock() }
func (f *fake) Input(ev Input) error {
	f.mu.Lock()
	f.inputs = append(f.inputs, ev)
	f.mu.Unlock()
	return nil
}
func (f *fake) Release() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.releaseErr != nil {
		return f.releaseErr
	}
	f.released++
	return nil
}
func (f *fake) send(jpeg string) {
	f.mu.Lock()
	frame := f.frame
	f.mu.Unlock()
	frame(Frame{JPEG: []byte(jpeg), Width: 1280, Height: 657})
}
func (f *fake) counts() (int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started, f.stopped, f.released
}

// world is a hub with one agent, a1, whose browser is the fake, and servers for the founder and for a1.
type world struct {
	hub            *Hub
	src            *fake
	founder, agent *rpc.Server
	told           []string
	mu             sync.Mutex
}

func newWorld(t *testing.T) *world {
	db, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	w := &world{src: &fake{}}
	w.hub = &Hub{Open: func(agent, source string) (Source, error) {
		if agent != "a1" {
			return nil, ErrNotOpen
		}
		return w.src, nil
	}, Tell: func(agent, text string) { w.mu.Lock(); w.told = append(w.told, agent+": "+text); w.mu.Unlock() }}
	for _, as := range []string{"", "a1"} {
		s := rpc.NewServer(auth{as}, "x", store.LatestVersion(), "local", func(string, ...any) {})
		s.DB = db
		w.hub.Register(s)
		if as == "" {
			w.founder = s
		} else {
			w.agent = s
		}
	}
	return w
}

func (w *world) heard() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string{}, w.told...)
}

// client is one open connection: replies by id, notices in order.
type client struct {
	t       *testing.T
	cl      net.Conn
	mu      sync.Mutex
	next    int
	replies map[int]map[string]any
	notices []map[string]any
	got     chan struct{}
}

func dial(t *testing.T, s *rpc.Server, key string) *client {
	t.Helper()
	cl, sv := net.Pipe()
	go func() { s.Serve(rpc.NewLineTransport(sv), 0); sv.Close() }()
	c := &client{t: t, cl: cl, next: 1, replies: map[int]map[string]any{}, got: make(chan struct{}, 1000)}
	t.Cleanup(func() { cl.Close() })
	go func() {
		rd := bufio.NewReader(cl)
		for {
			b, err := rd.ReadBytes('\n')
			if err != nil {
				return
			}
			var m map[string]any
			json.Unmarshal(b, &m)
			c.mu.Lock()
			if id, ok := m["id"].(float64); ok {
				c.replies[int(id)] = m
			} else {
				c.notices = append(c.notices, m)
			}
			c.mu.Unlock()
			c.got <- struct{}{}
		}
	}()
	c.call("hello", `"client":"t","protocol":1,"key":"`+key+`"`)
	return c
}

// call sends method and waits for its reply.
func (c *client) call(method, params string) map[string]any {
	c.t.Helper()
	c.mu.Lock()
	id := c.next
	c.next++
	c.mu.Unlock()
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":{%s}}`+"\n", id, method, params)
	if _, err := c.cl.Write([]byte(line)); err != nil {
		c.t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for {
		c.mu.Lock()
		r := c.replies[id]
		c.mu.Unlock()
		if r != nil {
			return r
		}
		select {
		case <-c.got:
		case <-deadline:
			c.t.Fatalf("no reply to %s", method)
		}
	}
}

// notice waits for the first notice named method after the first skip of them.
func (c *client) notice(method string, skip int) map[string]any {
	c.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		c.mu.Lock()
		seen := 0
		for _, n := range c.notices {
			if n["method"] == method {
				if seen == skip {
					c.mu.Unlock()
					p, _ := n["params"].(map[string]any)
					return p
				}
				seen++
			}
		}
		c.mu.Unlock()
		select {
		case <-c.got:
		case <-deadline:
			c.t.Fatalf("no %s notice (after %d)", method, skip)
		}
	}
}

func (c *client) count(method string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, x := range c.notices {
		if x["method"] == method {
			n++
		}
	}
	return n
}

func result(t *testing.T, r map[string]any) map[string]any {
	t.Helper()
	res, ok := r["result"].(map[string]any)
	if !ok {
		t.Fatalf("not accepted: %v", r["error"])
	}
	return res
}

func refusal(r map[string]any) string {
	e, _ := r["error"].(map[string]any)
	if e == nil {
		return ""
	}
	return fmt.Sprint(e["message"])
}

var rid = 0

func req() string { rid++; return fmt.Sprintf(`,"request_id":"r%d"`, rid) }

const page = `"agent":"a1","source":"browser"`

func TestAttachAnswersTheCurrentFrameAndASourceSendsOnlyWhileWatched(t *testing.T) {
	w := newWorld(t)
	c := dial(t, w.founder, "founder")
	if e, _ := c.call("screen.attach", `"agent":"a2","source":"browser"`)["error"].(map[string]any); e == nil ||
		e["data"].(map[string]any)["what"] != "source" {
		t.Fatalf("an agent with no browser open: %v", e)
	}
	snap := result(t, c.call("screen.attach", page))
	frame, _ := snap["frame"].(map[string]any)
	if frame == nil || frame["data"] != "Zmlyc3Q=" || frame["width"] != float64(1280) || snap["seq"] != float64(1) {
		t.Fatalf("the attach's frame: %v", snap)
	}
	if h := snap["holder"].(map[string]any); h["kind"] != "agent" || h["name"] != "a1" {
		t.Errorf("holder = %v", h)
	}
	w.src.send("second")
	if n := c.notice("screen_frame", 0); n["seq"] != float64(2) || n["data"] != "c2Vjb25k" || n["agent"] != "a1" {
		t.Errorf("the next frame: %v", n)
	}
	other := dial(t, w.founder, "founder")
	result(t, other.call("screen.attach", page))
	if started, _, _ := w.src.counts(); started != 1 {
		t.Errorf("a second watcher started the source again: %d starts", started)
	}
	result(t, c.call("screen.detach", page))
	if _, stopped, _ := w.src.counts(); stopped != 0 {
		t.Error("the source stopped while someone still watched it")
	}
	other.cl.Close() // a hang-up is a detach
	stopsOnce(t, w.src, "the source kept sending with nobody watching")
	result(t, c.call("screen.attach", page))
	if started, _, _ := w.src.counts(); started != 2 {
		t.Errorf("a watcher after nobody did started nothing: %d starts", started)
	}
}

func TestInputIsRefusedUnlessTheCallerHoldsTheSource(t *testing.T) {
	w := newWorld(t)
	c := dial(t, w.founder, "founder")
	click := `,"event":{"kind":"mouse","action":"down","x":10,"y":20,"button":"left"}`
	if got := refusal(c.call("screen.take", page+req())); !strings.Contains(got, "attach") {
		t.Errorf("a take by a caller that does not watch: %q", got)
	}
	result(t, c.call("screen.attach", page))
	if got := refusal(c.call("screen.input", page+click+req())); !strings.Contains(got, "take it first") {
		t.Errorf("input with nobody holding: %q", got)
	}
	taken := result(t, c.call("screen.take", page+req()))
	if h := taken["holder"].(map[string]any); h["kind"] != "person" {
		t.Errorf("the taker's hold: %v", h)
	}
	if n := c.notice("screen_holder", 0); n["holder"].(map[string]any)["kind"] != "person" {
		t.Errorf("holder notice: %v", n)
	}
	result(t, c.call("screen.input", page+click+req()))
	w.src.mu.Lock()
	got := w.src.inputs
	w.src.mu.Unlock()
	if len(got) != 1 || got[0].Kind != "mouse" || got[0].Action != "down" || got[0].X != 10 || got[0].Button != "left" {
		t.Errorf("input reached the source as %+v", got)
	}
	w.hub.mu.Lock()
	w.hub.holds[key{"a1", "browser"}].name = "someone else"
	w.hub.mu.Unlock()
	if got := refusal(c.call("screen.input", page+click+req())); got != "someone else has the page" {
		t.Errorf("input while another holds it: %q", got)
	}
	if got := refusal(c.call("screen.give_back", page+req())); got != "someone else has the page" {
		t.Errorf("a give back by who does not hold it: %q", got)
	}
	if got := refusal(c.call("screen.input", page+`,"event":{"kind":"key","action":"down"}`+req())); got == "" {
		t.Error("a key with no key was taken")
	}
}

func TestEveryPageVerbIsRefusedWhileHeldAndGivingBackReleasesAndTellsTheAgentOnce(t *testing.T) {
	w := newWorld(t)
	c := dial(t, w.founder, "founder")
	a := dial(t, w.agent, "")
	result(t, a.call("screen.act", `"source":"browser","kind":"open"`))
	result(t, c.call("screen.attach", page))
	result(t, c.call("screen.give_back", page+req())) // nobody held it: nothing to say
	result(t, c.call("screen.take", page+req()))
	name := result(t, c.call("screen.take", page+req()))["holder"].(map[string]any)["name"].(string)
	for _, kind := range append(append([]string{}, acting...), reading...) {
		if got := refusal(a.call("screen.act", `"source":"browser","kind":"`+kind+`"`)); got != name+" has the page" {
			t.Errorf("%s while held: %q", kind, got)
		}
	}
	if got := w.heard(); len(got) != 0 {
		t.Fatalf("the agent was told %v before the give back", got)
	}
	back := result(t, c.call("screen.give_back", page+req()))
	if back["holder"].(map[string]any)["kind"] != "agent" {
		t.Errorf("after give back: %v", back)
	}
	if _, _, released := w.src.counts(); released != 1 {
		t.Errorf("the give back released the source %d times", released)
	}
	if got := w.heard(); len(got) != 1 || got[0] != "a1: "+name+" gave the page back" {
		t.Errorf("the agent was told %v", got)
	}
	result(t, a.call("screen.act", `"source":"browser","kind":"read"`))
	// A holder that stops watching gives it back, and an agent not refused meanwhile hears nothing.
	result(t, c.call("screen.take", page+req()))
	result(t, c.call("screen.detach", page))
	if _, _, released := w.src.counts(); released != 2 {
		t.Errorf("the holder's detach released %d times in all", released)
	}
	if got := w.heard(); len(got) != 1 {
		t.Errorf("an agent never refused was told %v", got)
	}
	if got := refusal(c.call("screen.act", `"source":"browser","kind":"read"`)); got == "" {
		t.Error("a person called the agent's own screen.act")
	}
}

func TestAnActionReachesWatchersWithItsBoxAndNeverTheText(t *testing.T) {
	w := newWorld(t)
	c := dial(t, w.founder, "founder")
	a := dial(t, w.agent, "")
	result(t, c.call("screen.attach", page))
	result(t, a.call("screen.act", `"source":"browser","kind":"type"`)) // before the box: a check only
	result(t, a.call("screen.act", `"source":"browser","kind":"read"`))
	result(t, a.call("screen.act", `"source":"browser","kind":"type","text":"hunter2","box":{"x":1,"y":2,"width":30,"height":4}`))
	n := c.notice("screen_action", 0)
	box, _ := n["box"].(map[string]any)
	if n["kind"] != "type" || box == nil || box["x"] != float64(1) || box["height"] != float64(4) || n["agent"] != "a1" {
		t.Errorf("the type action: %v", n)
	}
	raw, _ := json.Marshal(n)
	if strings.Contains(string(raw), "hunter2") || strings.Contains(string(raw), "text") {
		t.Errorf("the action carries what was typed: %s", raw)
	}
	result(t, a.call("screen.act", `"source":"browser","kind":"open"`))
	if n := c.notice("screen_action", 1); n["kind"] != "open" || n["box"] != nil {
		t.Errorf("open: %v", n)
	}
	if got := c.count("screen_action"); got != 2 {
		t.Errorf("%d actions sent; the check before the box and the read send none", got)
	}
	if got := refusal(a.call("screen.act", `"source":"browser","kind":"click","box":{"x":1}`)); got == "" {
		t.Error("a box with no size was taken")
	}
}

func TestASourceIsInUseUntilItsTimeAfterTheLastActionAndClosingIt(t *testing.T) {
	if DefaultInUse != 20*time.Second {
		t.Errorf("in use lasts %s", DefaultInUse)
	}
	w := newWorld(t)
	var mu sync.Mutex
	n := 0
	w.hub.InUse = 200 * time.Millisecond
	w.hub.Changed = func(agent string) { mu.Lock(); n++; mu.Unlock() }
	a := dial(t, w.agent, "")
	if got := w.hub.Sources("a1"); len(got) != 0 {
		t.Fatalf("sources before any use: %v", got)
	}
	result(t, a.call("screen.act", `"source":"browser","kind":"open"`))
	if got := fmt.Sprint(w.hub.Sources("a1")); got != "[map[in_use:true source:browser]]" {
		t.Errorf("after open: %s", got)
	}
	time.Sleep(100 * time.Millisecond)
	result(t, a.call("screen.act", `"source":"browser","kind":"read"`)) // the time runs from the last action
	time.Sleep(150 * time.Millisecond)
	if got := fmt.Sprint(w.hub.Sources("a1")); got != "[map[in_use:true source:browser]]" {
		t.Errorf("150 ms after the last action: %s", got)
	}
	time.Sleep(150 * time.Millisecond)
	if got := fmt.Sprint(w.hub.Sources("a1")); got != "[map[in_use:false source:browser]]" {
		t.Errorf("past its time: %s", got)
	}
	mu.Lock()
	if n != 3 { // opened, in use, then no longer in use
		t.Errorf("the view was told %d times", n)
	}
	mu.Unlock()
	result(t, a.call("screen.act", `"source":"browser","kind":"close"`))
	if got := w.hub.Sources("a1"); len(got) != 0 {
		t.Errorf("after close: %v", got)
	}
}

func TestAHoldWhosePasswordsCannotBeEmptiedStaysHeld(t *testing.T) {
	w := newWorld(t)
	c := dial(t, w.founder, "founder")
	a := dial(t, w.agent, "")
	result(t, c.call("screen.attach", page))
	name := result(t, c.call("screen.take", page+req()))["holder"].(map[string]any)["name"].(string)
	w.src.mu.Lock()
	w.src.releaseErr = errors.New("the page did not answer")
	w.src.mu.Unlock()
	if got := refusal(c.call("screen.give_back", page+req())); !strings.Contains(got, "could not be emptied") {
		t.Errorf("a give back that could not clear the page: %q", got)
	}
	if got := refusal(a.call("screen.act", `"source":"browser","kind":"eval"`)); got != name+" has the page" {
		t.Errorf("the agent after a failed give back: %q", got)
	}
	w.src.mu.Lock()
	w.src.releaseErr = nil
	w.src.mu.Unlock()
	result(t, c.call("screen.give_back", page+req()))
	result(t, a.call("screen.act", `"source":"browser","kind":"eval"`))
}

func TestASourceThatEndsTellsItsWatchersAndIsClearedBeforeTheAgentGoesOn(t *testing.T) {
	w := newWorld(t)
	c := dial(t, w.founder, "founder")
	a := dial(t, w.agent, "")
	result(t, c.call("screen.attach", page))
	name := result(t, c.call("screen.take", page+req()))["holder"].(map[string]any)["name"].(string)
	refusal(a.call("screen.act", `"source":"browser","kind":"read"`))
	w.src.mu.Lock()
	ended := w.src.ended
	w.src.mu.Unlock()
	ended()
	if n := c.notice("screen_ended", 0); n["agent"] != "a1" {
		t.Errorf("ended: %v", n)
	}
	if _, stopped, released := w.src.counts(); stopped < 1 || released != 1 {
		t.Errorf("after the end: %d stops, %d releases", stopped, released)
	}
	if got := w.heard(); len(got) != 1 || got[0] != "a1: "+name+" gave the page back" {
		t.Errorf("told %v", got)
	}
	if got := refusal(c.call("screen.take", page+req())); !strings.Contains(got, "attach") {
		t.Errorf("a take after the end: %q", got)
	}
}

func TestALeaveWhileTheSourceStartsStopsItOnceItHasStarted(t *testing.T) {
	w := newWorld(t)
	w.src.gate = make(chan struct{})
	conn, k := &rpc.Conn{}, key{"a1", "browser"}
	joined := make(chan error, 1)
	go func() { _, err := w.hub.join(conn, k); joined <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w.hub.mu.Lock()
		ch := w.hub.chans[key{"a1", "browser"}]
		w.hub.mu.Unlock()
		if ch != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the attach made no channel")
		}
		time.Sleep(5 * time.Millisecond)
	}
	w.hub.leave(conn, k)
	close(w.src.gate)
	if err := <-joined; !errors.Is(err, ErrNotOpen) {
		t.Errorf("the attach left while it started: %v", err)
	}
	stopsOnce(t, w.src, "a source left while it started kept running")
}

// stopsOnce waits for f's first stop, failing with why after 5 s.
func stopsOnce(t *testing.T, f *fake, why string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for _, stopped, _ := f.counts(); stopped != 1; _, stopped, _ = f.counts() {
		if time.Now().After(deadline) {
			t.Fatal(why)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAnAttachToNoSourceLeavesNothingRunning(t *testing.T) {
	w := newWorld(t)
	c := dial(t, w.founder, "founder")
	c.call("screen.attach", `"agent":"a2","source":"browser"`)
	before := runtime.NumGoroutine()
	for range 20 {
		c.call("screen.attach", `"agent":"a2","source":"browser"`)
	}
	time.Sleep(100 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Errorf("20 attaches to no source left %d goroutines more", after-before)
	}
}

func TestADeviceIsHeldAndShownAsThePageIsUnderItsOwnWords(t *testing.T) {
	w := newWorld(t)
	const dev = `"agent":"a1","source":"sim"`
	c := dial(t, w.founder, "founder")
	a := dial(t, w.agent, "")
	result(t, c.call("screen.attach", dev))
	result(t, a.call("screen.act", `"source":"sim","kind":"tap"`)) // before the target is found: a check only
	result(t, a.call("screen.act", `"source":"sim","kind":"tap","box":{"x":100,"y":200,"width":0,"height":0}`))
	result(t, a.call("screen.act", `"source":"sim","kind":"type"`))
	if n := c.notice("screen_action", 0); n["kind"] != "tap" || n["source"] != "sim" || n["box"].(map[string]any)["y"] != float64(200) {
		t.Errorf("the tap: %v", n)
	}
	if n := c.notice("screen_action", 1); n["kind"] != "type" || n["box"] != nil {
		t.Errorf("a device's typing has no box and is shown: %v", n)
	}
	if got := c.count("screen_action"); got != 2 {
		t.Errorf("%d actions sent, want the boxed tap and the type", got)
	}
	result(t, c.call("screen.take", dev+req()))
	name := result(t, c.call("screen.take", dev+req()))["holder"].(map[string]any)["name"].(string)
	for _, kind := range []string{"tap", "swipe", "type", "button", "rotate", "launch", "install", "close"} {
		if got := refusal(a.call("screen.act", `"source":"sim","kind":"`+kind+`"`)); got != name+" has the device" {
			t.Errorf("%s while held: %q", kind, got)
		}
	}
	result(t, c.call("screen.input", dev+`,"event":{"kind":"text","text":"hi"}`+req()))
	result(t, c.call("screen.give_back", dev+req()))
	if got := w.heard(); len(got) != 1 || got[0] != "a1: "+name+" gave the device back" {
		t.Errorf("the agent was told %v", got)
	}
	result(t, a.call("screen.act", `"source":"sim","kind":"tap"`))
}

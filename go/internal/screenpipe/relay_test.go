package screenpipe

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// link is a master's link to one runner, n1, played in process: calls go to the runner's Streams as they would over the wire (through
// JSON), and the runner's notices come back to the stream they name.
type link struct {
	runner *Streams
	mu     sync.Mutex
	on     map[string]func(map[string]any)
	n      int
	calls  []string
}

func wire(v map[string]any) map[string]any {
	b, _ := json.Marshal(v)
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}

func (l *link) Stream(node, method string, params map[string]any, on func(map[string]any), ended func()) (map[string]any, string, func(), error) {
	l.mu.Lock()
	l.n++
	id := "s" + string(rune('0'+l.n))
	l.on[id] = on
	l.calls = append(l.calls, node+" "+method)
	l.mu.Unlock()
	p := wire(params)
	notify := func(m string, p map[string]any) bool {
		l.mu.Lock()
		f := l.on[p["stream"].(string)]
		l.mu.Unlock()
		if f != nil && m == "runner.output" {
			f(wire(p))
		}
		return true
	}
	if _, err := l.runner.Attach(id, p["agent"].(string), p["source"].(string), notify); err != nil {
		return nil, "", nil, err
	}
	return map[string]any{}, id, func() { l.runner.Detach(id) }, nil
}

func (l *link) Call(node, method string, params map[string]any) (map[string]any, error) {
	l.mu.Lock()
	l.calls = append(l.calls, node+" "+method)
	l.mu.Unlock()
	p := wire(params)
	switch method {
	case "runner.screen.input":
		return map[string]any{}, l.runner.Input(p["stream"].(string), p["event"])
	case "runner.screen.release":
		return map[string]any{}, l.runner.Release(p["agent"].(string), p["source"].(string))
	}
	return nil, nil
}

func TestARunnersAgentIsWatchedAndDrivenThroughTheMaster(t *testing.T) {
	w := newWorld(t)
	onRunner := &fake{}
	l := &link{on: map[string]func(map[string]any){}, runner: &Streams{Open: func(agent, source string) (Source, error) {
		if agent != "a1" {
			return nil, ErrNotOpen
		}
		return onRunner, nil
	}}}
	w.hub.Stream, w.hub.Call = l.Stream, l.Call
	w.hub.Node = func(agent string) string { return "n1" }
	c := dial(t, w.founder, "founder")
	if e, _ := c.call("screen.attach", `"agent":"a2","source":"browser"`)["error"].(map[string]any); e == nil ||
		e["data"].(map[string]any)["what"] != "source" {
		t.Fatalf("a runner's agent with no browser: %v", e)
	}
	snap := result(t, c.call("screen.attach", page))
	if f, _ := snap["frame"].(map[string]any); f == nil || f["data"] != "Zmlyc3Q=" || f["height"] != float64(657) {
		t.Fatalf("the runner's first frame: %v", snap)
	}
	onRunner.send("second")
	if n := c.notice("screen_frame", 0); n["data"] != "c2Vjb25k" {
		t.Errorf("the runner's next frame: %v", n)
	}
	result(t, c.call("screen.take", page+req()))
	result(t, c.call("screen.input", page+`,"event":{"kind":"text","text":"hi"}`+req()))
	onRunner.mu.Lock()
	in := onRunner.inputs
	onRunner.mu.Unlock()
	if len(in) != 1 || in[0].Kind != "text" || in[0].Text != "hi" {
		t.Errorf("the runner's source got %+v", in)
	}
	result(t, c.call("screen.give_back", page+req()))
	result(t, c.call("screen.detach", page))
	if started, stopped, released := onRunner.counts(); started != 1 || stopped < 1 || released != 1 {
		t.Errorf("on the runner: %d starts, %d stops, %d releases", started, stopped, released)
	}
	if started, _, _ := w.src.counts(); started != 0 {
		t.Error("the master's own browser was watched for a runner's agent")
	}
	if got := strings.Join(l.calls, ","); !strings.Contains(got, "n1 runner.screen.attach") || !strings.Contains(got, "n1 runner.screen.input") {
		t.Errorf("link calls: %s", got)
	}
}

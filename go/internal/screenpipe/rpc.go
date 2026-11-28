package screenpipe

import (
	"encoding/base64"
	"errors"
	"fmt"
	"slices"

	"github.com/handline/handline/go/protocol/rpc"
)

// Register adds screen.attach, detach, input, take, give_back, act and status.
func (h *Hub) Register(s *rpc.Server) {
	s.Register("screen.attach", rpc.Method{Verbatim: true, Handler: h.attach})
	s.Register("screen.detach", rpc.Method{Handler: h.detach})
	s.Register("screen.input", rpc.Method{Unkept: true, Handler: h.input})
	s.Register("screen.take", rpc.Method{Handler: h.take})
	s.Register("screen.give_back", rpc.Method{Handler: h.giveBackCall})
	s.Register("screen.act", rpc.Method{Handler: h.act})
	s.Register("screen.status", rpc.Method{Read: true, Handler: h.status})
}

func bad(msg, field string) error { return rpc.BadRequestError(msg, field, rpc.InvalidParams) }

func noSource(k key) error {
	err := rpc.NotFoundError(k.agent + " has no " + k.source + " open")
	err.Data = map[string]any{"what": "source", "name": k.source}
	return err
}

// keyOf is the agent and source a call names; an agent's own call names only the source.
func keyOf(c *rpc.Conn, params map[string]any, own bool) (key, error) {
	source, _ := params["source"].(string)
	if !slices.Contains(Sources, source) {
		return key{}, bad(fmt.Sprintf("source is one of %v", Sources), "source")
	}
	if own {
		if c.Caller.Kind != "agent" {
			return key{}, rpc.Refused("this is an agent's own call, on its own source", "role", "")
		}
		return key{c.Caller.Key, source}, nil
	}
	agent, _ := params["agent"].(string)
	if agent == "" {
		return key{}, bad("agent is a name", "agent")
	}
	return key{agent, source}, nil
}

func callerName(c *rpc.Conn) string {
	n, _ := c.Caller.Info["name"].(string)
	return n
}

func callerKind(c *rpc.Conn) string {
	if c.Caller.Kind == "device" {
		return "device"
	}
	return "person"
}

func (h *Hub) attach(c *rpc.Conn, params map[string]any) (any, error) {
	k, err := keyOf(c, params, false)
	if err != nil {
		return nil, err
	}
	ch, err := h.join(c, k)
	if errors.Is(err, ErrNotOpen) {
		return nil, noSource(k)
	}
	if err != nil {
		return nil, rpc.Refused(k.agent+"'s "+k.source+" could not be watched: "+err.Error(), "state", "")
	}
	select {
	case <-ch.first:
	case <-timeAfter(frameWithin):
	}
	ch.mu.Lock()
	seq, last := ch.seq, ch.last
	ch.mu.Unlock()
	var frame any
	if last != nil {
		frame = map[string]any{"data": base64.StdEncoding.EncodeToString(last.JPEG), "width": last.Width, "height": last.Height}
	}
	ch.mu.Lock()
	stopped := ch.stopped
	ch.mu.Unlock()
	if stopped { // it ended while the first frame was awaited
		return nil, noSource(k)
	}
	c.AfterReply(func() {
		ch.mu.Lock()
		if w := ch.watchers[c]; w != nil {
			w.live, w.sent = true, seq // its answer carried frame seq: any newer one follows
		}
		ch.mu.Unlock()
		ch.wake()
	})
	h.mu.Lock()
	inUse := h.inUseLocked(k)
	h.mu.Unlock()
	return map[string]any{"agent": k.agent, "source": k.source, "seq": seq, "frame": frame, "holder": h.holder(k), "in_use": inUse}, nil
}

func (h *Hub) hangUp(c *rpc.Conn) {
	h.mu.Lock()
	keys := h.conns[c]
	delete(h.conns, c)
	h.mu.Unlock()
	for k := range keys {
		h.leave(c, k)
	}
}

func (h *Hub) detach(c *rpc.Conn, params map[string]any) (any, error) {
	k, err := keyOf(c, params, false)
	if err != nil {
		return nil, err
	}
	h.leave(c, k)
	return map[string]any{}, nil
}

// watching is k's channel when c watches it, nil otherwise.
func (h *Hub) watching(c *rpc.Conn, k key) *channel {
	h.mu.Lock()
	h.init()
	ch := h.chans[k]
	h.mu.Unlock()
	if ch == nil {
		return nil
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.watchers[c] == nil {
		return nil
	}
	return ch
}

func (h *Hub) input(c *rpc.Conn, params map[string]any) (any, error) {
	k, err := keyOf(c, params, false)
	if err != nil {
		return nil, err
	}
	ev, err := ParseInput(params["event"])
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.init()
	hd := h.holds[k]
	h.mu.Unlock()
	switch {
	case hd == nil:
		return nil, rpc.Refused(k.agent+" has the "+pageWord(k.source)+": take it first", "input_holder", "")
	case hd.name != callerName(c):
		return nil, rpc.Refused(hd.name+" has the "+pageWord(k.source), "input_holder", "")
	}
	ch := h.watching(c, k)
	if ch == nil {
		return nil, rpc.Refused("attach the "+k.source+" first", "state", "")
	}
	ch.mu.Lock()
	src := ch.src
	ch.mu.Unlock()
	if err := src.Input(ev); err != nil {
		return nil, rpc.Refused("the input did not reach "+k.agent+"'s "+k.source+": "+err.Error(), "state", "")
	}
	return map[string]any{"accepted": true, "said": "sent"}, nil
}

// pageWord is what a refusal calls the source.
func pageWord(source string) string {
	switch source {
	case "browser":
		return "page"
	case "sim":
		return "device"
	}
	return source
}

func (h *Hub) take(c *rpc.Conn, params map[string]any) (any, error) {
	k, err := keyOf(c, params, false)
	if err != nil {
		return nil, err
	}
	force, _ := params["force"].(bool)
	if h.watching(c, k) == nil {
		return nil, rpc.Refused("attach the "+k.source+" first", "state", "")
	}
	name := callerName(c)
	h.mu.Lock()
	hd := h.holds[k]
	switch {
	case hd != nil && hd.name == name:
	case hd != nil && !force:
		h.mu.Unlock()
		return nil, rpc.Refused(hd.name+" has the "+pageWord(k.source), "input_holder", "")
	default:
		refused := hd != nil && hd.refused
		h.holds[k] = &hold{kind: callerKind(c), name: name, since: h.now(), refused: refused}
	}
	h.mu.Unlock()
	h.holderChanged(k)
	return map[string]any{"accepted": true, "said": "you have " + k.agent + "'s " + pageWord(k.source), "holder": h.holder(k)}, nil
}

func (h *Hub) giveBackCall(c *rpc.Conn, params map[string]any) (any, error) {
	k, err := keyOf(c, params, false)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.init()
	hd := h.holds[k]
	h.mu.Unlock()
	if hd != nil && hd.name != callerName(c) {
		return nil, rpc.Refused(hd.name+" has the "+pageWord(k.source), "input_holder", "")
	}
	if err := h.giveBack(k); err != nil {
		return nil, rpc.Refused(err.Error()+"; it is still yours: try again", "state", "")
	}
	return map[string]any{"accepted": true, "said": k.agent + " has its " + pageWord(k.source) + " back", "holder": h.holder(k)}, nil
}

// Kinds is what an agent does with a source (screen.act): the acting ones are shown to watchers, the reading ones only checked.
var (
	acting = []string{"open", "click", "type", "press", "hover", "drag", "select", "scroll", "back", "forward", "reload", "tab",
		"viewport", "upload", "download", "dialog", "eval", "close", "tap", "swipe", "button", "rotate", "launch", "install"}
	reading = []string{"read", "wait", "console", "network", "shot"}
	// boxed are each source's kinds whose target has a box: the call before it is known only checks the hold, and the one with it is
	// shown.
	boxed = map[string][]string{"browser": {"click", "type", "hover", "drag", "select", "scroll", "download"}, "sim": {"tap"}}
)

// act is the agent about to use its source: refused while a person holds it, else marked in use and, when it acts, shown to watchers.
func (h *Hub) act(c *rpc.Conn, params map[string]any) (any, error) {
	k, err := keyOf(c, params, true)
	if err != nil {
		return nil, err
	}
	kind, _ := params["kind"].(string)
	if !slices.Contains(acting, kind) && !slices.Contains(reading, kind) {
		return nil, bad(fmt.Sprintf("kind is one of %v or %v", acting, reading), "kind")
	}
	box, err := boxParam(params["box"])
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.init()
	hd := h.holds[k]
	if hd != nil {
		hd.refused = true
	}
	h.mu.Unlock()
	if hd != nil {
		return nil, rpc.Refused(hd.name+" has the "+pageWord(k.source), "input_holder", "")
	}
	if kind == "close" {
		h.closed(k)
	} else {
		h.opened(k)
		h.use(k)
	}
	if slices.Contains(acting, kind) && (box != nil || !slices.Contains(boxed[k.source], kind)) {
		to, params := h.watchers(k), map[string]any{"agent": k.agent, "source": k.source, "kind": kind, "box": box,
			"at": h.now().UTC().Format("2006-01-02T15:04:05Z")}
		h.inOrder(k, func() { notify(to, "screen_action", params) })
	}
	return map[string]any{"accepted": true, "said": "go on"}, nil
}

func boxParam(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, bad("box is {x, y, width, height}", "box")
	}
	out := map[string]any{}
	for _, f := range []string{"x", "y", "width", "height"} {
		n, ok := num(m[f])
		if !ok {
			return nil, bad("box is {x, y, width, height}", "box")
		}
		out[f] = n
	}
	return out, nil
}

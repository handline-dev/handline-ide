package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// ErrHungUp is a call whose connection ended before its answer came.
var ErrHungUp = errors.New("the connection ended before the answer came")

// outCalls is the calls one end of a connection made to the other and waits on, matched to their answers by id.
type outCalls struct {
	mu     sync.Mutex
	next   int
	prefix string
	waits  map[string]chan reply
	closed bool
}

type reply struct {
	result map[string]any
	err    error
}

// call sends method with params as a request of its own id and waits up to timeout for the answer.
func (p *outCalls) call(send func(map[string]any) bool, method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrHungUp
	}
	if p.waits == nil {
		p.waits = map[string]chan reply{}
	}
	p.next++
	id := p.prefix + strconv.Itoa(p.next)
	ch := make(chan reply, 1)
	p.waits[id] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.waits, id)
		p.mu.Unlock()
	}()
	if params == nil {
		params = map[string]any{}
	}
	if !send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}) {
		return nil, ErrHungUp
	}
	select {
	case r := <-ch:
		return r.result, r.err
	case <-time.After(timeout):
		return nil, errors.New(method + ": no answer in " + timeout.String())
	}
}

// deliver hands an answer to the call waiting on its id; false when obj is no answer to a call of this end.
func (p *outCalls) deliver(obj map[string]any) bool {
	if _, isCall := obj["method"]; isCall {
		return false
	}
	id, _ := obj["id"].(string)
	p.mu.Lock()
	ch, ok := p.waits[id]
	p.mu.Unlock()
	if !ok {
		return false
	}
	r := reply{}
	if e, isErr := obj["error"].(map[string]any); isErr {
		r.err = errorFrom(e)
	} else {
		r.result, _ = obj["result"].(map[string]any)
	}
	select {
	case ch <- r:
	default: // answered twice: the first stands
	}
	return true
}

// close ends every call still waiting: no answer can come.
func (p *outCalls) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for id, ch := range p.waits {
		select {
		case ch <- reply{err: ErrHungUp}:
		default: // its answer came first
		}
		delete(p.waits, id)
	}
}

// errorFrom is the *Error a wire error object says.
func errorFrom(e map[string]any) *Error {
	out := &Error{Code: "internal", Data: map[string]any{}}
	if n, ok := e["code"].(json.Number); ok {
		v, _ := n.Int64()
		out.RPCCode = int(v)
	}
	out.Message, _ = e["message"].(string)
	if data, ok := e["data"].(map[string]any); ok {
		for k, v := range data {
			if k == "code" {
				out.Code, _ = v.(string)
				continue
			}
			out.Data[k] = v
		}
	}
	return out
}

// Call calls the other end of this connection: the daemon asks the client, which serves it (a runner's link, Link). It waits up to
// timeout. Not from a handler of this same connection, whose loop must be free to read the answer.
func (c *Conn) Call(method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	return c.calls.call(c.send, method, params, timeout)
}

// Link is the client's side of a connection whose other end also calls it: it calls the server (Call) and serves what the server
// asks (Handle), each request on its own goroutine.
type Link struct {
	tr     Transport
	Handle func(method string, params map[string]any) (any, error)
	calls  outCalls
	wmu    sync.Mutex
}

// NewLink is a link over tr; handle serves the requests the other end sends.
func NewLink(tr Transport, handle func(method string, params map[string]any) (any, error)) *Link {
	return &Link{tr: tr, Handle: handle, calls: outCalls{prefix: "l"}}
}

func (l *Link) send(obj map[string]any) bool { return sendOn(l.tr, &l.wmu, obj) }

// Call calls the other end and waits up to timeout for its answer. Run must be reading.
func (l *Link) Call(method string, params map[string]any, timeout time.Duration) (map[string]any, error) {
	return l.calls.call(l.send, method, params, timeout)
}

// Notify sends the other end a notification; false when it cannot be sent.
func (l *Link) Notify(method string, params map[string]any) bool {
	return l.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// Run reads the connection until it ends, handing answers to the calls that wait on them and serving requests; idle is how long it
// waits for the next message.
func (l *Link) Run(idle time.Duration) error {
	defer l.calls.close()
	for {
		raw, err := l.tr.ReadMessage(idle)
		if err != nil {
			return err
		}
		msg, err := decode(raw)
		obj, ok := msg.(map[string]any)
		if err != nil || !ok || l.calls.deliver(obj) {
			continue
		}
		name, _ := obj["method"].(string)
		if name == "" {
			continue // an answer to no call that waits: given up on
		}
		params, _ := obj["params"].(map[string]any)
		id, hasID := obj["id"]
		go func() {
			defer func() {
				if p := recover(); p != nil && hasID {
					l.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": (&Error{RPCCode: Internal, Code: "internal",
						Message: fmt.Sprint("internal error: ", p)}).wire()})
				}
			}()
			res, err := l.Handle(name, params)
			if !hasID {
				return
			}
			if err != nil {
				e, ok := err.(*Error)
				if !ok {
					e = &Error{RPCCode: Internal, Code: "internal", Message: err.Error()}
				}
				l.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": e.wire()})
				return
			}
			l.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": res})
		}()
	}
}

// AgentCaller is agent as a caller, the way a session the daemon proves (identify) is one.
func (s *Server) AgentCaller(agent string) Caller {
	return s.keyed(Caller{Kind: "agent", Key: agent, Role: "agent", Info: map[string]any{"kind": "agent", "key": agent, "name": agent}})
}

// Dispatch runs one call as caller, past hello, as if it came on a connection of its own whose hello named client from cwd: a call a
// runner forwards for its agent. What the call keeps on its connection ends with it.
func (s *Server) Dispatch(caller Caller, client, cwd, name string, params map[string]any) (any, error) {
	c := &Conn{srv: s, tr: noTransport{}, Caller: caller, saidHello: true, helloClient: client, cwd: cwd, done: make(chan struct{}),
		reads: make(chan struct{}, ReadsBeside)}
	defer func() {
		close(c.done)
		for _, f := range c.onClose {
			f()
		}
	}()
	if name == "hello" {
		return nil, BadRequestError("hello is not forwarded", "method", BadRequest)
	}
	if params == nil {
		params = map[string]any{}
	}
	call, err := c.route(map[string]any{"jsonrpc": "2.0", "id": "f", "method": name, "params": params})
	if err != nil {
		return nil, err
	}
	res, err := call.run()
	on := c
	if call.on != nil { // a waiting call ran on a Conn of its own
		on = call.on
	}
	for _, f := range on.afterReply {
		f()
	}
	on.afterReply = nil
	return res, err
}

// noTransport is a dispatched call's: nothing is read, and nothing it writes is sent.
type noTransport struct{}

func (noTransport) ReadMessage(time.Duration) ([]byte, error) { return nil, ErrHungUp }
func (noTransport) WriteMessage([]byte) error                 { return ErrHungUp }
func (noTransport) Pending() bool                             { return false }

// Close ends the link's connection: Run returns, and every call still waiting fails.
func (l *Link) Close() {
	if c, ok := l.tr.(interface{ Close() error }); ok {
		c.Close()
	}
}

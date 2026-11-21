package rpc

import (
	"net"
	"sync"
	"testing"
	"time"
)

// The server calls the client over the client's own connection: two calls in flight are each answered by id, the client's own call
// is served meanwhile, an error comes back typed, and a call after the hang-up fails at once.
func TestTheServerCallsTheClientOnItsOwnConnection(t *testing.T) {
	cl, sv := net.Pipe()
	srv := NewServer(fakeAuth{key: "k"}, "abc1234", 7, "local", func(string, ...any) {})
	conns := make(chan *Conn, 1)
	srv.Register("t.join", Method{Role: "owner", Handler: func(c *Conn, _ map[string]any) (any, error) {
		conns <- c
		return map[string]any{"joined": true}, nil
	}})
	done := make(chan struct{})
	go func() { srv.Serve(NewLineTransport(sv), 0); sv.Close(); close(done) }()
	release := make(chan struct{})
	l := NewLink(NewLineTransport(cl), func(method string, p map[string]any) (any, error) {
		if method == "t.slow" {
			<-release
		}
		if method == "t.fail" {
			return nil, Refused("no", "test", "")
		}
		return map[string]any{"echo": p["n"]}, nil
	})
	go l.Run(time.Minute)
	if _, err := l.Call("hello", map[string]any{"client": "t", "protocol": 1, "key": "k"}, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if r, err := l.Call("t.join", nil, 5*time.Second); err != nil || r["joined"] != true {
		t.Fatal(r, err)
	}
	c := <-conns
	var wg sync.WaitGroup
	var slow map[string]any
	wg.Add(1)
	go func() { defer wg.Done(); slow, _ = c.Call("t.slow", map[string]any{"n": "a"}, 5*time.Second) }()
	fast, err := c.Call("t.echo", map[string]any{"n": "b"}, 5*time.Second)
	close(release)
	wg.Wait()
	if err != nil || fast["echo"] != "b" || slow["echo"] != "a" {
		t.Fatalf("fast %v %v, slow %v", fast, err, slow)
	}
	if _, err := c.Call("t.fail", nil, 5*time.Second); err == nil || err.(*Error).Data["gate"] != "test" {
		t.Fatalf("the client's refusal: %v", err)
	}
	cl.Close()
	<-done
	if _, err := c.Call("t.echo", nil, time.Second); err != ErrHungUp {
		t.Fatalf("after the hang-up: %v", err)
	}
}

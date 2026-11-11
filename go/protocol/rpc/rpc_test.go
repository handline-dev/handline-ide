package rpc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/handline/handline/go/kit/storetest"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

type fakeAuth struct{ agent, key string }

func (f fakeAuth) AgentOf(pid int) string       { return f.agent }
func (f fakeAuth) FounderKeyOK(key string) bool { return f.key != "" && key == f.key }

type client struct {
	c net.Conn
	r *bufio.Reader
}

// dial serves one connection of a fresh server over an in-process pipe.
func dial(t *testing.T, a fakeAuth) *client {
	t.Helper()
	cl, sv := net.Pipe()
	srv := NewServer(a, "abc1234", 7, "local", func(string, ...any) {})
	done := make(chan struct{})
	go func() { srv.Serve(NewLineTransport(sv), 0); sv.Close(); close(done) }()
	t.Cleanup(func() { cl.Close(); <-done })
	return &client{cl, bufio.NewReader(cl)}
}

func (c *client) raw(t *testing.T, line string) map[string]any {
	t.Helper()
	c.c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.c.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
	return c.read(t)
}

func (c *client) read(t *testing.T) map[string]any {
	t.Helper()
	b, err := c.r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("no reply: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func errOf(t *testing.T, m map[string]any) (int, string) {
	t.Helper()
	e, ok := m["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error in %v", m)
	}
	return int(e["code"].(float64)), e["data"].(map[string]any)["code"].(string)
}

const helloLine = `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t/1","protocol":1}}`

func TestHelloReportsVersionSchemaRoleAndMethods(t *testing.T) {
	c := dial(t, fakeAuth{})
	r := c.raw(t, helloLine)["result"].(map[string]any)
	if r["protocol"].(float64) != 1 || r["min_client"].(float64) != 1 || r["role"] != "anonymous" || r["transport"] != "local" {
		t.Fatalf("%v", r)
	}
	if int(r["schema"].(float64)) != 7 {
		t.Fatalf("schema %v", r["schema"])
	}
	if r["daemon"] != "abc1234" {
		t.Fatal("no daemon sha")
	}
	ms := r["methods"].([]any)
	allowed := map[string]bool{}
	for _, m := range ms {
		mm := m.(map[string]any)
		allowed[mm["name"].(string)] = mm["allowed"].(bool)
	}
	if !allowed["hello"] || allowed["whoami"] {
		t.Fatalf("an anonymous caller may call hello only: %v", allowed)
	}
}

func TestRolesComeFromProofOnly(t *testing.T) {
	a := fakeAuth{key: "k"}
	c := dial(t, a)
	r := c.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)["result"].(map[string]any)
	if r["role"] != "owner" {
		t.Fatalf("%v", r)
	}
	if w := c.raw(t, `{"jsonrpc":"2.0","id":2,"method":"whoami"}`)["result"].(map[string]any); w["role"] != "owner" {
		t.Fatalf("%v", w)
	}
	c = dial(t, fakeAuth{key: "k"})
	if r := c.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"wrong"}}`)["result"].(map[string]any); r["role"] != "anonymous" {
		t.Fatalf("a wrong key is no proof: %v", r)
	}
	// an agent's session wins over a founder key it somehow held
	c = dial(t, fakeAuth{agent: "agent-x", key: "k"})
	r = c.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1,"key":"k"}}`)["result"].(map[string]any)
	if r["role"] != "agent" || r["caller"].(map[string]any)["name"] != "agent-x" {
		t.Fatalf("%v", r)
	}
}

func TestErrors(t *testing.T) {
	a := fakeAuth{}
	for _, tc := range []struct {
		name, line string
		rpc        int
		code       string
	}{
		{"not json", `{nope`, -32700, "bad_request"},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"hello"}]`, -32600, "bad_request"},
		{"not 2.0", `{"id":1,"method":"hello"}`, -32600, "bad_request"},
		{"bool id", `{"jsonrpc":"2.0","id":true,"method":"hello"}`, -32600, "bad_request"},
		{"method not a string", `{"jsonrpc":"2.0","id":1,"method":3}`, -32600, "bad_request"},
		{"params not an object", `{"jsonrpc":"2.0","id":1,"method":"hello","params":[1]}`, -32602, "bad_request"},
		{"hello first", `{"jsonrpc":"2.0","id":1,"method":"whoami"}`, -32600, "bad_request"},
		{"hello without client", `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"protocol":1}}`, -32602, "bad_request"},
		{"protocol a float", `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":1.0}}`, -32602, "bad_request"},
		{"protocol a bool", `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":true}}`, -32602, "bad_request"},
		{"protocol too old", `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"t","protocol":0}}`, -32002, "refused"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rpc, code := errOf(t, dial(t, a).raw(t, tc.line))
			if rpc != tc.rpc || code != tc.code {
				t.Fatalf("got %d %s", rpc, code)
			}
		})
	}
}

func TestErrorsAfterHello(t *testing.T) {
	c := dial(t, fakeAuth{})
	c.raw(t, helloLine)
	if rpc, code := errOf(t, c.raw(t, `{"jsonrpc":"2.0","id":2,"method":"no.such"}`)); rpc != -32601 || code != "bad_request" {
		t.Fatalf("%d %s", rpc, code)
	}
	m := c.raw(t, `{"jsonrpc":"2.0","id":3,"method":"whoami"}`)
	rpc, code := errOf(t, m)
	data := m["error"].(map[string]any)["data"].(map[string]any)
	if rpc != -32002 || code != "refused" || data["gate"] != "role" || data["reason"] == "" {
		t.Fatalf("%v", m)
	}
	if m["id"].(float64) != 3 {
		t.Fatal("the error carries the request's id")
	}
	// the connection stays usable after errors, and a string id is echoed
	if r := c.raw(t, `{"jsonrpc":"2.0","id":"abc","method":"hello","params":{"client":"t","protocol":1}}`); r["id"] != "abc" || r["result"] == nil {
		t.Fatalf("%v", r)
	}
}

func TestPolicyOfTheConnection(t *testing.T) {
	a := fakeAuth{}
	// a notification is not answered; the next call is
	c := dial(t, a)
	c.c.Write([]byte(`{"jsonrpc":"2.0","method":"hello","params":{"client":"t","protocol":1}}` + "\n"))
	if r := c.raw(t, `{"jsonrpc":"2.0","id":9,"method":"hello","params":{"client":"t","protocol":1}}`); r["id"].(float64) != 9 {
		t.Fatalf("%v", r)
	}
	// a line past the limit is bad_request and the connection ends
	big := dial(t, a)
	big.c.SetDeadline(time.Now().Add(5 * time.Second))
	go big.c.Write([]byte(strings.Repeat("x", (1<<20)+70000)))
	if rpc, code := errOf(t, big.read(t)); rpc != -32600 || code != "bad_request" {
		t.Fatalf("%d %s", rpc, code)
	}
	// a request in the old {verb} shape is not JSON-RPC
	l := dial(t, a)
	if rpc, code := errOf(t, l.raw(t, `{"verb":"wake","agent":"x"}`)); rpc != -32600 || code != "bad_request" {
		t.Fatalf("%d %s", rpc, code)
	}
}

// A client that shuts its write side after one object needs no newline.
func TestLastLineNeedsNoNewline(t *testing.T) {
	cl, sv := net.Pipe()
	go func() { cl.Write([]byte(helloLine)); cl.Close() }()
	tr := NewLineTransport(sv)
	got, err := tr.ReadMessage(time.Second)
	if err != nil || !bytes.Equal(got, []byte(helloLine)) {
		t.Fatalf("%s %v", got, err)
	}
	if _, err := tr.ReadMessage(time.Second); err != io.EOF {
		t.Fatalf("%v", err)
	}
}

func TestAWaitingCallRunsBesideTheOthersAndStopsWhenTheClientHangsUp(t *testing.T) {
	cl, sv := net.Pipe()
	srv := NewServer(fakeAuth{}, "abc1234", 7, "local", func(string, ...any) {})
	stopped := make(chan struct{})
	srv.Register("slow", Method{Role: "anonymous", Waits: true, Handler: func(c *Conn, _ map[string]any) (any, error) {
		<-c.Done()
		close(stopped)
		return nil, nil
	}})
	done := make(chan struct{})
	go func() { srv.Serve(NewLineTransport(sv), 0); sv.Close(); close(done) }()
	c := &client{cl, bufio.NewReader(cl)}
	c.raw(t, helloLine)
	cl.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := cl.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"slow","params":{}}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if r := c.raw(t, strings.Replace(helloLine, `"id":1`, `"id":3`, 1)); r["id"].(float64) != 3 || r["result"] == nil {
		t.Fatalf("the call behind a waiting one was not answered first: %v", r)
	}
	cl.Close()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the waiting call did not stop when the client hung up")
	}
	<-done
}

// readServer serves a "fast" Read and a "slow" one, which holds until the channel it takes from releases is closed, on a pipe.
func readServer(t *testing.T, releases chan chan struct{}) (*client, func()) {
	t.Helper()
	cl, sv := net.Pipe()
	srv := NewServer(fakeAuth{}, "abc1234", 7, "local", func(string, ...any) {})
	quit := make(chan struct{}) // closed first by stop, so a failed test lets the held handlers go
	srv.Register("slow", Method{Role: "anonymous", Read: true, Handler: func(*Conn, map[string]any) (any, error) {
		select {
		case r := <-releases:
			select {
			case <-r:
			case <-quit:
			}
		case <-quit:
		}
		return "slow", nil
	}})
	srv.Register("fast", Method{Role: "anonymous", Read: true, Handler: func(*Conn, map[string]any) (any, error) { return "fast", nil }})
	done := make(chan struct{})
	go func() { srv.Serve(NewLineTransport(sv), 0); sv.Close(); close(done) }()
	c := &client{cl, bufio.NewReader(cl)}
	c.raw(t, helloLine)
	cl.SetDeadline(time.Now().Add(5 * time.Second))
	return c, func() { close(quit); cl.Close(); <-done }
}

func (c *client) send(t *testing.T, id int, method string) {
	t.Helper()
	if _, err := fmt.Fprintf(c.c, `{"jsonrpc":"2.0","id":%d,"method":%q,"params":{}}`+"\n", id, method); err != nil {
		t.Fatal(err)
	}
}

func (c *client) try() (map[string]any, error) {
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(line, &m)
}

func (c *client) next(t *testing.T) map[string]any {
	t.Helper()
	m, err := c.try()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestASlowReadDoesNotHoldUpTheCallSentAfterIt(t *testing.T) {
	releases := make(chan chan struct{}, 1)
	c, stop := readServer(t, releases)
	defer stop()
	for round := range ReadsBeside + 1 { // more rounds than slots: a read that ends gives its slot back
		release := make(chan struct{})
		releases <- release
		c.send(t, 2, "slow")
		c.send(t, 3, "fast")
		if r := c.next(t); r["id"].(float64) != 3 || r["result"] != "fast" {
			t.Fatalf("round %d: the read after a slow one was not answered first: %v", round, r)
		}
		close(release)
		if r := c.next(t); r["id"].(float64) != 2 || r["result"] != "slow" {
			t.Fatalf("%v", r)
		}
	}
}

func TestOnceEveryReadSlotIsTakenTheNextReadWaitsItsTurn(t *testing.T) {
	releases := make(chan chan struct{}, ReadsBeside+1)
	release := make(chan struct{})
	c, stop := readServer(t, releases)
	defer stop()
	for i := range ReadsBeside + 1 { // the last finds no slot: it holds the loop until release
		releases <- release
		c.send(t, 10+i, "slow")
	}
	go fmt.Fprintf(c.c, `{"jsonrpc":"2.0","id":2,"method":"fast","params":{}}`+"\n") // the pipe takes it only once the loop reads again
	got := make(chan map[string]any, 1)
	go func() { m, _ := c.try(); got <- m }() // not t.Fatal: this is not the test goroutine
	select {
	case r := <-got:
		t.Fatalf("answered while every slot and the loop were held: %v", r)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	first := <-got
	if first == nil {
		t.Fatal("the waiting call was never answered")
	}
	ids := map[float64]bool{first["id"].(float64): true}
	for range ReadsBeside + 1 {
		ids[c.next(t)["id"].(float64)] = true
	}
	if len(ids) != ReadsBeside+2 || !ids[2] {
		t.Fatalf("not every call was answered once: %v", ids)
	}
}

// Both kinds of call that run beside the read loop, a Waits call and a Read, see the caller they were routed with while later hellos
// replace the connection's caller.
func TestAWaitingCallKeepsTheCallerItWasRoutedWithWhileAnotherHelloReplacesIt(t *testing.T) {
	for _, kind := range []string{"waits", "read"} {
		t.Run(kind, func(t *testing.T) {
			cl, sv := net.Pipe()
			srv := NewServer(fakeAuth{agent: "worker"}, "abc1234", 7, "local", func(string, ...any) {})
			seen := make(chan string, 1)
			srv.Register("slow", Method{Role: "agent", Waits: kind == "waits", Read: kind == "read", Handler: func(c *Conn, _ map[string]any) (any, error) {
				for i := 0; i < 200; i++ { // reads the caller while the read loop handles hellos
					_ = c.Caller.Role
					_ = c.Caller.Info["name"]
					time.Sleep(time.Millisecond)
				}
				seen <- c.Caller.Info["name"].(string)
				return nil, nil
			}})
			done := make(chan struct{})
			go func() { srv.Serve(NewLineTransport(sv), 0); sv.Close(); close(done) }()
			c := &client{cl, bufio.NewReader(cl)}
			c.raw(t, helloLine)
			cl.SetDeadline(time.Now().Add(10 * time.Second))
			if _, err := cl.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"slow","params":{}}` + "\n")); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 50; i++ {
				c.raw(t, strings.Replace(helloLine, `"id":1`, `"id":3`, 1))
			}
			if got := c.read(t); got["id"].(float64) != 2 {
				t.Fatalf("%v", got)
			}
			if name := <-seen; name != "worker" {
				t.Fatalf("caller %q", name)
			}
			cl.Close()
			<-done
		})
	}
}

func (f fakeAuth) SessionsReadable() error { return nil }

// A founder's method refused to a caller that is not an agent is logged with what its hello said, once per connection and method; an agent's
// refusal is by design, and a member's method refused is not a founder client's trouble: neither is logged.
func TestARefusedFoundersCallIsLoggedWithWhatItsHelloSaid(t *testing.T) {
	for _, c := range []struct {
		about string
		auth  fakeAuth
		hello string   // "" sends no hello
		calls []string // methods called, each refused
		want  []string // the log lines, by a part of each
	}{
		{"no key", fakeAuth{key: "k"}, `{"client":"electron","protocol":1}`, []string{"only.founder"},
			[]string{`refused only.founder: needs owner, caller is anonymous; hello from "electron" with no key`}},
		{"a wrong key", fakeAuth{key: "k"}, `{"client":"electron","protocol":1,"key":"old"}`, []string{"only.founder"},
			[]string{`hello from "electron" with a key that is not the founder's`}},
		{"no hello", fakeAuth{key: "k"}, "", []string{"only.founder"}, nil}, // bad_request: the first call is hello
		{"a retry", fakeAuth{key: "k"}, `{"client":"electron","protocol":1}`, []string{"only.founder", "only.founder", "also.founder"},
			[]string{"refused only.founder", "refused also.founder"}},
		{"an agent", fakeAuth{agent: "bob", key: "k"}, `{"client":"hl","protocol":1}`, []string{"only.founder"}, nil},
		{"a member's method", fakeAuth{key: "k"}, `{"client":"electron","protocol":1}`, []string{"only.member"}, nil},
	} {
		var logged []string
		srv := NewServer(c.auth, "abc1234", 7, "local", func(f string, a ...any) {
			if l := fmt.Sprintf(f, a...); strings.Contains(l, "refused ") {
				logged = append(logged, l)
			}
		})
		ok := func(*Conn, map[string]any) (any, error) { return map[string]any{}, nil }
		srv.Register("only.founder", Method{Role: "owner", Handler: ok})
		srv.Register("also.founder", Method{Role: "owner", Handler: ok})
		srv.Register("only.member", Method{Role: "member", Handler: ok})
		cl, sv := net.Pipe()
		done := make(chan struct{})
		go func() { srv.Serve(NewLineTransport(sv), 0); sv.Close(); close(done) }()
		conn := &client{cl, bufio.NewReader(cl)}
		if c.hello != "" {
			conn.raw(t, `{"jsonrpc":"2.0","id":1,"method":"hello","params":`+c.hello+`}`)
		}
		for i, m := range c.calls {
			reply := conn.raw(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":{}}`, i+2, m))
			if c.hello != "" {
				if _, code := errOf(t, reply); code != "refused" {
					t.Fatalf("%s: %s %v", c.about, m, reply)
				}
			}
		}
		cl.Close()
		<-done
		if len(logged) != len(c.want) {
			t.Fatalf("%s: %q", c.about, logged)
		}
		for i, w := range c.want {
			if !strings.Contains(logged[i], w) {
				t.Fatalf("%s: %q, want %q", c.about, logged[i], w)
			}
		}
	}
}

func TestAHelloNoteIsHandedToTheServerOnTheLocalSocketOnly(t *testing.T) {
	var got []string
	run := func(transport string, line string) {
		cl, sv := net.Pipe()
		srv := NewServer(fakeAuth{}, "abc1234", 7, transport, func(string, ...any) {})
		srv.Note = func(agent, kind string, argv []string, chain string) {
			got = append(got, fmt.Sprint(agent, "|", kind, "|", argv, "|", chain))
		}
		done := make(chan struct{})
		go func() { srv.Serve(NewLineTransport(sv), 0); sv.Close(); close(done) }()
		c := &client{cl, bufio.NewReader(cl)}
		c.raw(t, line)
		cl.Close()
		<-done
	}
	hello := func(note string) string {
		return `{"jsonrpc":"2.0","id":1,"method":"hello","params":{"client":"hl","protocol":1,"note":` + note + `}}`
	}
	run("local", hello(`{"agent":"a.one","kind":"verb_chain","argv":["task","done","x","y"],"chain":"zsh:1 > claude:2"}`))
	run("local", hello(`{"agent":"a/one","kind":"verb_chain"}`))
	run("local", hello(`{"agent":"a.one","kind":"anything else"}`))
	run("network", hello(`{"agent":"a.one","kind":"verb_chain"}`))
	if want := "a.one|verb_chain|[task done x]|zsh:1 > claude:2"; len(got) != 1 || got[0] != want {
		t.Fatalf("noted %q, want only %q", got, want)
	}
}

// A who param's word, and a who inside an object param, reach the handler as the key addr.Resolve gives; anything else is left as
// sent. A word that names no one goes on as sent for the app, and is refused with the names to use for hl.
func TestWhoParamsAreResolvedBeforeTheHandler(t *testing.T) {
	srv := NewServer(fakeAuth{}, "abc1234", 7, "local", func(string, ...any) {})
	db := storetest.Open(t)
	for _, q := range []string{
		"INSERT INTO workspaces(name, folder, created_at) VALUES('bob', '/x', 0)",
		"INSERT INTO agents(workspace, name, owner, created_at) VALUES(1, 'lead', 'p1', 0)",
		"UPDATE workspaces SET lead = 1",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	srv.Names = db
	c := &Conn{srv: srv, Caller: Caller{Kind: "person", Key: "p1"}}
	p := map[string]any{"agent": "bob", "minutes": json.Number("5")}
	f := map[string]any{"owner": "bob", "reporter": "human", "state": "doing"}
	v := map[string]any{"name": "bob", "filter": f}
	if c.resolveWho("load.get", p) != nil || c.resolveWho("task.view.save", v) != nil {
		t.Fatal("refused")
	}
	if p["agent"] != "a1" || p["minutes"] != json.Number("5") || f["owner"] != "a1" || f["reporter"] != "p1" || f["state"] != "doing" || v["name"] != "bob" {
		t.Fatalf("%v %v", p, v)
	}
	p = map[string]any{"agent": "nobody"}
	if err := c.resolveWho("load.get", p); err != nil || p["agent"] != "nobody" {
		t.Fatalf("the app's word that names no one: %v %v", p, err)
	}
	c.helloClient = "hl"
	if err := c.resolveWho("load.get", p); err == nil || !strings.Contains(err.Error(), "names no one you can reach: use human, or a workspace") {
		t.Fatalf("hl's word that names no one: %v", err)
	}
}

// A reply to hl is written out for its reader: a key as the name, a <@key> in text as @name, and a field named key kept. The app's
// reply keeps keys, and writing one out leaves the value the handler keeps as it was.
func TestARepliesWhosAreWrittenOutForHlOnly(t *testing.T) {
	db := storetest.Open(t)
	for _, q := range []string{
		"INSERT INTO workspaces(name, folder, created_at) VALUES('shop', '/p/shop', 0)",
		"INSERT INTO agents(workspace, name, owner, created_at) VALUES(1, 'designer', 'p1', 0)",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	srv := NewServer(fakeAuth{key: "k"}, "abc1234", 7, "local", func(string, ...any) {})
	srv.DB, srv.Names = db, db
	kept := map[string]any{"from": "a1", "key": "a1", "text": "ask <@a1>", "who": []any{"p1"}}
	srv.Register("test.read", Method{Role: "owner", Read: true, Handler: func(*Conn, map[string]any) (any, error) { return kept, nil }})
	srv.Register("test.fail", Method{Role: "owner", Read: true, Handler: func(*Conn, map[string]any) (any, error) {
		return nil, NotFoundError("no task for <@a1>")
	}})
	read := func(client, method string) map[string]any {
		c := connect(t, srv, `{"client":"`+client+`","protocol":1,"key":"k","cwd":"/p/shop/go"}`)
		return c.raw(t, `{"jsonrpc":"2.0","id":2,"method":"`+method+`","params":{}}`)
	}
	got := read("hl", "test.read")["result"].(map[string]any)
	if got["from"] != "designer" || got["key"] != "a1" || got["text"] != "ask @designer" || got["who"].([]any)[0] != "human" {
		t.Fatalf("hl reads %v", got)
	}
	if e := read("hl", "test.fail")["error"].(map[string]any); e["message"] != "no task for @designer" {
		t.Fatalf("hl's error %v", e)
	}
	if got := read("app", "test.read")["result"].(map[string]any); got["from"] != "a1" || got["text"] != "ask <@a1>" {
		t.Fatalf("the app reads %v", got)
	}
	if kept["from"] != "a1" || kept["text"] != "ask <@a1>" {
		t.Fatalf("the handler's value changed: %v", kept)
	}
}

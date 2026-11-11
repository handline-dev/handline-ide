package rpc

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/addr"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/authority"
)

const (
	Protocol     = 1
	MinClient    = 1
	MaxLineBytes = 1 << 20
	// A message that has started must finish within RecvTimeout; a connection past its hello may wait
	// between calls up to IdleTimeout. Before hello it gets the short limit.
	RecvTimeout = 30 * time.Second
	IdleTimeout = time.Hour
)

// Transport carries whole messages: a line on the local socket, a text frame on the network listener.
// ReadMessage returns the next one, or io.EOF at a clean end. A message past MaxLineBytes is a *Error.
type Transport interface {
	ReadMessage(timeout time.Duration) ([]byte, error)
	WriteMessage([]byte) error
	// Pending says whether bytes of a message already wait, so the long idle limit applies only between messages.
	Pending() bool
}

// Method is one entry of the method table. Role is the lowest role that may call it.
type Method struct {
	Role    string
	Handler func(c *Conn, params map[string]any) (any, error)
	// Waits marks a method that may hold its call for a long time: it runs beside the connection's other calls
	// and stops when the client hangs up (Conn.Done).
	Waits bool
	// Read marks a method that only reads and keeps nothing on the connection: it runs beside the connection's other calls, up to
	// ReadsBeside at once, so a slow read does not hold up the calls sent after it.
	Read bool
	// Write marks a method that changes something: it takes a request_id and runs once per id (runWrite). Role and Write of a
	// contract method come from the spec (Register).
	// OwnEvent says it writes its own event, or none, so the generic rpc_write one is not added.
	Write, OwnEvent bool
	// Unkept marks a write whose reply is never kept for a retry: one that holds a secret given once (a pairing code, a device
	// token), or one too frequent to keep (doc.push, a keystroke). The retry runs again and is judged on its own.
	Unkept bool
	// Refresh, if set, rewrites a kept reply before a retry gets it: the reply for what the write is now, not frozen at its first answer.
	// An error is the retry's answer instead.
	Refresh func(c *Conn, reply map[string]any) error
}

// Server holds what every connection shares.
type Server struct {
	Auth      authority.Authority
	Sha       string // the build's git sha, reported in hello
	Exe       string // the executable this process runs, reported in hello and daemon.status
	Schema    int
	Transport string // "local" or "network"
	Log       func(format string, args ...any)
	DB        *sql.DB // the database the write methods keep their replies in
	Net       NetGate // the paired devices of a network server; nil on the local one
	// Note, when set on the local server, records what a hello's note says: a command's verb and the programs above it.
	Note func(agent, kind string, argv []string, chain string)
	// Names, when set, is the database who words resolve in (addr.Resolve): the home's. With none, they reach the handler as sent.
	Names        *sql.DB
	capabilities []func() map[string]any
	methods      map[string]Method
	order        []string
	founder      string
}

// baseCapabilities is what hello reports under capabilities before any feature adds its own: one entry per line, in alphabetical order,
// so a port that adds one merges with the others instead of conflicting on a shared line.
func baseCapabilities() map[string]any {
	return map[string]any{
		"chat":            false,
		"review":          false,
		"terminal_format": []any{},
		"transcript":      false,
	}
}

// DeclareCapabilities adds to what hello reports under capabilities: the feature that owns a capability declares it from its own Register
// (the terminal formats). A later declaration of the same name replaces the earlier one.
func (s *Server) DeclareCapabilities(fn func() map[string]any) {
	s.capabilities = append(s.capabilities, fn)
}

func NewServer(auth authority.Authority, sha string, schema int, transport string, log func(string, ...any)) *Server {
	s := &Server{Auth: auth, Sha: sha, Schema: schema, Transport: transport, Log: log, methods: map[string]Method{}}
	s.Exe, _ = os.Executable()
	if u, err := user.Current(); err == nil {
		s.founder = u.Username
	}
	s.Register("hello", Method{Handler: hello})
	s.Register("whoami", Method{Handler: whoami})
	return s
}

// SpecMethod is one method as contract/spec defines it (methods_gen.go). Callers is its x-callers, which dispatch does not read: a
// method that needs it checks the caller itself.
type SpecMethod struct {
	Name, Role string
	Write      bool
	Since      int
	Callers    []string
	Params     []SpecParam
	Result     string // the spec's schema of the result; "" when the spec writes it inline
}

// SpecParam is one param: Type is the Go value a handler gets for it (numbers as json.Number), any for a union. Who is a param that
// names a who, which Server.Who resolves before the handler runs.
type SpecParam struct {
	Name, Type string
	Required   bool
	Who        bool
	WhoIn      []string // the properties of an object param that name a who
}

var specByName = func() map[string]SpecMethod {
	out := make(map[string]SpecMethod, len(specMethods))
	for _, m := range specMethods {
		out[m.Name] = m
	}
	return out
}()

// Spec is the contract's entry for a method.
func Spec(name string) (SpecMethod, bool) { m, ok := specByName[name]; return m, ok }

// Register adds a method by name: a method of the contract takes its Role and Write from it, so a registration gives the handler and
// the flags the spec does not hold. A Role or Write that disagrees with the spec panics. A name the spec lacks keeps the Role it gives
// (a test's method), and must give one; the conformance run fails a daemon that serves one.
func (s *Server) Register(name string, m Method) {
	if sp, ok := Spec(name); ok {
		if (m.Role != "" && m.Role != sp.Role) || (m.Write && !sp.Write) || (sp.Write && m.Read) {
			panic(fmt.Sprintf("rpc: %s is registered as role %q write %v read %v; contract/spec says role %q write %v", name, m.Role, m.Write, m.Read, sp.Role, sp.Write))
		}
		m.Role, m.Write = sp.Role, sp.Write
	} else if m.Role == "" {
		panic("rpc: " + name + " is in no contract/spec and gives no Role, so anyone could call it")
	}
	if _, dup := s.methods[name]; !dup {
		s.order = append(s.order, name)
	}
	s.methods[name] = m
}

// Has is whether the method is registered.
func (s *Server) Has(name string) bool { _, ok := s.methods[name]; return ok }

// Wrap replaces a registered method's handler with wrap's; a name not registered is left alone.
func (s *Server) Wrap(name string, wrap func(next func(*Conn, map[string]any) (any, error)) func(*Conn, map[string]any) (any, error)) {
	if m, ok := s.methods[name]; ok {
		m.Handler = wrap(m.Handler)
		s.methods[name] = m
	}
}

// Conn is one connection's state.
type Conn struct {
	srv        *Server
	tr         Transport
	Pid        int
	Caller     Caller
	saidHello  bool
	buildNoted bool // a client_build that differs from Sha has been logged on this connection
	// helloClient and helloKey are what the last hello said: its client name, and whether it carried the founder key, another key or none
	// (or came as a device). refusalNoted is the methods whose refusal was logged on this connection, once each.
	helloClient, helloKey string
	// cwd is the directory the last hello was sent from: where a person's agent names resolve (addr.Of).
	cwd          string
	refusalNoted map[string]bool
	// Remote is the peer's host on the network listener, "" elsewhere.
	Remote     string
	closeFn    func()
	device     string // the paired device this connection hello'd with
	deviceName string
	release    func()
	wmu        sync.Mutex
	done       chan struct{} // closed when the client is gone
	waiting    sync.WaitGroup
	reads      chan struct{} // a slot per read running beside the loop
	// afterReply runs once the reply of the call in hand is sent; onClose when the connection ends.
	afterReply, onClose []func()
	// callName and callRID name the write in hand while its handler runs (see Effect); calls on a plain connection run one at a time,
	// so if that ever changes they move into the call.
	callName, callRID string
}

// NetGate is what a network server knows of paired devices: a call from a connection that no device token
// opened is refused before any method runs.
type NetGate interface {
	// Know says whether token is a paired device's.
	Know(token string) bool
	// Login binds a connection to the device token belongs to; client is what its hello named, kept on the device. closeConn is what
	// revoking the device calls; release unbinds it when the connection ends.
	Login(token, client string, closeConn func()) (id, name, person string, release func(), ok bool)
	// Live says whether the device still has its token.
	Live(id string) bool
}

// Server is the server this connection belongs to.
func (c *Conn) Server() *Server { return c.srv }

// SendRaw writes one whole message, as the stream's notices are; false when the client is gone.
func (c *Conn) SendRaw(b []byte) bool {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.tr.WriteMessage(b) == nil
}

// HangUp closes the connection from the daemon's side.
func (c *Conn) HangUp() {
	if cl, ok := c.tr.(interface{ Close() error }); ok {
		cl.Close()
	}
}

// OnClose runs f when the connection ends. Call it from a method's handler.
func (c *Conn) OnClose(f func()) { c.onClose = append(c.onClose, f) }

// AfterReply runs f once the reply to the call being handled is sent: for what must not reach the client first.
// Not for a method that Waits.
func (c *Conn) AfterReply(f func()) { c.afterReply = append(c.afterReply, f) }

// Done is closed once the client has hung up: a call that waits stops then.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Serve answers calls on tr until it ends. pid is the peer process on the local socket, 0 elsewhere.
func (s *Server) Serve(tr Transport, pid int) { s.serve(tr, pid, "", nil) }

// ServeNet serves a connection of the network listener: remote is the peer's host, closeConn ends the connection.
func (s *Server) ServeNet(tr Transport, remote string, closeConn func()) {
	s.serve(tr, 0, remote, closeConn)
}

func (s *Server) serve(tr Transport, pid int, remote string, closeConn func()) {
	c := &Conn{srv: s, tr: tr, Pid: pid, Remote: remote, closeFn: closeConn, done: make(chan struct{}), reads: make(chan struct{}, ReadsBeside)}
	defer func() {
		close(c.done)
		c.waiting.Wait()
		for _, f := range c.onClose {
			f()
		}
		if c.release != nil {
			c.release()
		}
	}()
	c.Caller = identify(s.Auth, s.Transport == "local", pid, "", s.owner, s.founder)
	c.Caller = s.keyed(c.Caller)
	for {
		timeout := RecvTimeout
		if c.saidHello && !tr.Pending() {
			timeout = IdleTimeout
		}
		raw, err := tr.ReadMessage(timeout)
		if err != nil {
			if e, ok := err.(*Error); ok {
				c.send(map[string]any{"jsonrpc": "2.0", "id": nil, "error": e.wire()})
			}
			return // the rest of the stream cannot be framed, or the client is gone
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if s.Net != nil {
			switch c.admit(raw) {
			case refuse:
				if !c.send(unpairedReply(raw)) {
					return
				}
				continue
			case drop:
				c.send(unpairedReply(raw))
				return
			}
		}
		if !c.one(raw) {
			return
		}
	}
}

type admission int

const (
	let admission = iota
	refuse
	drop
)

// admit is the network gate, before the message is even parsed into a call: a connection no device token opened
// may call device.redeem, or hello with a token that is paired; a paired one stays only while its device does.
func (c *Conn) admit(raw []byte) admission {
	if c.device != "" {
		if c.srv.Net.Live(c.device) {
			return let
		}
		return drop
	}
	if msg, err := decode(raw); err == nil {
		if obj, ok := msg.(map[string]any); ok && obj["jsonrpc"] == "2.0" {
			switch obj["method"] {
			case "device.redeem":
				return let
			case "hello":
				if p, ok := obj["params"].(map[string]any); ok {
					if tok, ok := p["token"].(string); ok && c.srv.Net.Know(tok) {
						return let
					}
				}
			}
		}
	}
	return refuse
}

// UnpairedError is the one refusal an unpaired network caller gets: the same words every time, so a reply
// says nothing about why.
func UnpairedError() *Error {
	return &Error{RPCCode: RefusedCode, Code: "refused", Message: "refused", Data: map[string]any{"gate": "unpaired", "reason": "not paired"}}
}

func unpairedReply(raw []byte) map[string]any {
	var rid any
	if msg, err := decode(raw); err == nil {
		if obj, ok := msg.(map[string]any); ok {
			switch v := obj["id"].(type) {
			case json.Number, string:
				rid = v
			}
		}
	}
	return map[string]any{"jsonrpc": "2.0", "id": rid, "error": UnpairedError().wire()}
}

func (c *Conn) send(obj map[string]any) bool {
	b, err := json.Marshal(obj)
	if err != nil {
		return false
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.tr.WriteMessage(b) == nil
}

func decode(raw []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	if d.More() {
		return nil, fmt.Errorf("extra data after the first JSON value")
	}
	return v, nil
}

// one answers a message; false when the connection is done.
func (c *Conn) one(raw []byte) bool {
	msg, err := decode(raw)
	if err != nil {
		return c.send(map[string]any{"jsonrpc": "2.0", "id": nil,
			"error": BadRequestError("not JSON: "+err.Error(), "", ParseError).wire()})
	}
	obj, isObj := msg.(map[string]any)
	var rid any
	if isObj {
		switch v := obj["id"].(type) {
		case json.Number, string:
			rid = v
		}
	}
	call, err := c.route(msg, raw)
	if err != nil {
		return c.fail(rid, err)
	}
	_, hasID := obj["id"]
	answer := func() bool {
		result, err := call.run()
		if err != nil {
			return c.fail(rid, err)
		}
		sent := true // a notification is never answered
		if hasID {
			sent = c.send(map[string]any{"jsonrpc": "2.0", "id": rid, "result": result})
		}
		if !call.waits { // a waiting call runs beside the loop on a Conn of its own
			for _, f := range c.afterReply {
				f()
			}
			c.afterReply = nil
		}
		return sent
	}
	if call.waits && call.read {
		select {
		case c.reads <- struct{}{}:
		default:
			return answer() // every slot is taken: this read waits its turn on the loop
		}
	}
	if call.waits {
		c.waiting.Add(1)
		go func() {
			defer c.waiting.Done()
			answer()
			if call.read {
				<-c.reads
			}
		}()
		return true
	}
	return answer()
}

// ReadsBeside is how many Read calls of one connection run at once beside its loop.
const ReadsBeside = 8

func (c *Conn) fail(rid any, err error) bool {
	e, ok := err.(*Error)
	if !ok {
		c.srv.Log("  rpc: %v", err)
		e = &Error{RPCCode: Internal, Code: "internal", Message: "internal error"}
	}
	return c.send(map[string]any{"jsonrpc": "2.0", "id": rid, "error": e.wire()})
}

// routed is a call that passed every check before its handler: run it now, or beside the connection's other calls.
type routed struct {
	waits bool // it runs beside the loop
	read  bool // a Read, which takes one of the connection's read slots to run beside it
	run   func() (any, error)
}

func (c *Conn) route(msg any, raw []byte) (routed, error) {
	if _, batch := msg.([]any); batch {
		return routed{}, BadRequestError("batches are not accepted", "", BadRequest)
	}
	obj, ok := msg.(map[string]any)
	if !ok || obj["jsonrpc"] != "2.0" {
		return routed{}, BadRequestError("a request is a JSON-RPC 2.0 object", "", BadRequest)
	}
	if id, has := obj["id"]; has {
		switch id.(type) {
		case json.Number, string:
		default:
			return routed{}, BadRequestError("id is an integer or a string", "id", BadRequest)
		}
	}
	name, isStr := obj["method"].(string)
	if !isStr {
		return routed{}, BadRequestError("method is a string", "method", BadRequest)
	}
	params := map[string]any{}
	if p, has := obj["params"]; has {
		pm, isMap := p.(map[string]any)
		if !isMap {
			return routed{}, BadRequestError("params go by name, as an object", "params", InvalidParams)
		}
		params = pm
	}
	if name != "hello" && name != "device.redeem" && !c.saidHello {
		return routed{}, BadRequestError("the first call is hello", "method", BadRequest)
	}
	m, found := c.srv.methods[name]
	if !found {
		return routed{}, BadRequestError("no such method: "+name, "method", NoSuchMethod)
	}
	if !c.Caller.Allows(m.Role) {
		c.logRefusal(name, m.Role)
		return routed{}, Refused(fmt.Sprintf("%s is for a %s or above; this caller is %s", name, m.Role, c.Caller.Role), "role", "")
	}
	sentKey := ""
	if m.Write {
		sentKey = paramsKey(params)
	}
	if err := c.resolveWho(name, params); err != nil {
		return routed{}, err
	}
	// A call that waits runs on its own goroutine while a later hello may replace c.Caller: it gets the caller as
	// it was when routed, on a Conn of its own that shares only what stays fixed.
	beside := m.Waits || m.Read
	at := c
	if beside {
		at = &Conn{srv: c.srv, tr: c.tr, Pid: c.Pid, Remote: c.Remote, Caller: c.Caller, saidHello: c.saidHello, done: c.done,
			helloClient: c.helloClient, cwd: c.cwd}
	}
	return routed{waits: beside, read: m.Read && !m.Waits, run: func() (any, error) {
		var result any
		var err error
		if m.Write {
			result, err = at.runWrite(name, m, params, sentKey)
		} else {
			result, err = m.Handler(at, params)
		}
		if at.Typed() && at.srv.Names != nil && name != "hello" {
			result, err = at.writeOut(result, err)
		}
		if err != nil {
			return nil, err
		}
		if name == "hello" {
			c.saidHello = true
		}
		return result, nil
	}}, nil
}

// keyed is c with an agent caller named by its key: a session recorded under its old name still acts as the agent's key.
func (s *Server) keyed(c Caller) Caller {
	if c.Kind == "agent" && s.Names != nil {
		if k, err := paths.AgentKeyIn(s.Names, c.Key); err == nil && k != "" {
			c.Key = k
		}
		c.Info["name"] = c.Key
	}
	return c
}

// owner is the owner's key: p1 when there is no database to ask, or it cannot say (logged).
func (s *Server) owner() string {
	if s.DB == nil {
		return actor.OwnerKey
	}
	k, err := actor.Owner(s.DB)
	if err != nil {
		s.Log("  the owner's key could not be read, so p1 is taken: %v", err)
		return actor.OwnerKey
	}
	return k
}

// deviceCaller is the paired device id acting as its person, with that person's role. A device with no person is the owner's. A member's
// device is refused until the reads it would reach are scoped to its person (team mode).
func (s *Server) deviceCaller(id, name, person string) (Caller, error) {
	role := "owner"
	if person == "" {
		person = s.owner()
	} else {
		k, n, ok := actor.Parse(person)
		if !ok || k != actor.Person || s.DB == nil || s.DB.QueryRow("SELECT role FROM people WHERE id = ?", n).Scan(&role) != nil {
			s.Log("  device %s (%s): its person %q is no one: refused as unpaired", name, id, person)
			return Caller{}, UnpairedError()
		}
	}
	if role != "owner" {
		return Caller{}, Refused("a member's device is not served before team mode", "role", "")
	}
	return Caller{Kind: "device", Key: person, Person: person, Role: role, Info: map[string]any{"kind": "device", "device": id, "name": name}}, nil
}

// Typed is whether the caller is hl (not its hooks, which send their agent back), whose who words are typed and whose replies are read as text, by an agent or a person. A word that
// names no one is refused with the names to use, and the reply is written out for the reader (writeOut). Every other client gets
// keys, as the contract says, and writes names out itself.
func (c *Conn) Typed() bool { return c.helloClient == "hl" }

// Reader is the caller as addr resolves its words and writes text out for it.
func (c *Conn) Reader() (addr.Who, error) {
	if c.srv.Names == nil {
		return addr.Who{Key: c.Caller.Key}, nil
	}
	return addr.Of(c.srv.Names, c.Caller.Key, c.cwd)
}

// resolveWho puts the key in place of each who param's word (SpecParam.Who, and WhoIn inside an object), so a handler sees keys only.
// A word addr.Resolve refuses is refused here for a Typed caller; for another it goes on as sent, for the handler to refuse.
func (c *Conn) resolveWho(method string, params map[string]any) error {
	names := c.srv.Names
	sp, ok := Spec(method)
	if !ok || names == nil {
		return nil
	}
	r, err := c.Reader()
	if err != nil {
		return err
	}
	var failed error
	key := func(param, w string) string {
		if failed != nil || w == "" || w == c.Caller.Key { // the caller naming itself, whether or not it has a row
			return w
		}
		k, err := addr.Resolve(names, r, w)
		var refused *addr.Refusal
		switch {
		case err == nil:
			return k
		case !errors.As(err, &refused):
			failed = err
		case c.Typed():
			e := NotFoundError(refused.Msg)
			e.Data = map[string]any{"what": "who", "name": w, "field": param}
			failed = e
		}
		return w
	}
	for _, p := range sp.Params {
		if m, ok := params[p.Name].(map[string]any); ok {
			for _, k := range p.WhoIn {
				if w, ok := m[k].(string); ok {
					m[k] = key(p.Name+"."+k, w)
				}
			}
		}
		if !p.Who {
			continue
		}
		switch v := params[p.Name].(type) {
		case string:
			params[p.Name] = key(p.Name, v)
		case []any:
			for i, w := range v {
				if w, ok := w.(string); ok {
					v[i] = key(p.Name, w)
				}
			}
		}
	}
	return failed
}

// Mentions is text as it is stored for this caller: each @word that resolves for it as <@key> (addr.Mentions).
func (c *Conn) Mentions(text string) (string, error) {
	if c.srv.Names == nil {
		return text, nil
	}
	r, err := c.Reader()
	if err != nil {
		return text, err
	}
	return addr.Mentions(c.srv.Names, r, text)
}

// writeOut is a reply as a Typed caller reads it: each <@key> in its text written out as @ and the name the caller would use
// (addr.Render), and a key held by a who field (whoFields) as that name. Other values, a field named key among them, stay as they are.
func (c *Conn) writeOut(result any, err error) (any, error) {
	r, rerr := c.Reader()
	if rerr != nil {
		return result, err
	}
	names := map[string]string{} // one lookup per key in a reply
	name := func(k string) string {
		n, ok := names[k]
		if !ok {
			n = addr.Name(c.srv.Names, r, k)
			names[k] = n
		}
		return n
	}
	var walk func(v any, who bool) any
	walk = func(v any, who bool) any {
		switch v := v.(type) {
		case string:
			if _, _, ok := actor.Parse(v); ok && who {
				return name(v)
			}
			return addr.Render(c.srv.Names, r, v)
		case map[string]any:
			for k, x := range v {
				v[k] = walk(x, whoFields[k])
			}
		case []any:
			for i, x := range v {
				v[i] = walk(x, who)
			}
		}
		return v
	}
	if e := (*Error)(nil); errors.As(err, &e) {
		out := *e
		out.Message = addr.Render(c.srv.Names, r, e.Message)
		out.Data, _ = walk(generic(e.Data), false).(map[string]any)
		err = &out
	}
	return walk(generic(result), false), err
}

// whoFields are the reply fields that hold a who's key, as the contract names them.
var whoFields = map[string]bool{"acting": true, "agent": true, "agents": true, "assignee": true, "author": true, "by": true,
	"discarded_by": true, "from": true, "opened_by": true, "owner": true, "pushed_by": true, "reporter": true, "requested_by": true,
	"requester": true, "target": true, "to": true, "who": true}

// generic is v as JSON decodes it: a copy in maps and slices, so writing out a reply changes no value a handler keeps.
func generic(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out any
	if d.Decode(&out) != nil {
		return v
	}
	return out
}

// RequireSelf is for the methods that act as the caller's own session: an agent that is not the caller is refused.
func (c *Conn) RequireSelf(agent string) error {
	if c.Caller.Kind == "agent" && agent != c.Caller.Key {
		return Refused(fmt.Sprintf("%q is not this session's agent", agent), "not_self", "")
	}
	return nil
}

func (c *Conn) methodList() []map[string]any {
	out := make([]map[string]any, 0, len(c.srv.order))
	own := append([]string(nil), c.srv.order...)
	sort.Strings(own) // by name, so the list does not depend on the order the features registered in
	for _, n := range own {
		m := c.srv.methods[n]
		out = append(out, map[string]any{"name": n, "since": 1, "role": m.Role, "allowed": c.Caller.Allows(m.Role)})
	}
	return out
}

// AsInt is a JSON number that is a whole one.
func AsInt(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok || strings.ContainsAny(string(n), ".eE") {
		return 0, false
	}
	i, err := n.Int64()
	return i, err == nil
}

func hello(c *Conn, params map[string]any) (any, error) {
	if client, ok := params["client"].(string); !ok || client == "" {
		return nil, BadRequestError("hello needs client, the name/version of the caller", "client", InvalidParams)
	}
	protocol, ok := AsInt(params["protocol"])
	if !ok {
		return nil, BadRequestError("hello needs protocol, an integer", "protocol", InvalidParams)
	}
	if protocol < MinClient {
		return nil, Refused(fmt.Sprintf("this daemon serves clients of protocol %d or newer; yours speaks %d", MinClient, protocol),
			"protocol", "Open the Handline app.")
	}
	key, _ := params["key"].(string)
	c.helloClient, _ = params["client"].(string)
	c.cwd, _ = params["cwd"].(string)
	c.helloKey = "a device token" // the network listener's; a local hello says below
	if c.srv.Net != nil {
		tok, _ := params["token"].(string)
		// The earlier login goes first: Login counts this device's connections, and at its cap it would close the oldest, which can be this one.
		if c.release != nil {
			c.release()
			c.device, c.deviceName, c.release = "", "", nil
			c.Caller = anonymous()
		}
		id, name, person, release, ok := c.srv.Net.Login(tok, c.helloClient, c.closeFn)
		if !ok {
			return nil, UnpairedError()
		}
		c.device, c.deviceName, c.release = id, name, release
		caller, err := c.srv.deviceCaller(id, name, person)
		if err != nil {
			release()
			c.device, c.deviceName, c.release = "", "", nil
			return nil, err
		}
		c.Caller = caller
	} else {
		c.Caller = identify(c.srv.Auth, c.srv.Transport == "local", c.Pid, key, c.srv.owner, c.srv.founder)
		c.Caller = c.srv.keyed(c.Caller)
		switch {
		case key == "":
			c.helloKey = "no key"
		case c.Caller.Role == "owner":
			c.helloKey = "the founder key"
			c.srv.noteFounderKey(c.Pid, c.helloClient)
		default:
			c.helloKey = "a key that is not the founder's"
		}
	}
	if note, ok := params["note"].(map[string]any); ok && c.srv.Note != nil && c.srv.Transport == "local" {
		c.srv.noteVerb(note)
	}
	// Logged once the caller is known, and quoted: an unproven caller's words do not shape the log.
	if build, _ := params["client_build"].(string); build != "" && build != c.srv.Sha && !c.buildNoted && c.Caller.Role != "anonymous" {
		c.buildNoted = true
		client, _ := params["client"].(string)
		c.srv.Log("  hello: %q is build %q, this daemon is %s", client, build, c.srv.Sha)
	}
	caps, methods := baseCapabilities(), c.methodList()
	for _, declared := range c.srv.capabilities {
		for k, v := range declared() {
			caps[k] = v
		}
	}
	return map[string]any{
		"protocol": Protocol, "min_client": MinClient, "daemon": c.srv.Sha, "version": c.srv.Sha, "exe": c.srv.Exe,
		"schema": c.srv.Schema,
		"role":   c.Caller.Role, "caller": c.Caller.Info,
		"capabilities": caps,
		"transport":    c.srv.Transport,
		"methods":      methods,
	}, nil
}

func whoami(c *Conn, _ map[string]any) (any, error) {
	return map[string]any{"caller": c.Caller.Info, "role": c.Caller.Role, "methods": c.methodList()}, nil
}

// noteVerb hands a hello's note to Note: a record of a command's verb or of a refused nested session, for the agent it names.
func (s *Server) noteVerb(note map[string]any) {
	agent, _ := note["agent"].(string)
	kind, _ := note["kind"].(string)
	chain, _ := note["chain"].(string)
	if agent == "" || len(agent) > 64 || strings.ContainsAny(agent, "/\\\x00") || (kind != "verb_chain" && kind != "nested_claude_refused") {
		return
	}
	var argv []string
	list, _ := note["argv"].([]any)
	for i, a := range list {
		if i == 3 {
			break
		}
		if t, ok := a.(string); ok {
			argv = append(argv, t)
		}
	}
	if len(chain) > 400 {
		chain = chain[:400]
	}
	s.Note(agent, kind, argv, chain)
}

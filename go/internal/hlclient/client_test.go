package hlclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/contracttest"
	"github.com/handline/handline/go/protocol/pairsock"
)

// hangUp is an answer that closes the connection instead.
const hangUp = "hang up"

// daemon serves socketpairs, so no socket file is bound: each connection's lines are kept, and answer says what to
// send back for each. dial is a client's way to it.
type daemon struct {
	ln    net.Listener
	mu    sync.Mutex
	lines []map[string]any
}

type dial = func() (net.Conn, error)

func serve(t *testing.T, answer func(msg map[string]any) string) (*daemon, dial) {
	t.Helper()
	ln := pairsock.NewListener()
	d := &daemon{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					var m map[string]any
					_ = json.Unmarshal(sc.Bytes(), &m)
					d.mu.Lock()
					d.lines = append(d.lines, m)
					d.mu.Unlock()
					out := answer(m)
					if out == hangUp {
						return
					}
					if out != "" {
						conn.Write([]byte(out + "\n"))
					}
				}
			}()
		}
	}()
	return d, ln.Dial
}

func ok(m map[string]any, result string) string {
	b, _ := json.Marshal(m["id"])
	return `{"jsonrpc":"2.0","id":` + string(b) + `,"result":` + result + `}`
}

func client(dial dial) *Client {
	return &Client{Home: "/nowhere", Dial: dial}
}

func TestCallSaysHelloWithTheKeyAndAnswersTheResult(t *testing.T) {
	d, to := serve(t, func(m map[string]any) string {
		if m["method"] == "hello" {
			return ok(m, `{"protocol":1}`)
		}
		return ok(m, `{"note":"half done"}`)
	})
	c := client(to)
	c.Key = "k1"
	got, err := c.Call("progress.get", nil)
	if err != nil || got["note"] != "half done" {
		t.Fatalf("got %v, %v", got, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	hello := d.lines[0]["params"].(map[string]any)
	if d.lines[0]["method"] != "hello" || hello["client"] != "hl" || hello["key"] != "k1" || hello["protocol"] != 1.0 {
		t.Fatalf("hello: %v", d.lines[0])
	}
	if d.lines[1]["method"] != "progress.get" || d.lines[1]["id"] != 2.0 {
		t.Fatalf("call: %v", d.lines[1])
	}
}

func TestHelloCarriesNoKeyWhenNoneIsHeld(t *testing.T) {
	d, to := serve(t, func(m map[string]any) string { return ok(m, `{}`) })
	if _, err := client(to).Call("whoami", nil); err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, has := d.lines[0]["params"].(map[string]any)["key"]; has {
		t.Fatalf("hello: %v", d.lines[0])
	}
}

func TestARefusalIsTypedAndAWriteSaysIt(t *testing.T) {
	_, to := serve(t, func(m map[string]any) string {
		if m["method"] == "hello" {
			return ok(m, `{}`)
		}
		return `{"jsonrpc":"2.0","id":2,"error":{"code":-32000,"message":"no task in progress","data":{"code":"refused"}}}`
	})
	_, err := client(to).Call("task.progress", nil)
	var r *RPCError
	if !errors.As(err, &r) || r.Code != "refused" || r.Error() != "no task in progress" {
		t.Fatalf("got %v", err)
	}
	if done, said := client(to).Write("task.progress", map[string]any{"note": "x"}); done || said != "no task in progress" {
		t.Fatalf("write: %v %q", done, said)
	}
}

func TestAWriteCarriesAFreshRequestID(t *testing.T) {
	d, to := serve(t, func(m map[string]any) string { return ok(m, `{"said":"kept"}`) })
	c := client(to)
	for i := 0; i < 2; i++ {
		if done, said := c.Write("task.progress", map[string]any{"note": "x"}); !done || said != "kept" {
			t.Fatalf("write: %v %q", done, said)
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	a, b := d.lines[1]["params"].(map[string]any), d.lines[3]["params"].(map[string]any)
	if a["note"] != "x" || len(a["request_id"].(string)) != 32 || a["request_id"] == b["request_id"] {
		t.Fatalf("params %v then %v", a, b)
	}
}

func TestAHangUpBeforeTheReplyIsNotAResult(t *testing.T) {
	_, to := serve(t, func(m map[string]any) string {
		if m["method"] == "hello" {
			return ok(m, `{}`)
		}
		return hangUp
	})
	c := client(to)
	if _, err := c.Call("x", nil); err == nil || err.Error() != "the daemon closed the connection before it replied" {
		t.Fatalf("got %v", err)
	}
	done, said := c.Write("x", nil)
	if done || said != "the daemon did not answer, so it is not known whether this was done: the daemon closed the connection before it replied" {
		t.Fatalf("write: %v %q", done, said)
	}
}

// errNoDaemon is a dial that finds nothing listening.
var errNoDaemon = errors.New("connect: no such file or directory")

func TestNoDaemonIsDownAndNothingIsDone(t *testing.T) {
	c := client(func() (net.Conn, error) { return nil, errNoDaemon })
	c.Home = "/h"
	_, err := c.Call("x", nil)
	var down *DaemonDown
	if !errors.As(err, &down) || err.Error() != "the daemon is not running: open the Handline app to start it (log: /h/logs/daemon.err.log); nothing was done" {
		t.Fatalf("got %v", err)
	}
	if done, said := c.Write("x", nil); done || said != "daemon not running, nothing done: "+err.Error() {
		t.Fatalf("write %v %q", done, said)
	}
}

func TestTheRigSocketOutlivesACollectionBetweenDialers(t *testing.T) {
	ln := pairsock.NewListener()
	t.Cleanup(func() { ln.Close() })
	ctl, err := ln.Control()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HANDLINE_TEST_ROOT", t.TempDir())
	fd, err := syscall.Dup(int(ctl.Fd())) // as a child inherits it: its own descriptor, which it may close
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HANDLINE_DAEMON_FD", strconv.Itoa(fd))
	rigConn = sync.OnceValue(takeRigConn) // taken once a process; a fresh one for this test
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	for i := 0; i < 2; i++ {
		dial := Dialer("/nowhere", time.Second)
		runtime.GC()
		runtime.GC()
		c, err := dial()
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		c.Close()
	}
}

func TestALargeRequestAfterALargeHelloReplyIsAnswered(t *testing.T) {
	big := strings.Repeat("x", 32<<10)
	_, to := serve(t, func(m map[string]any) string {
		if m["method"] == "hello" {
			return ok(m, `{"pad":"`+big+`"}`)
		}
		text, _ := m["params"].(map[string]any)["text"].(string)
		return ok(m, fmt.Sprintf(`{"got":%d}`, len(text)))
	})
	c := client(to)
	c.Deadline = 3 * time.Second
	got, err := c.Call("task.memo.add", map[string]any{"text": big})
	if err != nil || got["got"] != float64(len(big)) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestEachMethodHasItsDeadline(t *testing.T) {
	for method, want := range map[string]time.Duration{
		"hello": Quick, "progress.get": Quick, "tasks.list": Quick, "task.get": Quick, "chat.search": Quick, "settings.get": Quick,
		"task.done": Quick, "task.new": Quick, "task.comment.add": Quick, "inbox.queue": Quick, "hook.record": Quick, "a.method.not.yet.known": Quick,
		"session.wake": Timeout, "session.start": Timeout, "session.restart": Timeout, "session.stop": Timeout, "session.park": Timeout,
		"agent.create": Timeout, "agent.retire": Timeout, "agent.move": Timeout, "agent.switch_now": Timeout, "agent.unstall": Timeout, "agent.force_start": Timeout,
		"workspace.create": Timeout, "workspace.delete": Timeout,
		"account.wake": Timeout, "say": Timeout, "chat.send": Timeout, "chat.send_now": Timeout,
	} {
		if got := DeadlineOf(method); got != want {
			t.Errorf("%s: %v, want %v", method, got, want)
		}
	}
}

// deadlined remembers the deadline its connection was given.
type deadlined struct {
	net.Conn
	set *atomic.Int64
}

func (c deadlined) SetDeadline(t time.Time) error {
	c.set.Store(int64(time.Until(t)))
	return c.Conn.SetDeadline(t)
}

func TestACallIsGivenItsMethodsDeadlineAndAClientsOwnWins(t *testing.T) {
	_, to := serve(t, func(m map[string]any) string { return ok(m, `{}`) })
	var set atomic.Int64
	c := client(func() (net.Conn, error) {
		conn, err := to()
		return deadlined{conn, &set}, err
	})
	for method, want := range map[string]time.Duration{"progress.get": Quick, "session.wake": Timeout} {
		if _, err := c.Call(method, nil); err != nil {
			t.Fatal(err)
		}
		if got := time.Duration(set.Load()); got > want || got < want-5*time.Second {
			t.Errorf("%s: deadline %v, want about %v", method, got, want)
		}
	}
	c.Deadline = 2 * time.Second
	if _, err := c.Call("session.wake", nil); err != nil {
		t.Fatal(err)
	}
	if got := time.Duration(set.Load()); got > 2*time.Second {
		t.Errorf("the client's own deadline: %v", got)
	}
}

func TestEverySlowMethodIsAMethodTheDaemonServes(t *testing.T) {
	var spec struct {
		Methods []struct{ Name string } `json:"methods"`
	}
	if err := json.Unmarshal(contracttest.Spec(t), &spec); err != nil {
		t.Fatal(err)
	}
	served := map[string]bool{}
	for _, m := range spec.Methods {
		served[m.Name] = true
	}
	for method := range slow {
		if !served[method] {
			t.Errorf("%s is in slow but the contract has no such method: renamed, it would fall to the %v deadline", method, Quick)
		}
	}
}

func TestAVerbChainNoteRidesOnTheNextHelloOnce(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "a.one")
	d, to := serve(t, func(m map[string]any) string { return ok(m, `{}`) })
	c := client(to)
	c.NoteVerbChain([]string{"say", strings.Repeat("x", 80), "c", "dropped"})
	for range 2 {
		if _, err := c.Call("whoami", nil); err != nil {
			t.Fatal(err)
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	note, _ := d.lines[0]["params"].(map[string]any)["note"].(map[string]any)
	argv, _ := note["argv"].([]any)
	if note["agent"] != "a.one" || note["kind"] != "verb_chain" || len(argv) != 3 || argv[1] != strings.Repeat("x", 40) {
		t.Fatalf("note: %v", note)
	}
	if _, again := d.lines[2]["params"].(map[string]any)["note"]; again {
		t.Fatalf("the second call's hello carries it again: %v", d.lines[2])
	}
}

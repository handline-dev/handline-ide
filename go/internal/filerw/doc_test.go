package filerw

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/protocol/rpc"
)

// client is one founder connection that stays open: replies by id, notices in order.
type client struct {
	t       *testing.T
	cl      net.Conn
	enc     *json.Encoder
	mu      sync.Mutex
	replies map[float64]map[string]any
	notices []map[string]any
	next    float64
}

func (r *rig) client() *client {
	r.t.Helper()
	cl, sv := net.Pipe()
	go func() { r.srv.Serve(rpc.NewLineTransport(sv), 0); sv.Close() }()
	e := &client{t: r.t, cl: cl, enc: json.NewEncoder(cl), replies: map[float64]map[string]any{}}
	go func() {
		dec := json.NewDecoder(cl)
		for {
			var m map[string]any
			if dec.Decode(&m) != nil {
				return
			}
			e.mu.Lock()
			if id, ok := m["id"].(float64); ok {
				e.replies[id] = m
			} else {
				e.notices = append(e.notices, m)
			}
			e.mu.Unlock()
		}
	}()
	r.t.Cleanup(func() { cl.Close() })
	e.call("hello", map[string]any{"client": "t", "protocol": 1, "key": "founder"})
	return e
}

func (e *client) call(method string, params map[string]any) map[string]any {
	e.t.Helper()
	e.next++
	id := e.next
	if err := e.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		e.t.Fatal(err)
	}
	var m map[string]any
	waitFor(e.t, method, func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		m = e.replies[id]
		return m != nil
	})
	return m
}

// told is the notices of method so far, as their params.
func (e *client) told(method string) []map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []map[string]any
	for _, n := range e.notices {
		if n["method"] == method {
			out = append(out, n["params"].(map[string]any))
		}
	}
	return out
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("no %s within 5s", what)
		}
	}
}

func (e *client) open(rel string) (id string, version float64, text string) {
	e.t.Helper()
	res := result(e.t, e.call("doc.open", map[string]any{"agent": "alice", "path": rel}))
	return res["doc"].(string), res["version"].(float64), res["text"].(string)
}

func (e *client) push(rel, id string, version float64, changes ...any) map[string]any {
	ups := make([]any, len(changes))
	for i, c := range changes {
		ups[i] = map[string]any{"client": "c1", "changes": c}
	}
	return e.call("doc.push", map[string]any{"agent": "alice", "path": rel, "doc": id, "version": version, "updates": ups,
		"request_id": fmt.Sprint(time.Now().UnixNano())})
}

func (e *client) pull(rel, id string, version float64) map[string]any {
	return e.call("doc.pull", map[string]any{"agent": "alice", "path": rel, "doc": id, "version": version})
}

func (e *client) close(rel string) {
	result(e.t, e.call("doc.close", map[string]any{"agent": "alice", "path": rel}))
}

func (r *rig) file(rel string) string {
	b, _ := os.ReadFile(filepath.Join(r.wt, rel))
	return string(b)
}

// cs is a ChangeSet's JSON as a call carries it; an int is a length.
func cs(parts ...any) []any {
	out := make([]any, len(parts))
	for i, p := range parts {
		switch p := p.(type) {
		case int:
			out[i] = json.Number(fmt.Sprint(p))
		case []any:
			out[i] = cs(p...)
		default:
			out[i] = p
		}
	}
	return out
}

func TestAChangeSetCountsUTF16UnitsAndAddsUpToTheText(t *testing.T) {
	text := encode("한😀b") // 1 + 2 + 1 units
	for _, c := range []struct {
		json []any
		want string
	}{
		{cs(3, []any{1, "c"}), "한😀c"},
		{cs(1, []any{2, "x"}, 1), "한xb"},
		{cs([]any{0, "a", "b"}, 4), "a\nb한😀b"},
		{cs([]any{4}), ""},
	} {
		set, err := parseChangeSet(c.json)
		if err != nil {
			t.Fatalf("%v: %v", c.json, err)
		}
		got, err := set.apply(text)
		if err != nil || decode(got) != c.want {
			t.Errorf("%v: %q %v, want %q", c.json, decode(got), err, c.want)
		}
	}
	for _, bad := range []any{cs(5), cs(3), cs(1, []any{0, "a\nb"}, 3), cs(1, []any{0, "a\rb"}, 3), cs(-1, 5), cs("x"), cs([]any{}),
		cs(json.Number("1.5"), json.Number("2.5")), cs([]any{"1"}), cs([]any{1, 2}, 3), cs(4.0), "4"} {
		set, err := parseChangeSet(bad)
		if err == nil {
			_, err = set.apply(text)
		}
		if err == nil {
			t.Errorf("%v was taken", bad)
		}
	}
	if !whole(text) || whole(text[:2]) || whole(text[2:]) {
		t.Fatal("whole is not whether every surrogate has its other half")
	}
}

func TestAPushIsTakenWholeOrNotAtAll(t *testing.T) {
	r := newRig(t)
	r.put("a.md", "한😀b")
	e := r.client()
	id, _, _ := e.open("a.md")
	for _, c := range []struct {
		changes []any
		why     string
	}{
		{[]any{cs(4, []any{0, "c"}), cs(9)}, "bad:updates"}, // the second does not fit what the first left
		{[]any{cs(2, []any{1, "x"}, 1)}, "bad:updates"},     // cuts the emoji in two
		{[]any{cs(1, json.Number("9223372036854775807"))}, "bad:updates"},
		{[]any{cs(1, []any{json.Number("9223372036854775807")})}, "bad:updates"},
		{[]any{cs(4, []any{0, strings.Repeat("x", MaxBytes)})}, "too_big"},
	} {
		if got := refusal(e.push("a.md", id, 0, c.changes...)); got != c.why {
			t.Errorf("%v: %s, want %s", c.changes, got, c.why)
		}
	}
	many := make([]any, 101)
	for i := range many {
		many[i] = cs(4)
	}
	if got := refusal(e.push("a.md", id, 0, many...)); got != "bad:updates" {
		t.Errorf("101 updates: %s", got)
	}
	for _, client := range []string{"", strings.Repeat("c", 101)} {
		if got := refusal(e.call("doc.push", map[string]any{"agent": "alice", "path": "a.md", "doc": id, "version": 0,
			"updates": []any{map[string]any{"client": client, "changes": cs(4)}}, "request_id": "r" + client})); got != "bad:updates" {
			t.Errorf("client %q: %s", client, got)
		}
	}
	if got := result(t, e.pull("a.md", id, 0)); got["version"] != 0.0 {
		t.Fatalf("a refused push was taken: %v", got)
	}
}

func TestACallOnADocMustNameItAndHaveItOpen(t *testing.T) {
	r := newRig(t)
	r.put("a.md", "x")
	e, other := r.client(), r.client()
	id, _, _ := e.open("a.md")
	for _, c := range []struct {
		got, want string
	}{
		{refusal(other.push("a.md", id, 0, cs(1))), "not_found"},
		{refusal(other.pull("a.md", id, 0)), "not_found"},
		{refusal(e.pull("a.md", id, 1)), "bad:version"},
		{refusal(e.pull("a.md", "", 0)), "bad:doc"},
		{refusal(e.pull("a.md", "another", 0)), "reopen"},
		{refusal(e.push("a.md", "another", 0, cs(1))), "reopen"},
		{refusal(e.pull("a.txt", id, 0)), "bad:path"},
		{refusal(e.push("../a.md", id, 0, cs(1))), "bad:path"},
		{refusal(e.call("doc.close", map[string]any{"agent": "alice", "path": "a.txt"})), "bad:path"},
	} {
		if c.got != c.want {
			t.Errorf("%s, want %s", c.got, c.want)
		}
	}
	other.close("a.md") // not open there: not an error
}

func TestEveryEditorIsToldEachUpdateInVersionOrder(t *testing.T) {
	r := newRig(t)
	r.put("a.md", "x")
	a, b := r.client(), r.client()
	id, _, _ := a.open("a.md")
	a.open("a.md") // twice on one connection: told once
	b.open("a.md")
	result(t, a.push("a.md", id, 0, cs(1, []any{0, "y"}), cs(2, []any{0, "z"})))
	result(t, b.push("a.md", id, 2, cs([]any{1, "w"}, 2)))
	for _, e := range []*client{a, b} {
		waitFor(t, "two notices", func() bool { return len(e.told("doc_changed")) == 2 })
		got := e.told("doc_changed")
		if got[0]["version"] != 2.0 || got[1]["version"] != 3.0 {
			t.Fatalf("told %v", got)
		}
		ups := got[0]["updates"].([]any)
		first := ups[0].(map[string]any)
		if len(ups) != 2 || first["client"] != "c1" || first["by"] != "human" || fmt.Sprint(first["changes"]) != "[1 [0 y]]" {
			t.Fatalf("updates %v", ups)
		}
	}
	if _, _, text := r.client().open("a.md"); text != "wyz" {
		t.Fatalf("text %q", text)
	}
}

func TestTheFileIsWrittenOnceThePushesPauseAndAtTheLastClose(t *testing.T) {
	r := newRig(t)
	r.env.WriteDelay = time.Second
	r.put("a.md", "x")
	e := r.client()
	id, _, _ := e.open("a.md")
	result(t, e.push("a.md", id, 0, cs(1, []any{0, "y"})))
	time.Sleep(600 * time.Millisecond)
	result(t, e.push("a.md", id, 1, cs(2, []any{0, "z"})))
	time.Sleep(600 * time.Millisecond) // past the first push's delay, not the second's
	if r.file("a.md") != "x" {
		t.Fatal("written before the pushes paused")
	}
	waitFor(t, "the write once the pushes paused", func() bool { return r.file("a.md") == "xyz" })

	other := r.client()
	other.open("a.md")
	result(t, e.push("a.md", id, 2, cs(3, []any{0, "!"})))
	e.close("a.md")
	if r.file("a.md") != "xyz" {
		t.Fatal("written while another editor still has the doc")
	}
	other.cl.Close() // a connection that drops closes its docs: the last one writes at once
	waitFor(t, "the write at the last close", func() bool { return r.file("a.md") == "xyz!" })
	if id2, v, text := r.client().open("a.md"); id2 == id || v != 0 || text != "xyz!" {
		t.Fatalf("opened again: %s %v %q", id2, v, text)
	}
}

func TestPullServesOnlyTheKeptUpdates(t *testing.T) {
	r := newRig(t)
	r.env.KeepUpdates = 2
	r.put("a.md", "")
	e := r.client()
	id, _, _ := e.open("a.md")
	for v := 0; v < 3; v++ {
		result(t, e.push("a.md", id, float64(v), cs([]any{0, "a"}, v)))
	}
	if got := refusal(e.pull("a.md", id, 0)); got != "reopen" {
		t.Fatalf("a version older than the kept updates: %s", got)
	}
	if ups := result(t, e.pull("a.md", id, 1))["updates"].([]any); len(ups) != 2 {
		t.Fatalf("from 1: %v", ups)
	}
}

func TestADocWithNothingTakenNeverWritesTheFile(t *testing.T) {
	r := newRig(t)
	r.put("a.md", "x")
	e := r.client()
	e.open("a.md")
	r.put("a.md", "an agent's edit")
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(r.wt, "a.md"), old, old)
	e.close("a.md")
	fi, _ := os.Stat(filepath.Join(r.wt, "a.md"))
	if r.file("a.md") != "an agent's edit" || !fi.ModTime().Equal(old) {
		t.Fatalf("the close wrote the file: %q at %v", r.file("a.md"), fi.ModTime())
	}
}

func TestAnOutsideChangeIsTakenInBeforeTheWriteNeverWrittenOver(t *testing.T) {
	r := newRig(t)
	r.env.WriteDelay = time.Hour
	r.put("a.md", "one\ntwo\nthree\n")
	e, watcher := r.client(), r.client()
	id, _, _ := e.open("a.md")
	watcher.open("a.md")
	result(t, e.push("a.md", id, 0, cs([]any{0, "1 "}, 14)))  // the founder types at the start of line one
	r.put("a.md", "one\ntwo\nTHREE\nfour\n")                  // meanwhile an agent's Edit changes line three and adds one
	result(t, e.push("a.md", id, 1, cs(9, []any{0, "+"}, 7))) // and the founder types at the end of line two
	r.env.docs.mu.Lock()
	d := r.env.docs.all[docKey("alice", "a.md")]
	r.env.docs.mu.Unlock()
	r.env.flushLater(d) // the write once the pushes paused
	if got := r.file("a.md"); got != "1 one\ntwo+\nTHREE\nfour\n" {
		t.Fatalf("file %q", got)
	}
	var got []map[string]any
	waitFor(t, "the disk update", func() bool { got = watcher.told("doc_changed"); return len(got) == 3 })
	disk := got[2]["updates"].([]any)[0].(map[string]any)
	if disk["client"] != "disk" || disk["by"] != nil || got[2]["version"] != 3.0 {
		t.Fatalf("disk update %v", got[2])
	}
	if id2, _, text := watcher.open("a.md"); id2 != id || text != "1 one\ntwo+\nTHREE\nfour\n" {
		t.Fatalf("the doc did not take the change in: %s %q", id2, text)
	}
}

func TestAFileWithCRLFIsHeldWithLFAndWrittenBackWithCRLF(t *testing.T) {
	r := newRig(t)
	r.put("a.md", "a\r\nb\r\n")
	r.put("b.md", "a\rb")
	e := r.client()
	id, _, text := e.open("a.md")
	if text != "a\nb\n" {
		t.Fatalf("text %q", text)
	}
	result(t, e.push("a.md", id, 0, cs(2, []any{0, "x", ""}, 2)))
	e.close("a.md")
	if got := r.file("a.md"); got != "a\r\nx\r\nb\r\n" {
		t.Fatalf("file %q", got)
	}
	if got := refusal(e.call("doc.open", map[string]any{"agent": "alice", "path": "b.md"})); got != "bad:path" {
		t.Fatalf("a lone \\r: %s", got)
	}
}

func TestAWriteThatFailsIsTriedAgain(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a folder whatever its mode")
	}
	r := newRig(t)
	r.env.WriteDelay, r.env.RetryDelay = 10*time.Millisecond, 50*time.Millisecond
	r.put("d/a.md", "x")
	e := r.client()
	id, _, _ := e.open("d/a.md")
	dir := filepath.Join(r.wt, "d")
	os.Chmod(dir, 0o555) // the file cannot be replaced in a folder that cannot be written
	defer os.Chmod(dir, 0o755)
	result(t, e.push("d/a.md", id, 0, cs(1, []any{0, "y"})))
	time.Sleep(100 * time.Millisecond) // the write after the delay has failed by now
	if r.file("d/a.md") != "x" {
		t.Fatal("written into a folder that cannot be written")
	}
	os.Chmod(dir, 0o755)
	waitFor(t, "the write tried again", func() bool { return r.file("d/a.md") == "xy" })
}

func TestAFileGoneOrUnreadableAtAWriteLetsTheDocGo(t *testing.T) {
	r := newRig(t)
	r.env.WriteDelay = 10 * time.Millisecond
	for rel, spoil := range map[string]func(string){
		"gone.md":   func(full string) { os.Remove(full) },
		"binary.md": func(full string) { os.WriteFile(full, []byte{0, 1, 2}, 0o644) },
	} {
		r.put(rel, "x")
		e := r.client()
		id, _, _ := e.open(rel)
		spoil(filepath.Join(r.wt, rel))
		result(t, e.push(rel, id, 0, cs(1, []any{0, "y"})))
		waitFor(t, "doc_closed", func() bool { return len(e.told("doc_closed")) == 1 })
		want := map[string]string{"gone.md": "gone", "binary.md": "unreadable"}[rel]
		if got := e.told("doc_closed")[0]; got["reason"] != want || got["doc"] != id {
			t.Errorf("%s: %v", rel, got)
		}
		if got := refusal(e.pull(rel, id, 0)); got != "not_found" {
			t.Errorf("%s: a let go doc still answers: %s", rel, got)
		}
		if _, err := os.Stat(filepath.Join(r.wt, rel)); rel == "gone.md" && err == nil {
			t.Error("a deleted file was written again")
		}
	}
}

func TestADocOpensOnlyWhereFileWriteMayWrite(t *testing.T) {
	r := newRig(t)
	r.put(".claude/notes.md", "x")
	r.put("a.txt", "x")
	e := r.client()
	for rel, why := range map[string]string{".claude/notes.md": "denied", "a.txt": "bad:path", "none.md": "not_found"} {
		if got := refusal(e.call("doc.open", map[string]any{"agent": "alice", "path": rel})); got != why {
			t.Errorf("%s: %s, want %s", rel, got, why)
		}
	}
}

func TestADocWhoseLastWriteFailsStaysLoadedUntilItIsMade(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes into a folder whatever its mode")
	}
	r := newRig(t)
	r.env.WriteDelay, r.env.RetryDelay = time.Hour, time.Hour
	r.put("d/a.md", "x")
	e := r.client()
	id, _, _ := e.open("d/a.md")
	result(t, e.push("d/a.md", id, 0, cs(1, []any{0, "y"})))
	dir := filepath.Join(r.wt, "d")
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)
	e.close("d/a.md")
	if id2, v, text := e.open("d/a.md"); id2 != id || v != 1 || text != "xy" {
		t.Fatalf("opened again after a failed write: %s %v %q", id2, v, text)
	}
	e.close("d/a.md")
	os.Chmod(dir, 0o755)
	r.env.docs.mu.Lock()
	d := r.env.docs.all[docKey("alice", "d/a.md")]
	r.env.docs.mu.Unlock()
	r.env.flushLater(d) // the retry
	if r.file("d/a.md") != "xy" {
		t.Fatalf("file %q", r.file("d/a.md"))
	}
	r.env.docs.mu.Lock()
	defer r.env.docs.mu.Unlock()
	if r.env.docs.all[docKey("alice", "d/a.md")] != nil {
		t.Fatal("still loaded once written")
	}
}

func TestTheSameChangeOnBothSidesIsTakenOnce(t *testing.T) {
	r := newRig(t)
	r.env.WriteDelay = time.Hour
	r.put("a.md", "a\n")
	e := r.client()
	id, _, _ := e.open("a.md")
	result(t, e.push("a.md", id, 0, cs(2, []any{0, "b", ""})))
	r.put("a.md", "a\nb\n") // another view saved the same text
	e.close("a.md")
	if got := r.file("a.md"); got != "a\nb\n" {
		t.Fatalf("file %q", got)
	}
}

func TestAFileOverTheCapDoesNotOpenAndCRLFCountsToIt(t *testing.T) {
	r := newRig(t)
	r.put("big.md", strings.Repeat("x", MaxBytes+1))
	k := (MaxBytes - 1) / 3
	r.put("crlf.md", strings.Repeat("x\r\n", k)) // one byte under the cap
	e := r.client()
	if got := refusal(e.call("doc.open", map[string]any{"agent": "alice", "path": "big.md"})); got != "too_big" {
		t.Fatalf("over the cap: %s", got)
	}
	id, _, _ := e.open("crlf.md")
	if got := refusal(e.push("crlf.md", id, 0, cs([]any{0, "x", ""}, 2*k))); got != "too_big" { // fits with \n, not with \r\n
		t.Fatalf("over the cap with CRLF: %s", got)
	}
}

func TestAnEditorThatReadsNothingIsHungUp(t *testing.T) {
	defer func(n int) { outboxSize = n }(outboxSize)
	outboxSize = 2
	r := newRig(t)
	r.put("a.md", "")
	e := r.client()
	id, _, _ := e.open("a.md")
	cl, sv := net.Pipe()
	go func() { r.srv.Serve(rpc.NewLineTransport(sv), 0); sv.Close() }()
	defer cl.Close()
	enc, dec := json.NewEncoder(cl), json.NewDecoder(cl)
	for i, req := range []map[string]any{{"method": "hello", "params": map[string]any{"client": "t", "protocol": 1, "key": "founder"}},
		{"method": "doc.open", "params": map[string]any{"agent": "alice", "path": "a.md"}}} {
		req["jsonrpc"], req["id"] = "2.0", i+1
		enc.Encode(req)
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
	}
	for v := 0; v < 6; v++ { // the slow editor reads none of these notices
		result(t, e.push("a.md", id, float64(v), cs([]any{0, "a"}, v)))
	}
	cl.SetReadDeadline(time.Now().Add(5 * time.Second))
	var err error
	for err == nil {
		var m map[string]any
		err = dec.Decode(&m)
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("the editor whose notices piled up was not hung up")
	}
}

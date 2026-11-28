package chatrelay

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/handline/handline/go/agentctl/holder"
	"github.com/handline/handline/go/kit/paths"
)

// fakeRunners is one runner, n1, with bob's session and his holder: each stream it opens is given the records past its from, as a
// holder that kept them gives them; write and close are kept.
type fakeRunners struct {
	mu      sync.Mutex
	linked  bool
	live    bool
	gone    bool  // bob's session left the runner
	pid     int64 // the holder's process
	recs    []holder.Record
	froms   []int64
	acks    []int64
	writes  []string
	closed  int
	streams map[string]func(map[string]any)
	ended   map[string]func()
	n       int
}

func (f *fakeRunners) Linked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.linked {
		return nil
	}
	return []string{"n1"}
}

func (f *fakeRunners) SessionsOn(node string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		return map[string]string{}, nil
	}
	return map[string]string{"bob": "a7"}, nil
}

func (f *fakeRunners) Quick(node, method string, p map[string]any) (map[string]any, error) {
	return f.Call(node, method, p)
}

func (f *fakeRunners) Stream(node, method string, p map[string]any, on func(map[string]any), ended func()) (map[string]any, string, func(), error) {
	f.mu.Lock()
	if !f.linked {
		f.mu.Unlock()
		return nil, "", nil, errors.New("not linked")
	}
	f.n++
	id := "s" + string(rune('0'+f.n))
	from, _ := p["from"].(int64)
	f.froms = append(f.froms, from)
	f.streams[id], f.ended[id] = on, ended
	pid := f.pid
	var out []holder.Record
	for _, r := range f.recs {
		if r.Seq > from-1 { // one record more than asked: a holder whose ack did not come gives it again
			out = append(out, r)
		}
	}
	f.mu.Unlock()
	for _, r := range out {
		b, _ := json.Marshal(r)
		on(map[string]any{"stream": id, "method": "record", "record": string(b)})
	}
	return map[string]any{"pid": pid}, id, func() {}, nil
}

// drop ends the link: each stream's ended runs.
func (f *fakeRunners) drop() {
	f.mu.Lock()
	f.linked = false
	ends := f.ended
	f.ended = map[string]func(){}
	f.mu.Unlock()
	for _, e := range ends {
		e()
	}
}

func (f *fakeRunners) Call(node, method string, p map[string]any) (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.linked {
		return nil, errors.New("not linked")
	}
	switch method {
	case "runner.chat.list":
		if !f.live {
			return map[string]any{"holders": []any{}}, nil
		}
		return map[string]any{"holders": []any{filepath.Base(paths.ChatSocket("", "bob"))}}, nil
	case "runner.chat.ack":
		n, _ := p["seq"].(int64)
		f.acks = append(f.acks, n)
	case "runner.chat.write":
		f.writes = append(f.writes, p["line"].(string))
	case "runner.chat.close":
		f.closed++
	}
	return map[string]any{}, nil
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal(what)
		}
	}
}

// A runner's holder is served at the agent's own socket while the runner lists it: what the CLI wrote is read there in order, acks,
// writes and the end of input go to the runner, a link that ends leaves the holder up and its reader waiting, and once the link is back
// the reader goes on from the last record it was given, with none given twice. A holder the runner no longer lists goes.
func TestARunnersHolderIsServedHereAndReadOnAcrossALinkThatEnds(t *testing.T) {
	home, err := os.MkdirTemp("", "cr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	f := &fakeRunners{linked: true, live: true, streams: map[string]func(map[string]any){}, ended: map[string]func(){}}
	for i := int64(1); i <= 3; i++ {
		f.recs = append(f.recs, holder.Record{Seq: i, Line: json.RawMessage(`{"n":` + string(rune('0'+i)) + `}`)})
	}
	m := &Relays{Home: home, Runners: f, Log: t.Logf, Retry: 20 * time.Millisecond}
	m.Sync()
	sock := paths.ChatSocket(home, "a7")
	if !holder.Alive(sock) {
		t.Fatal("bob's holder is not served here")
	}
	rd, err := holder.Open(context.Background(), sock, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	var got []int64
	next := func() {
		t.Helper()
		r, err := rd.Next()
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, r.Seq)
	}
	for range 3 {
		next()
	}
	if err := rd.Ack(3); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the ack did not reach the runner", func() bool { f.mu.Lock(); defer f.mu.Unlock(); return slices.Contains(f.acks, 3) })
	if err := holder.Write(sock, []byte(`{"type":"user"}`)); err != nil {
		t.Fatal(err)
	}
	if err := holder.CloseInput(sock); err != nil {
		t.Fatal(err)
	}

	f.drop()
	if holder.Write(sock, []byte(`{"x":1}`)) == nil {
		t.Fatal("a write was taken while the link was down")
	}
	m.Sync()
	if !holder.Alive(sock) {
		t.Fatal("the holder went with the link")
	}
	f.mu.Lock()
	f.recs = append(f.recs, holder.Record{Seq: 4, Line: json.RawMessage(`{"n":4}`)})
	f.linked = true
	f.mu.Unlock()
	next()
	if !slices.Equal(got, []int64{1, 2, 3, 4}) {
		t.Fatalf("records read %v", got)
	}
	f.mu.Lock()
	froms, writes, closed := slices.Clone(f.froms), slices.Clone(f.writes), f.closed
	f.mu.Unlock()
	if len(froms) < 2 || froms[0] != 0 || froms[len(froms)-1] != 3 || !slices.Equal(writes, []string{`{"type":"user"}`}) || closed != 1 {
		t.Fatalf("streams from %v, writes %v, closed %d", froms, writes, closed)
	}

	f.mu.Lock()
	f.live = false
	f.mu.Unlock()
	m.Sync()
	if holder.Alive(sock) {
		t.Fatal("a holder the runner no longer lists is still served")
	}
	if _, err := rd.Next(); err == nil {
		t.Fatal("the reader of a holder that went reads on")
	}
}

func relayed(t *testing.T) (*fakeRunners, *Relays, string) {
	t.Helper()
	home, err := os.MkdirTemp("", "cr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	f := &fakeRunners{linked: true, live: true, pid: 1, streams: map[string]func(map[string]any){}, ended: map[string]func(){},
		recs: []holder.Record{{Seq: 1, Line: json.RawMessage(`{"n":1}`)}, {Seq: 2, Holder: "exit"}}}
	m := &Relays{Home: home, Runners: f, Log: t.Logf, Retry: 20 * time.Millisecond}
	m.Sync()
	sock := paths.ChatSocket(home, "a7")
	if !holder.Alive(sock) {
		t.Fatal("bob's holder is not served here")
	}
	return f, m, sock
}

// The reader's last ack, the one for its holder's exit, reaches the runner though the reader goes at once: the holder there can end.
func TestTheAckOfAnExitReachesTheRunner(t *testing.T) {
	f, _, sock := relayed(t)
	rd, err := holder.Open(context.Background(), sock, 0)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := rd.Next(); err != nil {
			t.Fatal(err)
		}
	}
	rd.Ack(2)
	rd.Close()
	eventually(t, "the exit's ack did not reach the runner", func() bool { f.mu.Lock(); defer f.mu.Unlock(); return slices.Contains(f.acks, 2) })
}

// A relay goes once its agent's session has left its runner, though that runner is not linked; and a new holder at the same name,
// whose records start over, ends the reader of the old one.
func TestARelayGoesWithItsSessionAndAHolderStartedAgainEndsItsReader(t *testing.T) {
	f, m, sock := relayed(t)
	rd, err := holder.Open(context.Background(), sock, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	if _, err := rd.Next(); err != nil {
		t.Fatal(err)
	}
	f.drop()
	f.mu.Lock()
	f.pid, f.linked = 2, true
	f.mu.Unlock()
	if _, err := rd.Next(); err != nil { // the exit, still queued
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := rd.Next(); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the old holder's reader read the new one's records")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the old holder's reader was not ended")
	}

	f.drop()
	f.mu.Lock()
	f.gone = true
	f.mu.Unlock()
	m.Sync()
	if holder.Alive(sock) {
		t.Fatal("the relay of a session that left its runner is kept while the runner is away")
	}
}

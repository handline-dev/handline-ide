package daemon

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/handline/handline/go/scheduler/pause"
)

// reading reads f in the background, so the daemon never waits on the pipe for a reply or an event to be read.
type reading struct {
	f       *pipeConn
	replies map[float64]chan map[string]any
	status  chan map[string]any
	devices chan map[string]any // devices topic events, whole
}

func readAll(t *testing.T, f *pipeConn, ids ...float64) *reading {
	r := &reading{f: f, replies: map[float64]chan map[string]any{}, status: make(chan map[string]any, 16), devices: make(chan map[string]any, 16)}
	for _, id := range ids {
		r.replies[id] = make(chan map[string]any, 1)
	}
	go func() {
		for {
			b, err := f.r.ReadBytes('\n')
			if err != nil {
				return
			}
			var m map[string]any
			json.Unmarshal(b, &m)
			if id, ok := m["id"].(float64); ok && r.replies[id] != nil {
				r.replies[id] <- m
			} else if p, _ := m["params"].(map[string]any); m["method"] == "event" && p["type"] == "status.changed" {
				r.status <- p["data"].(map[string]any)
			} else if m["method"] == "event" && p["topic"] == "devices" {
				r.devices <- p
			}
		}
	}()
	return r
}

func (r *reading) ask(id float64, line string) map[string]any {
	r.f.say(line)
	select {
	case m := <-r.replies[id]:
		return m
	case <-time.After(5 * time.Second):
		r.f.t.Fatalf("no reply to %v", id)
		return nil
	}
}

// statusEvent is the next status.changed told, within wait.
func (r *reading) statusEvent(wait time.Duration) map[string]any {
	select {
	case m := <-r.status:
		return m
	case <-time.After(wait):
		r.f.t.Fatal("no status.changed")
		return nil
	}
}

// The founder's pause is in daemon.status and is told to the status topic when it starts, when it is resumed, and when its until comes.
func TestAPauseIsToldOnTheStatusTopicAndEndsByItselfAtItsUntil(t *testing.T) {
	d := startStream(t)
	f := readAll(t, pipeTo(t, d, "k"), 2, 3, 4, 5, 6, 7, 8, 9, 10)
	if r := f.ask(2, `{"jsonrpc":"2.0","id":2,"method":"daemon.status","params":{}}`)["result"].(map[string]any); r["paused"] != nil {
		t.Fatalf("paused by default: %v", r["paused"])
	}
	if r := f.ask(3, `{"jsonrpc":"2.0","id":3,"method":"subscribe","params":{"topics":["status"]}}`); r["result"] == nil {
		t.Fatalf("%v", r)
	}
	if r := f.ask(10, `{"jsonrpc":"2.0","id":10,"method":"daemon.resume","params":{"request_id":"r0"}}`)["result"].(map[string]any); r["said"] != "New work was not paused." {
		t.Fatalf("resume when not paused: %v", r)
	}
	select {
	case m := <-f.status:
		t.Fatalf("a resume that changed nothing was told: %v", m)
	case <-time.After(200 * time.Millisecond):
	}
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	if _, code := errOf(t, f.ask(4, `{"jsonrpc":"2.0","id":4,"method":"daemon.pause","params":{"until":"`+past+`","request_id":"p0"}}`)); code != "bad_request" {
		t.Fatalf("a past until: %s", code)
	}
	soon := time.Now().Truncate(time.Second).Add(500 * time.Millisecond).UTC().Format(time.RFC3339Nano) // within this second: over once kept
	if _, code := errOf(t, f.ask(9, `{"jsonrpc":"2.0","id":9,"method":"daemon.pause","params":{"until":"`+soon+`","request_id":"p9"}}`)); code != "bad_request" {
		t.Fatalf("an until within this second: %s", code)
	}
	f.ask(5, `{"jsonrpc":"2.0","id":5,"method":"daemon.pause","params":{"request_id":"p1"}}`)
	if p, _ := f.statusEvent(5 * time.Second)["paused"].(map[string]any); p == nil || p["since"] == nil || p["until"] != nil {
		t.Fatalf("paused %v", p)
	}
	r := f.ask(6, `{"jsonrpc":"2.0","id":6,"method":"daemon.status","params":{}}`)["result"].(map[string]any)
	m, _ := r["machine"].(map[string]any)
	if r["paused"] == nil || m["id"] == "" || m["id"] == nil || m["name"] == nil || !reflect.DeepEqual(r["paused"].(map[string]any)["machine"], m) {
		t.Fatalf("daemon.status does not say this machine is paused: %v", r)
	}
	f.ask(7, `{"jsonrpc":"2.0","id":7,"method":"daemon.resume","params":{"request_id":"r1"}}`)
	if p := f.statusEvent(5 * time.Second)["paused"]; p != nil {
		t.Fatalf("still paused after resume: %v", p)
	}

	until := time.Now().Add(2 * time.Second).Truncate(time.Second).Add(time.Second)
	f.ask(8, fmt.Sprintf(`{"jsonrpc":"2.0","id":8,"method":"daemon.pause","params":{"until":%q,"request_id":"p2"}}`, until.UTC().Format(time.RFC3339)))
	if p, _ := f.statusEvent(5 * time.Second)["paused"].(map[string]any); p == nil || p["until"] != until.UTC().Format("2006-01-02T15:04:05Z") {
		t.Fatalf("paused %v", p)
	}
	if p := f.statusEvent(5 * time.Second)["paused"]; p != nil || time.Now().Before(until) {
		t.Fatalf("the until came and went: %v at %v", p, time.Now())
	}
}

// A pause kept across a restart still ends at its until: the new daemon tells the status topic then.
func TestAPauseKeptAcrossARestartStillEndsAtItsUntil(t *testing.T) {
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth = pidAuth{}
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(2 * time.Second).Truncate(time.Second).Add(time.Second)
	if _, err := pause.Set(d.db, time.Now(), until.Unix()); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if d, err = Start(cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	f := readAll(t, pipeTo(t, d, "k"), 2, 3)
	if r := f.ask(2, `{"jsonrpc":"2.0","id":2,"method":"daemon.status","params":{}}`)["result"].(map[string]any); r["paused"] == nil {
		t.Fatal("the restart forgot the pause")
	}
	f.ask(3, `{"jsonrpc":"2.0","id":3,"method":"subscribe","params":{"topics":["status"]}}`)
	if p := f.statusEvent(5 * time.Second)["paused"]; p != nil || time.Now().Before(until) {
		t.Fatalf("the until came and went: %v at %v", p, time.Now())
	}
}

package watcher

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/internal/reads"
)

// agentsOn is the agents topic over d, its rows read as agents.list reads them, in the home d's file is in.
func agentsOn(t *testing.T, d *sql.DB) *Agents {
	t.Helper()
	var seq int
	var name, file string
	if err := d.QueryRow("PRAGMA database_list").Scan(&seq, &name, &file); err != nil {
		t.Fatal(err)
	}
	return &Agents{Reads: &reads.Env{DB: d, Home: filepath.Dir(file)}}
}

// hire registers agents named names in one workspace, the first as a1 and so on, as agents.list lists them.
func hire(t *testing.T, d *sql.DB, names ...string) {
	t.Helper()
	exec(t, d, "INSERT INTO workspaces(id, name, folder, created_at) VALUES(1, 'team', '', 0)")
	for i, n := range names {
		exec(t, d, "INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(?, 1, ?, 'p1', 0)", i+1, n)
	}
}

func view(state string) string { return `{"name":"bob","state":"` + state + `"}` }

func TestAnAgentViewThatChangesTurnsStalledOrGoesIsToldOnceEach(t *testing.T) {
	d, l := rig(t)
	hire(t, d, "bob", "amy")
	exec(t, d, "INSERT INTO agent_view(agent, view, updated) VALUES('a1', ?, 0), ('a2', ?, 0)", view("idle"), `{"name":"amy","state":"idle"}`)
	a := agentsOn(t, d)
	emit := func(e stream.Entry) (*stream.Event, error) { return l.Append(e) }
	all := func() []*stream.Event {
		got, err := l.Since(0, []string{"agents", "notifications"}, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if err := a.Scan(d, emit); err != nil || len(all()) != 0 {
		t.Fatalf("the first look said something: %v %v", err, all())
	}
	if err := a.Scan(d, emit); err != nil || len(all()) != 0 {
		t.Fatalf("nothing changed: %v", all())
	}
	exec(t, d, "UPDATE agent_view SET view = ? WHERE agent = 'a1'", view("running"))
	a.Scan(d, emit)
	if got := all(); len(got) != 1 || got[0].Type != "agent.changed" || *got[0].Agent != "a1" || string(got[0].Target) != `{"id":"a1","screen":"agent"}` || got[0].Title != nil {
		t.Fatalf("%+v", got)
	}
	exec(t, d, "UPDATE agent_view SET view = ? WHERE agent = 'a1'", view("stalled"))
	a.Scan(d, emit)
	got := all()
	if len(got) != 3 || got[1].Type != "agent.changed" || got[2].Type != "stall" || got[2].Topic != "notifications" || *got[2].Title != "a1 stalled" || string(got[2].Data) != `{"agent":"a1"}` {
		t.Fatalf("%+v", got)
	}
	exec(t, d, "UPDATE agent_view SET view = ? WHERE agent = 'a1'", `{"name":"bob","state":"stalled","detail":"more"}`)
	a.Scan(d, emit)
	if got := all(); len(got) != 4 {
		t.Fatalf("a view that stays stalled is not a new stall: %d rows", len(got))
	}
	// An agent leaves the list when it leaves the roster, as agents.list no longer lists it.
	exec(t, d, "DELETE FROM agents WHERE id = 2")
	a.Scan(d, emit)
	got = all()
	if last := got[len(got)-1]; len(got) != 5 || last.Type != "agent.removed" || *last.Agent != "a2" || string(last.Data) != `{"key":"a2"}` {
		t.Fatalf("%+v", got)
	}
	// One hired stalled is new to the list: changed, and a stall.
	exec(t, d, "INSERT INTO agents(id, workspace, name, owner, created_at) VALUES(3, 1, 'dan', 'p1', 0)")
	exec(t, d, "INSERT INTO agent_view(agent, view, updated) VALUES('a3', ?, 0)", view("stalled"))
	a.Scan(d, emit)
	if got = all()[5:]; len(got) != 2 || got[0].Type != "agent.changed" || *got[0].Agent != "a3" || got[1].Type != "stall" {
		t.Fatalf("%+v", got)
	}
}

// A client puts agent.changed in place of its listed row: the row carries the queue count and the other fields agents.list reads
// beside the stored view, and a task that joins the queue sends it again with no change to the view.
func TestAgentChangedCarriesTheListedRowWithItsQueueCount(t *testing.T) {
	d, l := rig(t)
	hire(t, d, "bob")
	exec(t, d, "INSERT INTO agent_view(agent, view, updated) VALUES('a1', ?, 0)", view("idle"))
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(1, 'a', 'a1', 'p1', 1, 'p1')")
	exec(t, d, "INSERT INTO agent_state(agent, state, updated, current_task) VALUES('a1', 'IDLE', 0, 1)")
	a := agentsOn(t, d)
	emit := func(e stream.Entry) (*stream.Event, error) { return l.Append(e) }
	row := func(i int) map[string]any {
		t.Helper()
		got, err := l.Since(0, []string{"agents"}, "", 100)
		if err != nil || len(got) <= i {
			t.Fatalf("%v %d events", err, len(got))
		}
		var v map[string]any
		if err := json.Unmarshal(got[i].Data, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	a.Scan(d, emit)
	exec(t, d, "UPDATE agent_view SET view = ? WHERE agent = 'a1'", view("running"))
	a.Scan(d, emit)
	v := row(0)
	for _, k := range []string{"task_count", "ready", "kind", "last_to_founder", "founder_unread", "review_level", "folder", "node"} {
		if _, has := v[k]; !has {
			t.Errorf("agent.changed lacks %s: %v", k, v)
		}
	}
	if v["task_count"] != float64(1) || v["state"] != "running" {
		t.Fatalf("%v", v)
	}
	exec(t, d, "INSERT INTO tasks(id, title, owner, opened_by, created_at, queued_by) VALUES(2, 'b', 'a1', 'p1', 1, 'p1')")
	a.Scan(d, emit)
	if v := row(1); v["task_count"] != float64(2) {
		t.Fatalf("a queued task sends the row again with its count: %v", v)
	}
}

func TestAReadThatFailsAfterTheFirstRowSaysNothing(t *testing.T) {
	d, l := rig(t)
	hire(t, d, "amy", "bob")
	exec(t, d, "INSERT INTO agent_view(agent, view, updated) VALUES('a1', ?, 0), ('a2', ?, 0)", view("idle"), view("stalled"))
	emit := func(e stream.Entry) (*stream.Event, error) { return l.Append(e) }
	a := agentsOn(t, d)
	if err := a.Scan(d, emit); err != nil {
		t.Fatal(err)
	}
	// the read fails on bob's row, after amy's: a short list must not read as bob gone
	a.query = "SELECT agent, CASE WHEN agent = 'a2' THEN json_extract('{', '$') ELSE view END FROM agent_view"
	if err := a.Scan(d, emit); err == nil {
		t.Fatal("the failed read was swallowed")
	}
	a.query = ""
	if err := a.Scan(d, emit); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.Since(0, []string{"agents", "notifications"}, "", 100); len(got) != 0 {
		t.Fatalf("a failed read said %d things: %+v", len(got), got[0])
	}
}

// A card the daemon raised is a stall although the view says waiting: the notice is titled by the card and carries its title and text.
// A card from the agent's own session is no stall, and a card that stays is told once.
func TestAStallNoticeIsTitledByTheDaemonsCard(t *testing.T) {
	d, l := rig(t)
	card := func(source string) string {
		return `{"name":"bob","state":"waiting","needs_you":{"kind":"dialog","since":"x","title":"Stuck at a dialog","text":"it came back","source":"` + source + `"}}`
	}
	hire(t, d, "bob")
	exec(t, d, "INSERT INTO agent_view(agent, view, updated) VALUES('a1', ?, 0)", view("running"))
	a := agentsOn(t, d)
	emit := func(e stream.Entry) (*stream.Event, error) { return l.Append(e) }
	stalls := func() []*stream.Event {
		got, err := l.Since(0, []string{"notifications"}, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	a.Scan(d, emit)
	exec(t, d, "UPDATE agent_view SET view = ? WHERE agent = 'a1'", card("session"))
	if a.Scan(d, emit); len(stalls()) != 0 {
		t.Fatalf("a session's own card is no stall: %+v", stalls())
	}
	exec(t, d, "UPDATE agent_view SET view = ? WHERE agent = 'a1'", card("daemon"))
	a.Scan(d, emit)
	got := stalls()
	if len(got) != 1 || *got[0].Title != "a1: Stuck at a dialog" || string(got[0].Data) != `{"agent":"a1","text":"it came back","title":"Stuck at a dialog"}` {
		t.Fatalf("%+v", got)
	}
	exec(t, d, "UPDATE agent_view SET view = ? WHERE agent = 'a1'", strings.Replace(card("daemon"), "it came back", "again", 1))
	if a.Scan(d, emit); len(stalls()) != 1 {
		t.Fatal("a card that stays is told once")
	}
}

func TestAnOfferToSwitchAccountIsNoStall(t *testing.T) {
	offer := `{"name":"bob","state":"parked","needs_you":{"kind":"switch_offer","title":"No account has room to spare","source":"daemon"}}`
	if stuck(offer) {
		t.Fatal("nothing waits on the offer, so it tells no stall")
	}
	if !stuck(`{"name":"bob","state":"waiting","needs_you":{"kind":"dialog","title":"Stuck at a dialog","source":"daemon"}}`) {
		t.Fatal("a daemon's own card still does")
	}
}

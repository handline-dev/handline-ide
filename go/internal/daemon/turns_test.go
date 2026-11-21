package daemon

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/stream"
	"github.com/handline/handline/go/protocol/rpc"
)

// Two people: an agent's turn to human on a task lands on that task's person, the founder for theirs and Kim for hers. Each sees only
// their own in requests.pending; Kim answering hers hands it back to the agent, and her comment on the founder's leaves their turn.
func TestEachPersonSeesOnlyTheTurnsThatWaitOnThem(t *testing.T) {
	d, _ := tellDaemon(t, true)
	if _, err := d.db.Exec("INSERT INTO people(id, display_name, role, created_at) VALUES(2, 'Kim', 'member', 0)"); err != nil {
		t.Fatal(err)
	}
	bob := key(t, d, "bob")
	founder := rpc.Caller{Kind: "person", Key: "p1", Role: "owner"}
	kim := rpc.Caller{Kind: "device", Key: "p2", Person: "p2", Role: "member", Info: map[string]any{"device": "kim-phone"}}
	call := func(as rpc.Caller, method string, p map[string]any) map[string]any {
		t.Helper()
		got, err := d.local.Dispatch(as, "t", "", method, p)
		if err != nil {
			t.Fatalf("%s as %s: %v", method, as.Key, err)
		}
		m, _ := got.(map[string]any)
		return m
	}
	num := func(tid int64) json.Number { return json.Number(strconv.FormatInt(tid, 10)) }

	mine, err := db.NewTask(d.db, "the founder's question", bob, "p1", 0, db.DefaultPriority, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	hers, err := db.NewTask(d.db, "Kim's question", bob, bob, 0, db.DefaultPriority, "", 0, "p2") // bob opened it; it answers to Kim
	if err != nil {
		t.Fatal(err)
	}
	for _, tid := range []int64{mine, hers} {
		call(d.local.AgentCaller(bob), "task.comment.add", map[string]any{"task": num(tid), "text": "which one?", "turn": "human",
			"request_id": "turn-" + string(num(tid))})
	}
	for tid, want := range map[int64]string{mine: "p1", hers: "p2"} {
		if acting, err := db.Acting(d.db, tid); err != nil || acting != want {
			t.Fatalf("T%d waits on %q (%v), want %s", tid, acting, err, want)
		}
	}

	pending := func(as rpc.Caller) []int64 {
		t.Helper()
		var out []int64
		for _, r := range call(as, "requests.pending", nil)["answer"].([]map[string]any) {
			if task, ok := r["task"].(int64); ok {
				out = append(out, task)
			}
		}
		return out
	}
	if got := pending(founder); len(got) != 1 || got[0] != mine {
		t.Fatalf("the founder's requests: %v, want T%d", got, mine)
	}
	if got := pending(kim); len(got) != 1 || got[0] != hers {
		t.Fatalf("Kim's requests: %v, want T%d", got, hers)
	}

	unread := func(as rpc.Caller, tid int64) any {
		t.Helper()
		for _, r := range call(as, "requests.pending", nil)["answer"].([]map[string]any) {
			if r["task"] == tid {
				return r["unread"]
			}
		}
		return nil
	}
	before := unread(founder, mine)
	if n, _ := before.(int64); n == 0 {
		t.Fatalf("the founder has no unread comment on T%d: %v", mine, before)
	}
	call(kim, "task.seen", map[string]any{"task": num(mine), "request_id": "kim-seen"})
	if after := unread(founder, mine); after != before {
		t.Fatalf("Kim reading T%d moved the founder's unread from %v to %v", mine, before, after)
	}

	call(kim, "task.comment.add", map[string]any{"task": num(hers), "text": "the second", "request_id": "kim-answer"})
	if acting, _ := db.Acting(d.db, hers); acting != bob {
		t.Fatalf("Kim's answer left T%d with %s", hers, acting)
	}
	counts := func(as rpc.Caller) map[string]any {
		t.Helper()
		got := call(as, "requests.pending", nil)
		for _, c := range got["comments"].([]map[string]any) {
			if c["task"] == mine && as.Key != "p1" {
				t.Fatalf("%s's comments list the founder's T%d", as.Key, mine)
			}
		}
		return got["counts"].(map[string]any)
	}
	if n := counts(kim)["needs_you"]; n != 0 {
		t.Fatalf("Kim's needs_you counts others' tasks: %v", n)
	}
	if _, err := d.db.Exec("INSERT INTO task_memos(id, task_id, author, body, created_at) VALUES(90, ?, ?, '@human pick one?', 1)", mine, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.Exec("INSERT INTO requests(id, ref, ts, requester, target, body, state, memo_id, edge_ok) VALUES(90, 'bbbbbbbbbbb1', 1, ?, 'p1', '@human pick one?', 'queued', 90, 1)", bob); err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.Exec("INSERT INTO task_requests(task_id, request_id) VALUES(?, 90)", mine); err != nil {
		t.Fatal(err)
	}
	counts(kim) // the new comment is unread for Kim too, and still not hers
	unreadOf := func(as rpc.Caller) any {
		t.Helper()
		return call(as, "task.get", map[string]any{"task": num(mine)})["unread"]
	}
	if k, f := unreadOf(kim), unreadOf(founder); k != int64(1) || f == int64(0) {
		t.Fatalf("T%d's unread: Kim %v, the founder %v", mine, k, f)
	}
	call(kim, "task.comment.add", map[string]any{"task": num(mine), "text": "not mine", "request_id": "kim-other"})
	var state string
	if err := d.db.QueryRow("SELECT state FROM requests WHERE id = 90").Scan(&state); err != nil || state != "queued" {
		t.Fatalf("Kim's comment closed the founder's ask: %s %v", state, err)
	}
	if acting, _ := db.Acting(d.db, mine); acting != "p1" {
		t.Fatalf("Kim's comment answered the founder's turn: T%d is with %s", mine, acting)
	}
}

// A person's for_you event reaches that person's connection only: Kim gets hers, the founder's device does not.
func TestAPersonsEventReachesOnlyTheirConnections(t *testing.T) {
	d := startNet(t)
	code, _, _, _ := d.dev.Pair(time.Minute)
	owner := helloAs(t, d, joinAs(t, d, code, "owner-phone", "ios", "")["result"].(map[string]any)["token"].(string))
	kimKey := rpcAs(owner, "person.invite", `{"display_name":"Kim","request_id":"inv-kim"}`)["result"].(map[string]any)["person"].(string)
	link := rpcAs(owner, "device.pair", `{"person":"`+kimKey+`","request_id":"kim-link"}`)["result"].(map[string]any)
	kim := helloAs(t, d, joinAs(t, d, link["code"].(string), "kim-phone", "ios", "")["result"].(map[string]any)["token"].(string))
	for _, c := range []*wsClient{owner, kim} {
		if r := rpcAs(c, "subscribe", `{"topics":["for_you"]}`); r["result"] == nil {
			t.Fatalf("subscribe: %v", r)
		}
	}
	if _, err := d.hub.Append(stream.Entry{Topic: "for_you", Type: "for_you.opened", Title: "Kim's", Agent: kimKey,
		Data: map[string]any{"count": 1}}); err != nil {
		t.Fatal(err)
	}
	if m, ok := kim.next(5 * time.Second); !ok || !strings.Contains(fmt.Sprint(m), "for_you.opened") {
		t.Fatalf("Kim got %v", m)
	}
	if m, ok := owner.next(time.Second); ok {
		t.Fatalf("the founder got Kim's event: %v", m)
	}
}

// next is the next message on c within wait; false when none came.
func (c *wsClient) next(wait time.Duration) (map[string]any, bool) {
	c.ws.SetReadDeadline(time.Now().Add(wait))
	_, b, err := c.ws.ReadMessage()
	if err != nil {
		return nil, false
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	return m, true
}

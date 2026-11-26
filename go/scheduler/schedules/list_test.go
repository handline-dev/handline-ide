package schedules

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/cronspec"
)

// listed is List over the rig's one agent and its schedules.
func (g *rig) listed(now int64) []Row {
	g.t.Helper()
	rows, bad, err := List(g.q, []string{"bob"}, func(string) ([]cronspec.Schedule, error) { return g.items, nil }, now)
	if err != nil || len(bad) != 0 {
		g.t.Fatalf("list: %v, unreadable %v", err, bad)
	}
	return rows
}

func TestAScheduleTheRunnerHasNotSeenYetIsNextAfterThisMinuteAndHasNoFires(t *testing.T) {
	g := newRig(t)
	g.items = []cronspec.Schedule{{Cron: "*/5 * * * *", Message: "check the queue"}}
	rows := g.listed(t0 + 20)
	if len(rows) != 1 || rows[0].Agent != "bob" || rows[0].Schedule != g.items[0] {
		t.Fatalf("%+v", rows)
	}
	want, _ := cronspec.NextCronAfter("*/5 * * * *", t0)
	if rows[0].Next != want || rows[0].Due || len(rows[0].Fires) != 0 {
		t.Fatalf("next %d (want %d), due %v, fires %v", rows[0].Next, want, rows[0].Due, rows[0].Fires)
	}
	// the list writes nothing: the runner's first sweep still counts from its own minute
	if c, _ := db.Cursor(g.q, "schedule_at:bob:"+cronspec.SID(g.items[0])); c != 0 {
		t.Fatalf("the list wrote a cursor: %d", c)
	}
}

func TestNextIsTheFirstTimeAfterTheRunnersCursorAndTheFiresAreItsMessagesNewestFirst(t *testing.T) {
	g := newRig(t)
	g.sweep(t0)
	for i := int64(1); i <= FiresMax+2; i++ {
		g.sweep(t0 + 60*i)
		rid, _ := db.Cursor(g.q, "schedule_rid:bob:"+cronspec.SID(g.items[0]))
		// read at once, so the next minute sends again; written a minute apart
		g.q.Exec("UPDATE requests SET delivered_at = 1, ts = ? WHERE id = ?", t0+60*i, rid)
	}
	now := t0 + 60*(FiresMax+2) + 10
	row := g.listed(now)[0]
	if row.Next != t0+60*(FiresMax+3) || row.Due {
		t.Fatalf("next %d, due %v", row.Next, row.Due)
	}
	if len(row.Fires) != FiresMax {
		t.Fatalf("the newest %d of %d: %v", FiresMax, FiresMax+2, row.Fires)
	}
	last, _ := db.Cursor(g.q, "schedule_rid:bob:"+cronspec.SID(g.items[0]))
	if f := row.Fires[0]; f.Request != last || f.At != t0+60*(FiresMax+2) || len(f.Ref) != 12 {
		t.Fatalf("the newest fire is the runner's last request: %+v, want request %d", f, last)
	}
	for i := 1; i < len(row.Fires); i++ {
		if row.Fires[i].At >= row.Fires[i-1].At {
			t.Fatalf("newest first: %v", row.Fires)
		}
	}
}

func TestAScheduleDueWhileTheAgentIsMidTurnIsDueAtTheTimeItCameUp(t *testing.T) {
	g := newRig(t)
	g.sweep(t0)
	db.SetAgentState(g.q, "bob", states.Running, 0, "working")
	g.sweep(t0 + 180)
	row := g.listed(t0 + 200)[0]
	if row.Next != t0+60 || !row.Due {
		t.Fatalf("next %d (want %d), due %v", row.Next, t0+60, row.Due)
	}
}

func TestAMessageBelongsToTheScheduleWithItsCronAndItsWords(t *testing.T) {
	g := newRig(t)
	g.sweep(t0)
	g.sweep(t0 + 60)
	// edited: by its id it is another schedule, and the old one's message is not its own
	g.items = []cronspec.Schedule{{Cron: "* * * * *", Message: "check the queue twice"}, {Cron: "0 9 * * *", Message: "check the queue"}}
	for _, row := range g.listed(t0 + 90) {
		if len(row.Fires) != 0 {
			t.Fatalf("%v took the message of the schedule it replaced: %v", row.Schedule, row.Fires)
		}
	}
	// another agent's message with the same words is not this agent's
	g.q.Exec("UPDATE requests SET target = 'ann' WHERE requester = ?", Requester)
	g.items = []cronspec.Schedule{{Cron: "* * * * *", Message: "check the queue"}}
	if row := g.listed(t0 + 90)[0]; len(row.Fires) != 0 {
		t.Fatalf("another agent's message: %v", row.Fires)
	}
	// nor is a message someone else sent it with those words
	g.q.Exec("UPDATE requests SET target = 'bob', requester = 'ann' WHERE requester = ?", Requester)
	if row := g.listed(t0 + 90)[0]; len(row.Fires) != 0 {
		t.Fatalf("a message the runner did not send: %v", row.Fires)
	}
}

func TestAnExpressionThatNamesNoTimeHasNoNext(t *testing.T) {
	g := newRig(t)
	g.items = []cronspec.Schedule{{Cron: "0 0 31 2 *", Message: "never"}}
	if row := g.listed(t0)[0]; row.Next != 0 || row.Due {
		t.Fatalf("%+v", row)
	}
}

func TestAnAgentWhoseValueIsWrongIsNamedAndTheOthersAreStillListed(t *testing.T) {
	g := newRig(t)
	load := func(agent string) ([]cronspec.Schedule, error) {
		if agent == "ann" {
			return nil, &cronspec.ScheduleError{Msg: "schedule 1 is {cron, message} and nothing else"}
		}
		return g.items, nil
	}
	rows, bad, err := List(g.q, []string{"ann", "bob"}, load, t0)
	if err != nil || len(rows) != 1 || rows[0].Agent != "bob" {
		t.Fatalf("%v %+v", err, rows)
	}
	if len(bad) != 1 || bad[0] != (Unreadable{"ann", "schedule 1 is {cron, message} and nothing else"}) {
		t.Fatalf("%+v", bad)
	}
	if _, _, err := List(g.q, []string{"bob"}, func(string) ([]cronspec.Schedule, error) { return nil, fmt.Errorf("disk") }, t0); err == nil {
		t.Fatal("an agent row that cannot be read is the read's error, not an empty list")
	}
}

// asked is schedules.list's answer over home at now, for the params given.
func asked(t *testing.T, home string, p map[string]any) (map[string]any, error) {
	t.Helper()
	got, err := answer(teamtest.DB(t, home), home, p, time.Now())
	if err != nil {
		return nil, err
	}
	// as a client reads it
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var res map[string]any
	if err := json.Unmarshal(b, &res); err != nil {
		t.Fatal(err)
	}
	return res, nil
}

func TestSchedulesListGivesEachAgentsSchedulesWithASentenceTheZoneAndTheLastMessage(t *testing.T) {
	home := t.TempDir()
	d := teamtest.Seed(t, home, "gp", "gp/worker", "web")
	long := strings.Repeat("가", MessageMax+5)
	teamtest.Set(t, home, "gp/worker", "schedules", `[{"cron": "30 9 * * 1-5", "message": "morning check"}, {"cron": "*/7 * * * *", "message": "`+long+`"}]`)
	teamtest.Set(t, home, "web", "schedules", `[{"cron": "0 9 * * *", "message": "m", "paused": true}]`)
	worker, web := teamtest.Key(t, home, "gp/worker"), teamtest.Key(t, home, "web")
	morning := cronspec.Schedule{Cron: "30 9 * * 1-5", Message: "morning check"}
	rid, err := db.AddRequest(d, db.NewRequest{Requester: Requester, Target: worker, Body: Body(morning), Kind: "say"})
	if err != nil {
		t.Fatal(err)
	}

	res, err := asked(t, home, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := res["schedules"].([]any)
	if len(list) != 2 {
		t.Fatalf("the worker's two, and none of the agent whose value is wrong: %v", list)
	}
	first, second := list[0].(map[string]any), list[1].(map[string]any)
	if first["agent"] != worker || first["id"] != cronspec.SID(morning) || first["cron"] != "30 9 * * 1-5" ||
		first["sentence"] != "Every weekday at 09:30" || first["message"] != "morning check" || first["message_more"] != false ||
		first["due"] != false {
		t.Fatalf("%v", first)
	}
	if at, err := time.Parse(time.RFC3339, fmt.Sprint(first["next"])); err != nil || !at.After(time.Now()) {
		t.Fatalf("next is a time to come: %v", first["next"])
	}
	fires := first["fires"].([]any)
	if len(fires) != 1 || fires[0].(map[string]any)["request"] != float64(rid) || len(fmt.Sprint(fires[0].(map[string]any)["ref"])) != 12 {
		t.Fatalf("its one message, by the id and ref chat.locate takes: %v", fires)
	}
	// an expression with no exact sentence gets none, and a long message its first MessageMax characters
	if second["sentence"] != nil || second["message_more"] != true || len([]rune(fmt.Sprint(second["message"]))) != MessageMax ||
		len(second["fires"].([]any)) != 0 {
		t.Fatalf("%v", second)
	}
	bad := res["unreadable"].([]any)
	if len(bad) != 1 || bad[0].(map[string]any)["agent"] != web || bad[0].(map[string]any)["why"] != "schedule 1 is {cron, message} and nothing else" {
		t.Fatalf("%v", bad)
	}
	name, offset := time.Now().Zone()
	if zone := res["zone"].(map[string]any); zone["name"] != name || zone["offset_min"] != float64(offset/60) {
		t.Fatalf("the machine's zone: %v", zone)
	}

	// one agent's
	only, err := asked(t, home, map[string]any{"agent": web})
	if err != nil || len(only["schedules"].([]any)) != 0 || len(only["unreadable"].([]any)) != 1 {
		t.Fatalf("%v %v", err, only)
	}
	var missing *rpc.Error
	if _, err := asked(t, home, map[string]any{"agent": "a999"}); !errors.As(err, &missing) || !strings.Contains(missing.Error(), "a999") {
		t.Fatalf("an agent that is not one is not found: %v", err)
	}
}

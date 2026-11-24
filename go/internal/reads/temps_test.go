package reads

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/protocol/rpc"
)

// A seat's hire and its leave are lines in the chat of the agent it is of, naming the task and how it ended; the live event is the same item.
func TestASeatsHireAndLeaveAreLinesInItsAgentsChat(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	for _, q := range []string{
		`INSERT INTO events(id, ts, agent, kind, payload) VALUES(1, 1000, '` + bob + `', 'agent.hired', '{"seat":"a9","task_id":7}')`,
		`INSERT INTO events(id, ts, agent, kind, payload) VALUES(2, 1001, '` + bob + `', 'agent.left', '{"seat":"a9","task_id":7,"ended":"handed_off"}')`,
	} {
		run(t, e, q)
	}
	got, err := e.chatTranscript(nil, map[string]any{"agent": bob})
	if err != nil {
		t.Fatal(err)
	}
	items := got.(map[string]any)["items"].([]any)
	want := []map[string]any{
		{"task_id": int64(7), "seat": "a9", "phase": "started"},
		{"task_id": int64(7), "seat": "a9", "phase": "ended", "ended": "handed_off"},
	}
	if len(items) != len(want) {
		t.Fatalf("%d items: %v", len(items), items)
	}
	for i, w := range want {
		m := items[i].(map[string]any)
		if m["kind"] != "concurrent_work" || !reflect.DeepEqual(m["concurrent_work"], w) {
			t.Fatalf("item %d: %v, want %v", i, m, w)
		}
		live, agent, ok, err := e.TaskEventItem(int64(i + 1))
		if err != nil || !ok || agent != bob || !reflect.DeepEqual(live, m) {
			t.Fatalf("live item %d: %v %q %v %v", i, live, agent, ok, err)
		}
	}
}

// chat.transcript with a seat and a task reads that hire's items only, from the hire to the leave, or on to now while it is at work.
func TestASeatsTranscriptForATaskIsBoundedByThatHire(t *testing.T) {
	t.Parallel()
	e := testEnv(t)
	addAgent(t, e, "bob", "")
	bob := teamtest.Key(t, e.Home, "bob")
	seat, err := team.NewSeat(e.DB, bob, 1)
	if err != nil {
		t.Fatal(err)
	}
	run(t, e, fmt.Sprintf(`INSERT INTO events(ts, agent, kind, payload) VALUES(2000, '%[1]s', 'agent.hired', '{"seat":"%[2]s","task_id":7}'),
		(3000, '%[1]s', 'agent.left', '{"seat":"%[2]s","task_id":7,"ended":"done"}'), (4000, '%[1]s', 'agent.hired', '{"seat":"%[2]s","task_id":8}')`, bob, seat))
	for i, at := range []int{1500, 2500, 3000, 3500, 4500} {
		run(t, e, fmt.Sprintf(`INSERT INTO prompt_blocks(agent, kind, fingerprint, title, panel, detail, first_seen, last_seen, gone_at)
			VALUES('%s', 'permission', 'fp%d', 't', '[]', '{}', %d, %d, %d)`, seat, i, at, at, at))
	}
	seen := func(params map[string]any) []int64 {
		t.Helper()
		got, err := e.chatTranscript(nil, params)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, it := range got.(map[string]any)["items"].([]any) {
			out = append(out, it.(map[string]any)["id"].(int64)/1_000_000)
		}
		return out
	}
	for _, c := range []struct {
		params map[string]any
		want   []int64
	}{
		{map[string]any{"agent": seat}, []int64{1500, 2500, 3000, 3500, 4500}},
		{map[string]any{"agent": seat, "task": json.Number(fmt.Sprint(7))}, []int64{2500, 3000}},
		{map[string]any{"agent": seat, "task": json.Number(fmt.Sprint(8))}, []int64{4500}},
		{map[string]any{"agent": seat, "task": json.Number(fmt.Sprint(7)), "before": json.Number(fmt.Sprint(2600 * 1_000_000))}, []int64{2500}},
	} {
		if got := seen(c.params); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: %v, want %v", c.params, got, c.want)
		}
	}
	_, err = e.chatTranscript(nil, map[string]any{"agent": seat, "task": json.Number(fmt.Sprint(9))})
	if re, ok := err.(*rpc.Error); !ok || re.Data["what"] != "task" {
		t.Fatalf("a task the seat was never hired for: %v", err)
	}
}

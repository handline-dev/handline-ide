package usage

import (
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/paths"
)

func TestOneHourCacheWritesCostTwiceTheInputAndAWholeSessionMatchesClaudeCodesOwnTotal(t *testing.T) {
	// A real session's opus-5-5 tokens as its cost-state record kept them, and the costUSD Claude Code gave them: 5.147223.
	real := Tokens{Inp: 762, CC: 328202, CR: 9795175, Out: 48186, Thinking: 10280, CC1h: 193470}
	if got := Cost("claude-opus-5-5", real, nil); math.Abs(got-5.147223) > 1e-6 {
		t.Fatalf("priced %.6f, Claude Code said 5.147223", got)
	}
	real.CC1h = 0
	if got := Cost("claude-opus-5-5", real, nil); math.Abs(got-4.566813) > 1e-6 {
		t.Fatalf("a row with no split prices every write as a 5-minute one: %.6f", got)
	}
}

func call(ts int64, usd float64) PricedCall {
	return PricedCall{TS: ts, Model: "opus", Account: "a", USD: usd}
}

func TestAStretchIsTheCostOfItsCallsOverTheTimeTheyCoverAndIdleTimeIsNoPartOfIt(t *testing.T) {
	// 0..600: work, 1 dollar after the first call. Then idle for an hour. Then 600s more at 3 dollars, on a new session (a restart):
	// nothing falls, it is only more calls.
	calls := []PricedCall{call(0, 9), call(300, 0.5), call(600, 0.5), call(4200, 7), call(4500, 1), call(4800, 2)}
	got := Stretches(calls)
	if want := []Stretch{{0, 600, 1, "opus", "a"}, {4200, 4800, 3, "opus", "a"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v, want %+v", got, want)
	}
	if got[0].PerMin() != 0.1 || got[1].PerMin() != 0.3 {
		t.Fatalf("rates %v %v", got[0].PerMin(), got[1].PerMin())
	}
	if s := Stretches([]PricedCall{call(0, 1), call(SegMinS-1, 1)}); len(s) != 0 {
		t.Fatalf("shorter than SegMinS is no sample: %+v", s)
	}
}

func TestAStretchThatRunsOnIsClosedAtSegMaxSAndTheNextBeginsAtItsLastCall(t *testing.T) {
	var calls []PricedCall
	for ts := int64(0); ts <= 2*SegMaxS; ts += 300 {
		calls = append(calls, call(ts, 1))
	}
	got := Stretches(calls)
	if len(got) != 2 || got[0].End != SegMaxS || got[1].Start != SegMaxS || got[1].End != 2*SegMaxS || got[0].USD != 6 || got[1].USD != 6 {
		t.Fatalf("%+v", got)
	}
}

func TestAStretchIsTaggedWithTheModelOfTheAgentsOwnCallsNotItsSubagents(t *testing.T) {
	calls := []PricedCall{{TS: 0, Model: "opus"}, {TS: 100, Side: 1, Model: "haiku"}, {TS: 200, Side: 1, Model: "haiku"}, {TS: 700, Model: "opus", USD: 1}}
	if got := Stretches(calls); len(got) != 1 || got[0].Model != "opus" {
		t.Fatalf("%+v", got)
	}
}

func TestTheDollarPaceIsTheModelsMedianElseAllAgentsElseTheStartingValueAndEffortIsNoTier(t *testing.T) {
	p := &DollarPace{}
	if r, src, n := p.Rate("opus"); r != TypicalAgentUSDPerMin || src != "starting value" || n != 0 {
		t.Fatalf("%v %s %d", r, src, n)
	}
	for i, r := range []float64{1, 2, 3, 4} {
		p.Samples = append(p.Samples, DollarSample{T: int64(i), R: r, M: "sonnet", E: "high", Acct: "a"})
	}
	p.Samples = append(p.Samples, DollarSample{T: 9, R: 10, M: "opus", E: "low", Acct: "b"})
	if r, src, n := p.Rate("opus"); r != 3 || src != "all agents" || n != 5 {
		t.Fatalf("one opus sample is too few, so the pool across accounts: %v %s %d", r, src, n)
	}
	for i := 0; i < 4; i++ {
		p.Samples = append(p.Samples, DollarSample{T: int64(10 + i), R: 20, M: "opus", E: []string{"low", "high"}[i%2], Acct: "c"})
	}
	if r, src, n := p.Rate("opus"); r != 20 || src != "opus" || n != 5 {
		t.Fatalf("opus samples of any effort and account: %v %s %d", r, src, n)
	}
	by := p.ByModel()
	if len(by) != 3 || by[0].Model != "opus" || by[1].Model != "sonnet" || by[1].Source != "all agents" || by[2].Model != "all agents" || by[2].Samples != 9 {
		t.Fatalf("%+v", by)
	}
}

// ownerEnv is a home with agents a and b (keys a1 and a2, which its rows name), a's status file the shape the pull writes, and tasks 1
// (a's, doing), 2 (closed), 3 (b's).
func ownerEnv(t *testing.T) (*Env, db.Q, *int64) {
	t.Helper()
	e, q, now := burnEnv(t)
	for _, a := range []string{"a", "b"} {
		teamtest.Set(t, e.Home, a, "model", "opus")
	}
	status := `{
  "account": "gamma",
  "agent": "a",
  "collected_at": 1793746345,
  "cost_usd": 5.147891899999999,
  "ctx_pct": 16,
  "ctx_size": 1000000,
  "ctx_used": {"cache_creation_input_tokens": 593, "cache_read_input_tokens": 161877, "input_tokens": 2, "output_tokens": 203},
  "cwd": "/w/a",
  "lines_added": 55,
  "lines_removed": 0,
  "model_id": "claude-opus-5-5",
  "pulled_at": 1793746346,
  "session_id": "sess-a",
  "source": "pull",
  "transcript_path": "/h/accounts/gamma/.claude/projects/-w-a/sess-a.jsonl"
}
`
	if err := os.WriteFile(paths.AgentStatusFile(e.Home, "a"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"INSERT INTO tasks(id, title, owner, opened_by, created_at) VALUES(1, 'one', 'a1', 'h', 1), (2, 'two', 'a1', 'h', 1), (3, 'three', 'a2', 'h', 1)",
		"UPDATE tasks SET done_evidence = 'x' WHERE id = 2",
		"INSERT INTO agent_state(agent, state, updated, current_task, resume_session) VALUES('a1', 'RUNNING', 1, 1, 'old-a'), ('a2', 'RUNNING', 1, 3, 'sess-b')",
	} {
		if _, err := q.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return e, q, now
}

func TestCallsAreChargedToTheirSessionsAgentAndTheTaskItHoldsAsDoing(t *testing.T) {
	e, q, now := ownerEnv(t)
	owners, err := e.CallOwners(q)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(owners["sess-a"], Owner{Agent: "a1", Task: 1}) || !reflect.DeepEqual(owners["old-a"], Owner{Agent: "a1", Task: 1}) ||
		!reflect.DeepEqual(owners["sess-b"], Owner{Agent: "a2", Task: 3}) {
		t.Fatalf("%+v", owners)
	}
	if _, err := q.Exec("UPDATE agent_state SET current_task = 2 WHERE agent = 'a1'"); err != nil {
		t.Fatal(err)
	}
	if owners, _ := e.CallOwners(q); owners["sess-a"].Task != 0 {
		t.Fatalf("a closed task is held by nobody: %+v", owners)
	}
	if _, err := q.Exec("UPDATE agent_state SET current_task = 3 WHERE agent = 'a1'"); err != nil {
		t.Fatal(err)
	}
	if owners, _ := e.CallOwners(q); owners["sess-a"].Task != 0 {
		t.Fatalf("a task another agent owns is not a's: %+v", owners)
	}
	// a closed T2 100s ago and handed T3 on 50s ago, after the tick filed its last calls: a call made before each is that task's.
	if _, err := q.Exec("UPDATE tasks SET done_at = ? WHERE id = 2", *now-100); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Exec(`INSERT INTO events(ts, agent, kind, payload) VALUES(?, 'a1', 'task_handoff', '{"task_id": 3, "from": "a1", "to": "a2"}')`, *now-50); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Exec("UPDATE agent_state SET current_task = 1 WHERE agent = 'a1'"); err != nil {
		t.Fatal(err)
	}
	owners, _ = e.CallOwners(q)
	a := owners["sess-a"]
	if a.TaskAt(*now-120) != 2 || a.TaskAt(*now-100) != 2 || a.TaskAt(*now-60) != 3 || a.TaskAt(*now-10) != 1 {
		t.Fatalf("%+v", a)
	}
	if _, err := q.Exec("UPDATE tasks SET done_at = ? WHERE id = 2", *now-ChargeFreshS-1); err != nil {
		t.Fatal(err)
	}
	if owners, _ := e.CallOwners(q); len(owners["sess-a"].Ended) != 1 {
		t.Fatalf("only the window's ends are kept: %+v", owners["sess-a"])
	}
}

func charged(t *testing.T, q db.Q, req string) (agent string, task *int64, cc1h int64) {
	t.Helper()
	if err := q.QueryRow("SELECT agent, task_id, cc1h FROM token_calls WHERE req = ?", req).Scan(&agent, &task, &cc1h); err != nil {
		t.Fatal(err)
	}
	return
}

func TestAFreshCallGoesToTheHeldTaskALateOneToNoTaskAndALaterLineNeverMovesIt(t *testing.T) {
	_, q, now := ownerEnv(t)
	owners := map[string]Owner{"sess-a": {Agent: "a", Task: 1}}
	fresh := Call{Session: "sess-a", Req: "r1", TS: *now - 10, Model: "opus", Tokens: Tokens{CC: 100, CC1h: 40}}
	late := Call{Session: "sess-a", Req: "r2", TS: *now - ChargeFreshS - 1, Model: "opus"}
	stranger := Call{Session: "someone-else", Req: "r3", TS: *now, Model: "opus"}
	if err := saveCalls(q, "gamma", []Call{fresh, late, stranger}, owners, *now); err != nil {
		t.Fatal(err)
	}
	if a, task, cc1h := charged(t, q, "r1"); a != "a" || task == nil || *task != 1 || cc1h != 40 {
		t.Fatalf("%s %v %d", a, task, cc1h)
	}
	if a, task, _ := charged(t, q, "r2"); a != "a" || task != nil {
		t.Fatalf("a call found late is a's but no task's: %s %v", a, task)
	}
	if a, task, _ := charged(t, q, "r3"); a != "" || task != nil {
		t.Fatalf("a session of no agent is nobody's: %s %v", a, task)
	}
	// The same request's later line, after a moved on to task 3: the call stays where it was charged, its tokens grow.
	fresh.Tokens.CC1h = 60
	if err := saveCalls(q, "gamma", []Call{fresh}, map[string]Owner{"sess-a": {Agent: "a", Task: 3}}, *now); err != nil {
		t.Fatal(err)
	}
	if a, task, cc1h := charged(t, q, "r1"); a != "a" || *task != 1 || cc1h != 60 {
		t.Fatalf("%s %v %d", a, *task, cc1h)
	}
}

func TestATasksSpendFollowsItsHoldersAcrossATaskChangeAndAHandoffAndAParentSumsItsChildren(t *testing.T) {
	e, q, now := ownerEnv(t)
	if _, err := q.Exec("INSERT INTO tasks(id, title, owner, opened_by, created_at, parent_task) VALUES(4, 'child', 'b', 'h', 1, 1), (5, 'grandchild', 'b', 'h', 1, 4)"); err != nil {
		t.Fatal(err)
	}
	// One input token of haiku costs a millionth of a dollar: calls priced in millionths.
	put := func(sess, req string, ts int64, owner Owner, millionths int64) {
		t.Helper()
		c := Call{Session: sess, Req: req, TS: ts, Model: "claude-haiku-5-5", Tokens: Tokens{Inp: millionths}}
		if err := saveCalls(q, "gamma", []Call{c}, map[string]Owner{sess: owner}, ts); err != nil {
			t.Fatal(err)
		}
	}
	t0 := *now
	put("sess-a", "1", t0, Owner{Agent: "a", Task: 1}, 1_000_000)
	put("sess-a", "2", t0+60, Owner{Agent: "a", Task: 1}, 1_000_000)
	put("sess-a", "3", t0+120, Owner{Agent: "a", Task: 3}, 2_000_000) // a changed task mid-stretch: this call is task 3's
	put("sess-a", "4", t0+180, Owner{Agent: "a", Task: 1}, 1_000_000) // and back
	put("sess-b", "5", t0+240, Owner{Agent: "b", Task: 1}, 4_000_000) // T1 handed to b: same task, b's row
	put("sess-b", "6", t0+300+IdleGapS, Owner{Agent: "b", Task: 1}, 1_000_000)
	put("sess-b", "7", t0+400, Owner{Agent: "b", Task: 5}, 3_000_000)
	got, err := e.TaskSpend(q, []int64{1, 3, 4, 2})
	if err != nil {
		t.Fatal(err)
	}
	one, three, four := got[1], got[3], got[4]
	if three == nil || three.USD != 2 || three.ByAgent["a"] != 2 {
		t.Fatalf("task 3: %+v", three)
	}
	if four == nil || four.USD != 3 || four.ByAgent["b"] != 3 {
		t.Fatalf("a parent has its child's child: %+v", four)
	}
	// T1: its own 1+1+1 (a) + 4+1 (b), and 3 from T5 under T4 under it.
	if one == nil || one.USD != 11 || one.ByAgent["a"] != 3 || one.ByAgent["b"] != 8 || one.ByModel["claude-haiku-5-5"] != 11 {
		t.Fatalf("task 1: %+v", one)
	}
	// Minutes: only a gap between two calls both on the task counts: a's 0 -> 60 on T1. 60 -> 120 -> 180 crosses T3, b's 240 has no call
	// of b's before it, and b's T1 call after T5's comes past an idle gap. So no minute is counted for two tasks, nor twice up the chain.
	if one.Minutes != 1 || three.Minutes != 0 || four.Minutes != 0 {
		t.Fatalf("minutes %v %v %v", one.Minutes, three.Minutes, four.Minutes)
	}
	if got[2] != nil {
		t.Fatalf("a task with nothing charged is absent: %+v", got[2])
	}
}

func TestTheDollarPaceIsReadFromTheChargedCallsAtMostEveryTenMinutes(t *testing.T) {
	e, q, now := ownerEnv(t)
	for i := int64(0); i <= 4; i++ { // a, opus: 5 calls 300s apart, a dollar each after the first
		c := Call{Session: "sess-a", Req: string(rune('a' + i)), TS: *now - 1200 + i*300, Model: "claude-haiku-5-5", Tokens: Tokens{Inp: 1_000_000}}
		if err := saveCalls(q, "gamma", []Call{c}, map[string]Owner{"sess-a": {Agent: "a", Task: 1}}, c.TS); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.RefreshDollarPace(q, *now, false); err != nil {
		t.Fatal(err)
	}
	p, err := LoadDollarPace(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Samples) != 1 || p.Samples[0].R != 0.2 || p.Samples[0].Agent != "a" || p.Samples[0].E != "high" || p.Samples[0].Acct != "gamma" ||
		p.Samples[0].M != "claude-haiku-5-5" {
		t.Fatalf("%+v", p.Samples)
	}
	if _, err := q.Exec("DELETE FROM token_calls"); err != nil {
		t.Fatal(err)
	}
	e.RefreshDollarPace(q, *now+RefreshEveryS-1, false)
	if p, _ := LoadDollarPace(q); len(p.Samples) != 1 {
		t.Fatal("refreshed before ten minutes")
	}
	e.RefreshDollarPace(q, *now+RefreshEveryS, false)
	if p, _ := LoadDollarPace(q); len(p.Samples) != 0 {
		t.Fatalf("not refreshed after ten minutes: %+v", p.Samples)
	}
}

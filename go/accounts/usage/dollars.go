package usage

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
)

// What agents and tasks spend in API-equivalent dollars: each token_calls row priced at list price (Cost). A dollar is the same on every
// account and plan, so one pool of samples serves them all; turning dollars into an account's percent is the account factor's job, not
// this file's (factor.go), and Costs (load.go) prices an agent on an account with both.

const (
	// ChargeFreshS: a call filed this long after it was made is charged to no task, since the task its agent holds now need not be the
	// one it held then (a transcript found late, or a backfill).
	ChargeFreshS = 300
	// IdleGapS: a gap between an agent's calls longer than this is idle time, not work: it ends a stretch and counts toward no task.
	IdleGapS = 900
	// PaceWindowS is how far back the dollar pace is read.
	PaceWindowS = 3 * 86400
	// TypicalAgentUSDPerMin is the starting value for what one agent spends per minute of work, before any sample.
	TypicalAgentUSDPerMin = 0.15

	dollarPaceMeta = "dollarpace"
)

// Owner is who a session's calls are charged to: its agent, the task that agent holds now (0 for none), and the tasks it ended (closed,
// discarded or handed on) within the last ChargeFreshS, oldest first: a call made before one of those ended was that task's, since the
// tick files calls only after the agents have acted.
type Owner struct {
	Agent string
	Task  int64
	Ended []Ended
}

// Ended is a task an agent stopped holding, and when.
type Ended struct{ Task, At int64 }

// TaskAt is the task a call made at ts is charged to: the first task ended at or after ts, else the one held now.
func (o Owner) TaskAt(ts int64) int64 {
	for _, e := range o.Ended {
		if e.At >= ts {
			return e.Task
		}
	}
	return o.Task
}

// CallOwners maps each agent's session ids (the one its status file names and the one it would resume) to the agent and the task it
// holds as doing: its current task while that task is open and still its own.
func (e *Env) CallOwners(q db.Q) (map[string]Owner, error) {
	held := map[string]int64{}
	resume := map[string]string{}
	rows, err := q.Query("SELECT s.agent, COALESCE(s.resume_session, ''), COALESCE(t.id, 0) FROM agent_state s LEFT JOIN tasks t ON t.id = s.current_task " +
		"AND t.owner = s.agent AND t.done_evidence IS NULL AND t.discarded_by IS NULL")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var agent, sess string
		var tid int64
		if err := rows.Scan(&agent, &sess, &tid); err != nil {
			rows.Close()
			return nil, err
		}
		held[agent], resume[agent] = tid, sess
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ended, err := endedLately(q, int64(e.now())-ChargeFreshS)
	if err != nil {
		return nil, err
	}
	out := map[string]Owner{}
	for _, agent := range paths.ListAgents(e.Home) {
		o := Owner{Agent: agent, Task: held[agent], Ended: ended[agent]}
		if s := resume[agent]; s != "" {
			out[s] = o
		}
		if s := statusSession(paths.AgentStatusFile(e.Home, agent)); s != "" {
			out[s] = o
		}
	}
	return out, nil
}

// endedLately is, by agent, the tasks it closed, discarded or handed on since since, oldest first.
func endedLately(q db.Q, since int64) (map[string][]Ended, error) {
	rows, err := q.Query("SELECT owner, id, done_at FROM tasks WHERE done_at >= ?1 "+
		"UNION ALL SELECT owner, id, discarded_at FROM tasks WHERE discarded_at >= ?1 "+
		"UNION ALL SELECT json_extract(payload, '$.from'), json_extract(payload, '$.task_id'), ts FROM events WHERE kind = 'task_handoff' AND ts >= ?1 "+
		"ORDER BY 3", since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]Ended{}
	for rows.Next() {
		var agent string
		var e Ended
		if err := rows.Scan(&agent, &e.Task, &e.At); err != nil {
			return nil, err
		}
		out[agent] = append(out[agent], e)
	}
	return out, rows.Err()
}

// statusSession is the session id an agent's status file names, stale or not: a session id belongs to its agent for good.
func statusSession(file string) string {
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var v struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	return v.SessionID
}

// PricedCall is one call as the dollar figures read it.
type PricedCall struct {
	TS             int64
	Side           int
	Model, Account string
	USD            float64
}

// Stretch is a run of an agent's work: calls no more than IdleGapS apart, from Start to End, what they cost after the first (the first
// call's cost belongs to the time before it), and the model and account most of its own calls ran on.
type Stretch struct {
	Start, End     int64
	USD            float64
	Model, Account string
}

// PerMin is the stretch's dollars per minute of work.
func (s Stretch) PerMin() float64 { return s.USD / (float64(s.End-s.Start) / 60) }

// Stretches cuts one agent's calls, in time order, into stretches of at least SegMinS of work: a gap longer than IdleGapS ends one, and
// one reaching SegMaxS is closed and the next begins at its last call. Only rising totals exist here, so a session's restart is a new
// session's calls, never a fall.
func Stretches(calls []PricedCall) []Stretch {
	var out []Stretch
	var open []PricedCall
	flush := func() {
		if len(open) < 2 || open[len(open)-1].TS-open[0].TS < SegMinS {
			return
		}
		s := Stretch{Start: open[0].TS, End: open[len(open)-1].TS}
		models, accts := map[string]int{}, map[string]int{}
		for i, c := range open {
			if i > 0 {
				s.USD += c.USD
			}
			if c.Side == 0 {
				models[c.Model]++
			}
			accts[c.Account]++
		}
		s.Model, s.Account = mostOf(models), mostOf(accts)
		out = append(out, s)
	}
	for _, c := range calls {
		if len(open) > 0 && c.TS-open[len(open)-1].TS > IdleGapS {
			flush()
			open = nil
		}
		open = append(open, c)
		if c.TS-open[0].TS >= SegMaxS {
			flush()
			open = []PricedCall{c}
		}
	}
	flush()
	return out
}

func mostOf(m map[string]int) string {
	best, n := "", 0
	for k, v := range m {
		if v > n || (v == n && k < best) {
			best, n = k, v
		}
	}
	return best
}

// DollarSample is one stretch of one agent: when it ended, the rate, and the model, effort and account it ran on.
type DollarSample struct {
	T          int64   `json:"t"`
	R          float64 `json:"r"` // USD per minute
	Agent      string  `json:"agent"`
	M, E, Acct string
}

// DollarPace is the pooled dollar samples, oldest first, across every account.
type DollarPace struct {
	At      int64          `json:"at"`
	Samples []DollarSample `json:"samples"`
}

// Rate is what one agent of model spends per minute of work: the median of its model's latest Keep samples, else of all agents', else
// the starting value; with where it came from and how many samples stand behind it. Effort is no tier: it rides on each sample only.
func (p *DollarPace) Rate(model string) (float64, string, int) {
	pick := func(match func(DollarSample) bool) (float64, int, bool) {
		return latestMedian(p.Samples, match, func(s DollarSample) float64 { return s.R })
	}
	if model != "" {
		if m, n, ok := pick(func(s DollarSample) bool { return s.M == model }); ok {
			return m, model, n
		}
	}
	if m, n, ok := pick(func(DollarSample) bool { return true }); ok {
		return m, "all agents", n
	}
	return TypicalAgentUSDPerMin, "starting value", 0
}

// ModelRate is one model's line of the dollar pace, as the reads show it.
type ModelRate struct {
	Model     string  `json:"model"`
	USDPerMin float64 `json:"usd_per_min"`
	Source    string  `json:"source"`
	Samples   int     `json:"samples"`
}

// ByModel is the rate of every model with samples, sorted, then the pool of all agents.
func (p *DollarPace) ByModel() []ModelRate {
	seen := map[string]bool{}
	var models []string
	for _, s := range p.Samples {
		if s.M != "" && !seen[s.M] {
			seen[s.M] = true
			models = append(models, s.M)
		}
	}
	sort.Strings(models)
	out := []ModelRate{}
	for _, m := range append(models, "") {
		r, src, n := p.Rate(m)
		name := m
		if m == "" {
			name = "all agents"
		}
		out = append(out, ModelRate{Model: name, USDPerMin: r, Source: src, Samples: n})
	}
	return out
}

// priced reads the calls charged to an agent in (lo, hi], priced, by agent in time order.
func priced(q db.Q, table []priceEntry, query string, args ...any) (map[string][]PricedCall, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]PricedCall{}
	for rows.Next() {
		var agent string
		var c PricedCall
		var t Tokens
		if err := rows.Scan(&agent, &c.TS, &c.Side, &c.Model, &c.Account, &t.Inp, &t.CC, &t.CR, &t.Out, &t.Thinking, &t.CC1h); err != nil {
			return nil, err
		}
		c.USD = Cost(c.Model, t, table)
		out[agent] = append(out[agent], c)
	}
	return out, rows.Err()
}

const pricedCols = "agent, ts, side, model, account, inp, cc, cr, out, thinking, cc1h"

// pricedColsOf is pricedCols of the table named t.
func pricedColsOf(t string) string {
	cols := strings.Split(pricedCols, ", ")
	for i, c := range cols {
		cols[i] = t + "." + c
	}
	return strings.Join(cols, ", ")
}

// RefreshDollarPace recomputes the dollar samples from the last PaceWindowS of calls and keeps them, at most every RefreshEveryS.
func (e *Env) RefreshDollarPace(q db.Q, now int64, force bool) error {
	if !force {
		if p, err := LoadDollarPace(q); err != nil || now-p.At < RefreshEveryS {
			return err
		}
	}
	by, err := priced(q, e.prices(), "SELECT "+pricedCols+" FROM token_calls WHERE agent <> '' AND ts > ? ORDER BY ts, rowid", now-PaceWindowS)
	if err != nil {
		return err
	}
	p := DollarPace{At: now, Samples: []DollarSample{}}
	for agent, calls := range by {
		_, effort, _ := e.kindOf(agent)
		for _, s := range Stretches(calls) {
			p.Samples = append(p.Samples, DollarSample{T: s.End, R: s.PerMin(), Agent: agent, M: s.Model, E: effort, Acct: s.Account})
		}
	}
	sort.SliceStable(p.Samples, func(i, j int) bool { return p.Samples[i].T < p.Samples[j].T })
	if len(p.Samples) > KeepTotal {
		p.Samples = p.Samples[len(p.Samples)-KeepTotal:]
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return db.SetMeta(q, dollarPaceMeta, string(b))
}

// LoadDollarPace is the kept dollar samples; none yet is an empty pace.
func LoadDollarPace(q db.Q) (*DollarPace, error) {
	raw, _, err := db.Meta(q, dollarPaceMeta)
	if err != nil {
		return nil, err
	}
	p := &DollarPace{}
	if raw != "" && json.Unmarshal([]byte(raw), p) != nil {
		p = &DollarPace{} // an unreadable pace is recomputed on the next refresh
	}
	return p, nil
}

// Spend is what a task cost: dollars, minutes of work, and the dollars by agent and by model.
type Spend struct {
	USD      float64            `json:"usd"`
	Minutes  float64            `json:"minutes"`
	ByAgent  map[string]float64 `json:"by_agent"`
	ByModel  map[string]float64 `json:"by_model"`
	hasCalls bool
}

func (s *Spend) add(o *Spend) {
	s.USD += o.USD
	s.Minutes += o.Minutes
	for k, v := range o.ByAgent {
		s.ByAgent[k] += v
	}
	for k, v := range o.ByModel {
		s.ByModel[k] += v
	}
	s.hasCalls = s.hasCalls || o.hasCalls
}

func newSpend() *Spend { return &Spend{ByAgent: map[string]float64{}, ByModel: map[string]float64{}} }

// TaskSpend is what each task in ids cost, its open and closed children's included, at any depth: every call charged to it, and as
// minutes each gap of no more than IdleGapS between two of one agent's calls that were both this task's - a gap that ends or begins on
// another task belongs to neither, so no minute is counted twice up a parent chain. A task with no calls charged under it is absent.
func (e *Env) TaskSpend(q db.Q, ids []int64) (map[int64]*Spend, error) {
	if len(ids) == 0 {
		return map[int64]*Spend{}, nil
	}
	children := map[int64][]int64{}
	rows, err := q.Query("SELECT id, parent_task FROM tasks WHERE parent_task IS NOT NULL")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, p int64
		if err := rows.Scan(&id, &p); err != nil {
			rows.Close()
			return nil, err
		}
		children[p] = append(children[p], id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	subtree := map[int64][]int64{} // each asked task and every task under it
	want := map[int64]bool{}
	for _, id := range ids {
		seen := map[int64]bool{}
		for todo := []int64{id}; len(todo) > 0; todo = todo[1:] {
			if at := todo[0]; !seen[at] {
				seen[at], want[at] = true, true
				subtree[id] = append(subtree[id], at)
				todo = append(todo, children[at]...)
			}
		}
	}
	marks := make([]string, 0, len(want))
	args := make([]any, 0, len(want))
	for tid := range want {
		marks, args = append(marks, "?"), append(args, tid)
	}
	// prev is the agent's call just before this one, on whatever task: the (agent, ts) index finds it.
	rows, err = q.Query("SELECT c.task_id, "+pricedColsOf("c")+", (SELECT p.ts FROM token_calls p WHERE p.agent <> '' AND p.agent = c.agent AND p.ts <= c.ts AND p.rowid <> c.rowid "+
		"ORDER BY p.ts DESC, p.rowid DESC LIMIT 1), (SELECT p.task_id FROM token_calls p WHERE p.agent <> '' AND p.agent = c.agent AND p.ts <= c.ts AND p.rowid <> c.rowid "+
		"ORDER BY p.ts DESC, p.rowid DESC LIMIT 1) FROM token_calls c WHERE c.task_id IN ("+strings.Join(marks, ",")+")", args...)
	if err != nil {
		return nil, err
	}
	table := e.prices()
	own := map[int64]*Spend{}
	for rows.Next() {
		var tid int64
		var agent string
		var c PricedCall
		var t Tokens
		var prevTS, prevTask *int64
		if err := rows.Scan(&tid, &agent, &c.TS, &c.Side, &c.Model, &c.Account, &t.Inp, &t.CC, &t.CR, &t.Out, &t.Thinking, &t.CC1h, &prevTS, &prevTask); err != nil {
			rows.Close()
			return nil, err
		}
		s := own[tid]
		if s == nil {
			s = newSpend()
			own[tid] = s
		}
		usd := Cost(c.Model, t, table)
		s.USD += usd
		s.ByAgent[agent] += usd
		s.ByModel[c.Model] += usd
		s.hasCalls = true
		if prevTS != nil && prevTask != nil && *prevTask == tid && c.TS-*prevTS <= IdleGapS {
			s.Minutes += float64(c.TS-*prevTS) / 60
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[int64]*Spend{}
	for _, id := range ids {
		total := newSpend()
		for _, tid := range subtree[id] {
			if s := own[tid]; s != nil {
				total.add(s)
			}
		}
		if total.hasCalls {
			out[id] = total
		}
	}
	return out, nil
}

// prices is the price table config.yaml leaves.
func (e *Env) prices() []priceEntry { return PricesFromConfig(e.config()) }

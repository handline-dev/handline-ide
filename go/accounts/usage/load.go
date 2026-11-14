package usage

import (
	"database/sql"
	"fmt"
	"math"
	"sort"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
)

// What one more agent costs an account and how full the account is (load.py). The costs are dollars (dollars.go) over the account's
// factors (factor.go); with nothing measured a typical agent's starting value stands.

const (
	// TypicalAgentBurnPerS is triggers.TYPICAL_AGENT_BURN_PER_S: the starting value for what one agent costs, percent of the shortest
	// window per second.
	TypicalAgentBurnPerS = 0.75 / 60
	// MinStretchS is the least stretch worth beginning: below it a start costs more than it gets back.
	MinStretchS = 30 * 60

	Keep       = 15
	MinSamples = 5
	// FloorPerS and CeilingPerS keep an estimate inside what an agent mid-turn can cost.
	FloorPerS   = 0.1 / 60
	CeilingPerS = 2.0 / 60

	// WakeReserveS: a reservation no launch answered in this long is let go.
	WakeReserveS = 300
)

// StretchS is triggers.stretch_s: how long an account lasts before its wall with one more agent on it, at that agent's cost.
func StretchS(headroomPct, fillPerS, agentPerS float64) float64 {
	rate := max(0.0, fillPerS) + agentPerS
	if rate > 0 {
		return headroomPct / rate
	}
	return math.Inf(1)
}

// Costs is what one more agent costs, read once and shared by every account's load: its model's dollar pace (DollarPace), turned into
// points of an account's windows by that account's factors (AccountFactors).
type Costs struct {
	pace    *DollarPace
	factors map[string]AccountFactors
	kindOf  func(agent string) (model, effort string, ok bool)
}

// LongPerShort is the points of the week one point of the 5-hour window costs on acct: its 5-hour factor over its week factor; ok false
// when either is unknown, and no 5-hour figure can be turned into the week's.
func (c *Costs) LongPerShort(acct string) (float64, bool) {
	f, ok := c.factors[acct]
	if !ok || f.Short.USDPerPct <= 0 || f.Long.USDPerPct <= 0 {
		return 0, false
	}
	return f.Short.USDPerPct / f.Long.USDPerPct, true
}

// USDPerMin is what one agent of agent's kind spends per minute of work (DollarPace.Rate of its model), where it came from, and the
// samples behind it.
func (c *Costs) USDPerMin(agent string) (float64, string, int) {
	model := ""
	if agent != "" && c.kindOf != nil {
		model, _, _ = c.kindOf(agent)
	}
	return c.pace.Rate(model)
}

// Describe is what one agent of agent's kind costs acct in percent of its 5-hour window per second, where it came from and the samples
// behind it: its dollar pace over the account's 5-hour factor, else the starting value.
func (c *Costs) Describe(acct, agent string) (float64, string, int) {
	f := c.factors[acct].Short.USDPerPct
	usd, source, n := c.USDPerMin(agent)
	if f <= 0 || n == 0 {
		return TypicalAgentBurnPerS, "starting value", 0
	}
	return min(CeilingPerS, max(FloorPerS, usd/60/f)), source, n
}

// PerS is what one agent of agent's kind costs acct, percent of its 5-hour window per second.
func (c *Costs) PerS(acct, agent string) float64 { p, _, _ := c.Describe(acct, agent); return p }

// Measured is PerS when samples and the account's factor stand behind it; ok false for a starting value, which is a guess and holds
// nothing back.
func (c *Costs) Measured(acct, agent string) (float64, bool) {
	p, _, n := c.Describe(acct, agent)
	return p, n > 0
}

// Burn is how fast acct's 5-hour window fills with agents mid-turn on it, percent per second: the sum of their costs. ok false when the
// account has no factor or no agent's pace is measured yet, so nothing is guessed.
func (c *Costs) Burn(acct string, agents []string) (float64, bool) {
	if c.factors[acct].Short.USDPerPct <= 0 {
		return 0, false
	}
	if _, _, n := c.pace.Rate(""); n == 0 {
		return 0, false
	}
	sum := 0.0
	for _, a := range agents {
		sum += c.PerS(acct, a)
	}
	return sum, true
}

// LoadCosts reads the dollar pace and the account factors.
func (e *Env) LoadCosts(q db.Q) (*Costs, error) {
	pace, err := LoadDollarPace(q)
	if err != nil {
		return nil, err
	}
	factors, err := Factors(q)
	if err != nil {
		return nil, err
	}
	return &Costs{pace: pace, factors: factors, kindOf: e.kindOf}, nil
}

// BurnNow is Costs.Burn of acct with the agents mid-turn on it now, exclude left out.
func (e *Env) BurnNow(q db.Q, acct, exclude string) (float64, bool) {
	costs, err := e.LoadCosts(q)
	if err != nil {
		return 0, false
	}
	by, err := e.ActiveByAccount(q)
	if err != nil {
		return 0, false
	}
	return costs.Burn(acct, without(by[acct], exclude))
}

func without(names []string, exclude string) []string {
	var out []string
	for _, n := range names {
		if n != exclude {
			out = append(out, n)
		}
	}
	return out
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// latestMedian is the median of rate over the newest Keep of samples (oldest first) that pick takes, with how many; ok false below
// MinSamples.
func latestMedian[T any](samples []T, pick func(T) bool, rate func(T) float64) (float64, int, bool) {
	var vals []float64
	for i := len(samples) - 1; i >= 0 && len(vals) < Keep; i-- {
		if pick(samples[i]) {
			vals = append(vals, rate(samples[i]))
		}
	}
	if len(vals) < MinSamples {
		return 0, 0, false
	}
	return median(vals), len(vals), true
}

func decodeObject(raw string, out *map[string]any) bool {
	var v any
	if decodeNumbers([]byte(raw), &v) != nil {
		return false
	}
	m, ok := v.(map[string]any)
	*out = m
	return ok
}

// kindOf is burnrate.kind_of: (model, effort) as configured; the model of another vendor than the default is prefixed with it.
func (e *Env) kindOf(agent string) (string, string, bool) {
	cfg, err := e.agentConfig(agent)
	if err != nil {
		return "", "", false // one agent's bad config is not the estimate's problem
	}
	vendor, model, effort := team.Parts(cfg)
	if vendor != team.DefaultVendor {
		model = vendor + "/" + model
	}
	return model, effort, true
}

// Load is how full an account is with agents, as opposed to with usage.
type Load struct {
	Account string
	Active  int      // agents mid-turn on it
	Burn    *float64 // percent of the shortest window per second (Costs.Burn); nil unknown
	Costs   *Costs
}

// HoldDroppedKind lets go a slot held for a start that will not happen. It is not wake_dropped, which a start's waiter reads as the
// wake's own answer: a resume refused before a fresh retry starts the agent must not read as not started.
const HoldDroppedKind = "hold_dropped"

const activeSQL = "SELECT agent, state, closed_at, (SELECT w.account FROM events w WHERE w.agent = agent_state.agent " +
	"AND w.kind = 'wake_reserved' AND w.ts > ? AND w.id > COALESCE((SELECT MAX(s.id) FROM events s " +
	"WHERE s.agent = agent_state.agent AND s.kind IN ('Stop', 'wake_failed', 'wake_dropped', '" + HoldDroppedKind + "')), 0) " +
	"ORDER BY w.id DESC LIMIT 1) AS reserved FROM agent_state WHERE reserved IS NOT NULL OR " +
	"(closed_at IS NULL AND (state = 'RUNNING' OR " +
	"EXISTS (SELECT 1 FROM events m WHERE m.agent = agent_state.agent " +
	"AND m.kind = 'moved_account' AND m.id > COALESCE((SELECT MAX(s.id) FROM events s " +
	"WHERE s.agent = agent_state.agent AND s.kind = 'Stop'), 0))))"

// ActiveByAccount is load.active_by_account: the agents mid-turn, by the account each is running on. An agent just moved counts from
// the move until its next Stop, and a woken one from its wake_reserved event on the account reserved, unless it is seen running.
func (e *Env) ActiveByAccount(q db.Q) (map[string][]string, error) {
	rows, err := q.Query(activeSQL, int64(e.now())-WakeReserveS)
	if err != nil {
		return nil, err
	}
	type row struct {
		agent, state, reserved string
		closed                 bool
	}
	var got []row
	for rows.Next() {
		var agent, state string
		var closedAt sql.NullInt64
		var reserved sql.NullString
		if err := rows.Scan(&agent, &state, &closedAt, &reserved); err != nil {
			rows.Close()
			return nil, err
		}
		got = append(got, row{agent, state, reserved.String, closedAt.Valid && closedAt.Int64 != 0})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	found := map[string][]string{}
	for _, r := range got {
		running := r.state == "RUNNING" && !r.closed
		acct := r.reserved
		if running || r.reserved == "" {
			a, err := team.CurrentAccount(q, r.agent)
			if err != nil {
				return nil, err
			}
			acct = a
		}
		if acct != "" {
			found[acct] = append(found[acct], r.agent)
		}
	}
	return found, nil
}

// ActiveAgents is load.active_agents: how many agents are mid-turn on each account, exclude left out.
func (e *Env) ActiveAgents(q db.Q, exclude string) (map[string]int, error) {
	by, err := e.ActiveByAccount(q)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for a, names := range by {
		n := 0
		for _, x := range names {
			if x != exclude {
				n++
			}
		}
		if n > 0 {
			out[a] = n
		}
	}
	return out, nil
}

// ReadAll is load.read_all: every account's load; exclude leaves one agent out of the counts and the burn, the one being placed.
func (e *Env) ReadAll(q db.Q, exclude string) (map[string]*Load, error) {
	by, err := e.ActiveByAccount(q)
	if err != nil {
		return nil, err
	}
	costs, err := e.LoadCosts(q)
	if err != nil {
		return nil, err
	}
	out := map[string]*Load{}
	for _, a := range paths.ListAccounts(e.Home) {
		on := without(by[a], exclude)
		l := &Load{Account: a, Active: len(on), Costs: costs}
		if b, ok := costs.Burn(a, on); ok {
			l.Burn = &b
		}
		out[a] = l
	}
	return out, nil
}

// ByKind is one line per kind among agents (sorted by model, then effort): what an agent of it spends per minute of work and what that
// rests on.
func (c *Costs) ByKind(agents []string) []string {
	type kind struct{ model, effort string }
	seen := map[kind]string{}
	for _, n := range agents {
		if c.kindOf == nil {
			break
		}
		if m, e, ok := c.kindOf(n); ok {
			if _, dup := seen[kind{m, e}]; !dup {
				seen[kind{m, e}] = n
			}
		}
	}
	kinds := make([]kind, 0, len(seen))
	for k := range seen {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if kinds[i].model != kinds[j].model {
			return kinds[i].model < kinds[j].model
		}
		return kinds[i].effort < kinds[j].effort
	})
	out := []string{}
	for _, k := range kinds {
		usd, source, count := c.USDPerMin(seen[k])
		basis := source
		if count > 0 {
			basis = fmt.Sprintf("%d samples, %s", count, source)
		}
		out = append(out, fmt.Sprintf("%s %s $%.3f/min (%s)", k.model, k.effort, usd, basis))
	}
	return out
}

package usage

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
)

// An account's factors: API-equivalent dollars per point of its 5-hour window and per point of its week, so a pace measured in dollars
// reads as points on any plan. Learned from stretches of live readings of one window over which the figure rose FactorMinRise points or
// more (one point of rounding is then a small error): the priced token_calls of the account in the stretch over the rise. Until an
// account has FactorOwnAfter stretches of its own it takes the pooled factor, learned from every account in Pro units and scaled by its
// plan.
const (
	FactorMinRise  = 5
	FactorOwnAfter = 5
	// FactorRecent is how many of the newest stretches a median is taken over.
	FactorRecent = 20
	// FactorWindowS is how far back the stretches are read.
	FactorWindowS = 7 * 86400
	// FactorEveryS: the daemon recomputes the factors this often.
	FactorEveryS = 600
	factorMeta   = "account_factors"
)

// The factor's sources.
const (
	FactorOwn    = "own"
	FactorPooled = "pooled"
	FactorNone   = "none"
)

// WindowReading is one live reading of a window: when it was observed, its percent, and when the window resets.
type WindowReading struct {
	At, Reset int64
	Pct       int
}

// PointStretch is a span of one window between two live readings, Rise points apart.
type PointStretch struct {
	From, To int64
	Rise     int
}

// PointStretches cuts one window's live readings (any order) into back-to-back spans that each rose at least minRise points. A reset (the
// resets_at moved by more than a minute, which a poll's rounding never does) or a fall starts over at that reading.
func PointStretches(readings []WindowReading, minRise int) []PointStretch {
	rs := append([]WindowReading(nil), readings...)
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].At < rs[j].At })
	var out []PointStretch
	for i := 0; i < len(rs); {
		start, prev, j := rs[i], rs[i], i+1
		for ; j < len(rs); j++ {
			r := rs[j]
			if r.At == prev.At {
				continue
			}
			if r.Reset-start.Reset > 60 || start.Reset-r.Reset > 60 || r.Pct < prev.Pct {
				break
			}
			prev = r
			if r.Pct-start.Pct >= minRise {
				out = append(out, PointStretch{From: start.At, To: r.At, Rise: r.Pct - start.Pct})
				start = r
			}
		}
		i = j
	}
	return out
}

// Factor is one window's dollars per point for an account, where it came from, and how many stretches the account has of its own.
type Factor struct {
	USDPerPct float64 `json:"usd_per_pct"`
	Source    string  `json:"source"`
	Samples   int     `json:"samples"`
}

// AccountFactors are an account's two factors and the plan scale its pooled ones were scaled by.
type AccountFactors struct {
	Short Factor  `json:"short"`
	Long  Factor  `json:"long"`
	Scale float64 `json:"plan_scale"`
}

// PlanScale is a plan's size relative to Pro: Max 5x is 5, Max 20x 20. The rate-limit tier tells the two Max plans apart; a Max with no
// tier is taken as 5x.
func PlanScale(subscription, tier string) float64 {
	switch {
	case strings.Contains(tier, "max_20x"):
		return 20
	case strings.Contains(tier, "max_5x"), strings.Contains(subscription, "max"):
		return 5
	}
	return 1
}

// FactorsOf are every account's factors from its stretches' dollars-per-point values (newest last) and plan scales. An account with
// FactorOwnAfter of its own takes their recent median; else the pooled median (every account's values in Pro units) times its scale.
func FactorsOf(short, long map[string][]float64, scale map[string]float64) map[string]AccountFactors {
	one := func(per map[string][]float64, acct string) Factor {
		own := per[acct]
		if len(own) >= FactorOwnAfter {
			return Factor{USDPerPct: median(recent(own)), Source: FactorOwn, Samples: len(own)}
		}
		var pool []float64
		for a, vs := range per {
			for _, v := range recent(vs) {
				pool = append(pool, v/scaleOf(scale, a))
			}
		}
		if len(pool) == 0 {
			return Factor{Source: FactorNone, Samples: len(own)}
		}
		return Factor{USDPerPct: median(pool) * scaleOf(scale, acct), Source: FactorPooled, Samples: len(own)}
	}
	out := map[string]AccountFactors{}
	for acct := range scale {
		out[acct] = AccountFactors{Short: one(short, acct), Long: one(long, acct), Scale: scaleOf(scale, acct)}
	}
	return out
}

func recent(vs []float64) []float64 { return vs[max(0, len(vs)-FactorRecent):] }

func scaleOf(scale map[string]float64, acct string) float64 {
	if s := scale[acct]; s > 0 {
		return s
	}
	return 1
}

// readingsOf are each account's live readings of its shortest and longest windows from usage_samples rows.
func readingsOf(rows []sampleRow) (short, long map[string][]WindowReading) {
	short, long = map[string][]WindowReading{}, map[string][]WindowReading{}
	for _, r := range rows {
		acct, _ := r["account"].(string)
		es := entries(r)
		if acct == "" || len(es) == 0 {
			continue
		}
		add := func(into map[string][]WindowReading, w map[string]any) {
			pct, ok1 := num(w["used_percent"])
			at, ok2 := num(w["observed_at"])
			reset, ok3 := num(w["resets_at"])
			if !ok1 || !ok2 || !ok3 || at == 0 || w["readable"] == false || truthy(w["carried"]) || truthy(w["lost"]) {
				return
			}
			into[acct] = append(into[acct], WindowReading{At: int64(at), Reset: int64(reset), Pct: int(pct)})
		}
		add(short, es[0])
		if len(es) > 1 {
			add(long, es[len(es)-1])
		}
	}
	return short, long
}

// dollarsIn is the account's priced token_calls in (lo, hi].
func dollarsIn(q db.Q, acct string, lo, hi int64, table []priceEntry) (float64, error) {
	by, err := priced(q, table, "SELECT "+pricedCols+" FROM token_calls WHERE account = ? AND ts > ? AND ts <= ?", acct, lo, hi)
	total := 0.0
	for _, calls := range by {
		for _, c := range calls {
			total += c.USD
		}
	}
	return total, err
}

// perPoint is each account's dollars per point over its stretches, oldest first; a stretch with no priced calls (none recorded on this
// machine) says nothing and is left out.
func perPoint(q db.Q, readings map[string][]WindowReading, table []priceEntry) (map[string][]float64, error) {
	out := map[string][]float64{}
	for acct, rs := range readings {
		for _, s := range PointStretches(rs, FactorMinRise) {
			usd, err := dollarsIn(q, acct, s.From, s.To, table)
			if err != nil {
				return nil, err
			}
			if usd > 0 {
				out[acct] = append(out[acct], usd/float64(s.Rise))
			}
		}
	}
	return out, nil
}

// planScale is the account's plan scale from its recorded plan.
func (e *Env) planScale(acct string) float64 {
	sub, tier := team.Accounts{Home: e.Home}.RecordedPlan(acct)
	return PlanScale(sub, tier)
}

// RefreshFactors recomputes every account's factors from the last FactorWindowS of samples and keeps them in the meta table, at most
// every FactorEveryS unless forced.
func (e *Env) RefreshFactors(q db.Q, now int64, force bool) error {
	raw, _, err := db.Meta(q, factorMeta)
	if err != nil {
		return err
	}
	var kept struct {
		At int64 `json:"at"`
	}
	if !force && raw != "" && json.Unmarshal([]byte(raw), &kept) == nil && now-kept.At < FactorEveryS {
		return nil
	}
	rows, err := readSamples(q, now-FactorWindowS)
	if err != nil {
		return err
	}
	shortR, longR := readingsOf(rows)
	table := e.prices()
	short, err := perPoint(q, shortR, table)
	if err != nil {
		return err
	}
	long, err := perPoint(q, longR, table)
	if err != nil {
		return err
	}
	scale := map[string]float64{}
	for acct := range shortR {
		scale[acct] = e.planScale(acct)
	}
	for acct := range longR {
		scale[acct] = e.planScale(acct)
	}
	b, err := json.Marshal(map[string]any{"at": now, "accounts": FactorsOf(short, long, scale)})
	if err != nil {
		return err
	}
	return db.SetMeta(q, factorMeta, string(b))
}

// Factors are the factors the daemon last kept, by account; an account it has no readings of is absent.
func Factors(q db.Q) (map[string]AccountFactors, error) {
	raw, _, err := db.Meta(q, factorMeta)
	if err != nil || raw == "" {
		return map[string]AccountFactors{}, err
	}
	var kept struct {
		Accounts map[string]AccountFactors `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(raw), &kept); err != nil || kept.Accounts == nil {
		return map[string]AccountFactors{}, nil
	}
	return kept.Accounts, nil
}

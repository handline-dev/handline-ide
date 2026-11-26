package usage

import (
	"encoding/json"
	"math"
	"strings"
)

// Extra usage is money an account pays on top of its plan in its billing month, to keep working past its limits. It never enters plan
// money (plan.go), and a reading at 100% stays a wall whether it is on or not.

// Extra is an account's extra usage as its last reading named it, amounts in major units of Currency (dollars, not cents).
type Extra struct {
	On       bool
	Cap      *float64 // nil: no monthly cap named
	Spent    *float64 // spent this billing month; nil: none named
	Currency string   // "": none named
}

// ExtraMaxAgeS: an extra usage figure read longer ago than this is unknown, so a poll that stopped does not count a past billing month's
// spend as this one's.
const ExtraMaxAgeS = 86400

// CurrentExtra is the status's extra usage while its reading is recent enough to name this billing month's; nil otherwise.
func (s *AccountStatus) CurrentExtra() *Extra {
	if age := s.AgeS(); age == nil || *age > ExtraMaxAgeS {
		return nil
	}
	return s.Extra
}

// Map is x as the status file, a usage sample and stats.outlook keep it; nil x is nil (unknown, never zero).
func (x *Extra) Map() any {
	if x == nil {
		return nil
	}
	var cur any
	if x.Currency != "" {
		cur = x.Currency
	}
	return map[string]any{"on": x.On, "cap": roundPtr(x.Cap), "spent": roundPtr(x.Spent), "currency": cur}
}

func roundPtr(p *float64) any {
	if p == nil {
		return nil
	}
	return math.Round(*p*100) / 100
}

// ExtraOf reads what Map wrote, as a decoded object or its JSON text; nil is unknown.
func ExtraOf(v any) *Extra {
	if s, ok := v.(string); ok {
		var d any
		if decodeNumbers([]byte(s), &d) != nil {
			return nil
		}
		v = d
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	x := &Extra{On: truthy(m["on"])}
	if f, ok := num(m["cap"]); ok {
		x.Cap = &f
	}
	if f, ok := num(m["spent"]); ok {
		x.Spent = &f
	}
	x.Currency, _ = m["currency"].(string)
	return x
}

// extraJSON is x as a usage_samples column: NULL when unknown.
func extraJSON(x *Extra) any {
	if x == nil {
		return nil
	}
	b, err := json.Marshal(x.Map())
	if err != nil {
		return nil
	}
	return string(b)
}

// ExtraSpent is the accounts' extra usage spent this billing month, per currency: of the accounts whose reading names an amount.
func ExtraSpent(bs []AccountBudget) map[string]float64 {
	out := map[string]float64{}
	for _, b := range bs {
		if b.Extra != nil && b.Extra.Spent != nil {
			out[currencyOf(b.Extra)] += *b.Extra.Spent
		}
	}
	return out
}

func currencyOf(x *Extra) string {
	if c := strings.TrimSpace(x.Currency); c != "" {
		return strings.ToUpper(c)
	}
	return "USD"
}

// ExtraDaily is the extra usage spent per local day of (dayLo, hi] and currency, from rows oldest first: the rise of an account's spent
// between two of its readings, dated by the later one. A drop is its billing month starting over, so the later amount was spent from 0.
func ExtraDaily(rows []sampleRow, dayLo, hi int64) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	prev := map[string]*Extra{}
	for _, r := range rows {
		x := ExtraOf(r["extra"])
		if x == nil || x.Spent == nil {
			continue
		}
		a, _ := r["account"].(string)
		p := prev[a]
		prev[a] = x
		ts, _ := num(r["ts"])
		if p == nil || int64(ts) <= dayLo || int64(ts) > hi {
			continue
		}
		up := *x.Spent
		if currencyOf(p) == currencyOf(x) && *x.Spent >= *p.Spent {
			up = *x.Spent - *p.Spent
		}
		if up <= 0 {
			continue
		}
		day := dayOf(int64(ts))
		if out[day] == nil {
			out[day] = map[string]float64{}
		}
		out[day][currencyOf(x)] += up
	}
	return out
}

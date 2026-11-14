package usage

import (
	"math"

	"github.com/handline/handline/go/data/db"
)

// An account's weekly pace: a token bucket on the week, refilled linearly (the week's quota is spent as the week passes) with a 10% burst
// on top. The tokens an account has order the accounts a move picks from; the pace line (PaceOf) is shown, and holds nothing.
const (
	WeekS     = 7 * 86400
	PaceBurst = 0.10 // the share of the week's quota that may be spent ahead of the clock
)

// Headroom is the tokens in the account's tightest bucket, the 5h and the weekly window each judged against its own fill rate: how far
// the window has run, plus the burst, less what is spent, as a share of its quota. Positive is room; zero or less is ahead of the window.
// ok is false when neither figure can be acted on (unread, stale beyond carrying, or rolled over), in which case the pace says nothing.
func Headroom(st *AccountStatus, now int64) (float64, bool) {
	most, ok := math.Inf(1), false
	bucket := func(used *int, reset int64, length float64) {
		if used == nil || reset == 0 || length <= 0 {
			return
		}
		elapsed := 1 - math.Min(1, math.Max(0, float64(reset-now)/length))
		most, ok = math.Min(most, elapsed+PaceBurst-float64(*used)/100), true
	}
	bucket(st.ShortTrusted(), st.Short.Reset, float64(st.Short.Minutes*60))
	bucket(st.LongTrusted(), st.Long.Reset, WeekS)
	if !ok {
		return 0, false
	}
	return most, true
}

// FiveHourShare is how many times over the account's shortest window lasts to its reset at its burn (percent of that window per second)
// plus extra, one more agent's: 1 or more outlasts the reset, less hits the wall first. ok false where nothing trusted says so (a stale or
// unread figure, no reset), which holds and moves nothing: a start is never held on a guess.
func FiveHourShare(st *AccountStatus, burn, extra, now float64) (share float64, ok bool) {
	used, resetIn := st.ShortTrusted(), float64(st.Short.Reset)-now
	if used == nil || st.Short.Reset == 0 || resetIn <= 0 {
		return 1, false
	}
	if rate := burn + extra; rate > 0 {
		return float64(max(0, ShortBlock-*used)) / rate / resetIn, true
	}
	return 1, true
}

// WeekShare is how many times over the week's room lasts to the week's end at burn (percent of the 5-hour window per second) turned into
// week points by longPerShort: 1 or more outlasts it. ok false where the week, its end or the ratio is unknown; 1 with no burn.
func WeekShare(st *AccountStatus, burn, longPerShort float64, haveRatio bool, now float64) (float64, bool) {
	left, ok := WeekEndsIn(st, now)
	used := st.LongTrusted()
	if !ok || !haveRatio || used == nil || left <= 0 {
		return 1, false
	}
	if rate := burn * longPerShort; rate > 0 {
		return float64(max(0, LongBlock-*used)) / rate / left, true
	}
	return 1, true
}

// The pace line's config.yaml keys and defaults (settings.set): where the line ends in the week, and how far ahead of it is "ahead".
const (
	PaceTargetKey     = "pace_target_pct"
	PaceBurstKey      = "pace_burst_pct"
	DefaultPaceTarget = 98
	DefaultPaceBurst  = 10
	// MoveBeforeKey is the percent of the 5-hour window at which an idle agent with work waiting moves to an account with room before it
	// reaches the wall (tick.earlyTrigger), MoveBeforeDefault when unset; its week moves at states.MoveBeforeWeekPct. 0 moves only at the wall.
	MoveBeforeKey     = "usage_move_before_pct"
	MoveBeforeDefault = 90
	// SplitKey is how agents are spread over accounts: SplitAuto, the default, places and moves them; SplitManual never does, and an
	// agent stays on the account it is on and waits at its wall (a person's move still goes).
	SplitKey    = "account_split"
	SplitAuto   = "auto"
	SplitManual = "manual"
)

// MoveBeforeOf is usage_move_before_pct from config, a whole percent 0 to 100, MoveBeforeDefault when unset or not one.
func MoveBeforeOf(cfg Config) int {
	if n, ok := wholeIn(cfg[MoveBeforeKey], 0, 100); ok {
		return n
	}
	return MoveBeforeDefault
}

// ManualSplit is whether config says account_split manual.
func ManualSplit(cfg Config) bool { return cfg[SplitKey] == SplitManual }

// SplitOf is account_split as config says it, SplitAuto when unset or unknown.
func SplitOf(cfg Config) string {
	if ManualSplit(cfg) {
		return SplitManual
	}
	return SplitAuto
}

// PaceSettings is the pace line: it ends at Target percent of the week; Burst is how many points ahead of it is still on it. Neither
// holds a start.
type PaceSettings struct {
	Target, Burst int
}

// PaceSettingsOf reads them from config, each at its default when unset or out of range.
func PaceSettingsOf(cfg Config) PaceSettings {
	ps := PaceSettings{Target: DefaultPaceTarget, Burst: DefaultPaceBurst}
	if n, ok := wholeIn(cfg[PaceTargetKey], 50, 100); ok {
		ps.Target = n
	}
	if n, ok := wholeIn(cfg[PaceBurstKey], 0, 50); ok {
		ps.Burst = n
	}
	return ps
}

func wholeIn(v any, lo, hi int) (int, bool) {
	var n int
	switch x := v.(type) {
	case int:
		n = x
	case int64:
		n = int(x)
	default:
		return 0, false
	}
	return n, n >= lo && n <= hi
}

// The pace states (Account.pace.state).
const (
	PaceBehind = "behind"
	PaceAhead  = "ahead"
	PaceOver   = "over"
)

// Pace is an account's weekly use against its pace line (Account.pace).
type Pace struct {
	State string
	Line  float64
	Used  int
	RateH float64
}

// PaceOf is the account's pace now, judged on its last weekly reading, which holds until its reset even when stale; nil with no reading
// or no reset. The week runs from its reset less 7 days to its end: the reset, or the subscription end when that comes first. The line
// is Target times the share of that week passed.
func PaceOf(st *AccountStatus, now float64, ps PaceSettings) *Pace {
	used := st.Long.Pct
	if used == nil || st.Long.Reset == 0 || float64(st.Long.Reset) <= now {
		return nil
	}
	start := float64(st.Long.Reset - WeekS)
	left, _ := WeekEndsIn(st, now)
	end := now + left
	if end <= start {
		return nil
	}
	target := float64(ps.Target)
	p := &Pace{Used: *used, Line: target * math.Min(1, math.Max(0, (now-start)/(end-start)))}
	if hours := left / 3600; hours > 0 && float64(*used) < target {
		p.RateH = (target - float64(*used)) / hours
	}
	ahead := float64(*used) - p.Line
	switch {
	case ahead <= 0:
		p.State = PaceBehind
	case ahead <= float64(ps.Burst):
		p.State = PaceAhead
	default:
		p.State = PaceOver
	}
	return p
}

// Prediction is whether an account's windows outlast their resets with one more agent on it: how many times over each lasts to its reset
// (FiveHourShare, WeekShare), at Burn, the cost of the agents mid-turn on it, plus Extra, the added agent's (percent of the 5-hour
// window per second). A window nothing trusted judges (an unread or carried figure, no reset, no factor, an unmeasured cost) is not
// Known, and holds nothing.
type Prediction struct {
	Short, Long           float64
	ShortKnown, LongKnown bool
	Burn, Extra           float64
}

// Share is the shorter-lasting known window's share, ok false when neither is known: below 1, the start runs a window out first.
func (p Prediction) Share() (float64, bool) {
	switch {
	case p.ShortKnown && p.LongKnown:
		return math.Min(p.Short, p.Long), true
	case p.ShortKnown:
		return p.Short, true
	case p.LongKnown:
		return p.Long, true
	}
	return 1, false
}

// Predict is acct's Prediction with one more agent of agent's kind: the agents mid-turn on it at their models' dollar pace over the
// account's factors, plus agent's. A launch leaves agent out of those mid-turn (it is the one being added); a subagent's parent stays in
// (exclude ""). Launches and the subagent guard both judge a start by it.
func (e *Env) Predict(q db.Q, acct, agent, exclude string) Prediction {
	p := Prediction{Short: 1, Long: 1}
	costs, err := e.LoadCosts(q)
	if err != nil {
		return p
	}
	by, err := e.ActiveByAccount(q)
	if err != nil {
		return p
	}
	burn, ok := costs.Burn(acct, without(by[acct], exclude))
	extra, measured := costs.Measured(acct, agent)
	if !ok || !measured {
		return p
	}
	st, now := e.Read(acct), e.now()
	p.Burn, p.Extra = burn, extra
	p.Short, p.ShortKnown = FiveHourShare(st, burn, extra, now)
	ratio, haveRatio := costs.LongPerShort(acct)
	p.Long, p.LongKnown = WeekShare(st, burn+extra, ratio, haveRatio, now)
	return p
}

// PaceNow is the account's pace now under config's settings.
func (e *Env) PaceNow(acct string) *Pace {
	return PaceOf(e.Read(acct), e.now(), PaceSettingsOf(e.config()))
}

// Headroom is the account's headroom now, for ordering accounts by pace: the larger, the slower-paced.
func (e *Env) Headroom(acct string) (float64, bool) { return Headroom(e.Read(acct), int64(e.now())) }

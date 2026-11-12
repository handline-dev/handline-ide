package usage

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/handline/handline/go/agentctl/verdict"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/timefmt"
)

// The runway and the pick (handline/usage/status.py): how long an account's headroom lasts, and which account takes the next agent.

var posInf = math.Inf(1)

const (
	// PaceHysteresis: a pace this close to another's is noise, not a real difference in how far behind two accounts are; ties inside it
	// fall to account name, which never changes.
	PaceHysteresis = 0.02
	// LiveRefreshMaxPerCall caps how many candidates one pick may ask fresh: one success per tick, mirrored as one attempt per call.
	LiveRefreshMaxPerCall = 1
)

// confidence breaks a tie between accounts whose deadline and headroom match: a reported 0% is a stronger claim than an inferred one.
var confidence = map[string]int{"live": 3, "carried": 2, "quiet": 1}

func spanF(secs float64) string {
	if math.IsInf(secs, 0) || secs > 1e12 {
		return "unbounded"
	}
	return timefmt.Span(int64(secs), false)
}

// NotFor is why no agent of vendor is ever placed on this account, whatever its room: it is another vendor's, its month cycle has
// ended, it is paused, or it cannot be read. "" when it may be.
func (e *Env) NotFor(acct, vendor string) string {
	if theirs := team.AccountVendor(e.Home, acct); theirs != vendor {
		return fmt.Sprintf("a %s account, not %s", theirs, vendor)
	}
	if why := e.EndedWhy(acct); why != "" {
		return why
	}
	if why := e.PausedWhy(acct); why != "" {
		return why
	}
	if !e.metered(acct) {
		return UnmeteredWhy
	}
	return ""
}

// PausedWhy is why a paused account takes no agent, "" while it is not paused.
func (e *Env) PausedWhy(acct string) string {
	cfg := e.config()
	if cfg.RowsUnreadable() {
		return "the account settings could not be read this tick - not picked"
	}
	if !cfg.Paused(acct) {
		return ""
	}
	return "paused - not picked; resume it in the app's settings for " + acct
}

// EndedWhy is cycle.ended_why: why the account is out for good now, "" while it is not.
func (e *Env) EndedWhy(acct string) string {
	cfg, now := e.config(), time.Unix(int64(e.now()), 0)
	end, ok := cfg.EndOf(acct)
	if !ok || now.Before(end) {
		return ""
	}
	return fmt.Sprintf("subscription ended %s (month cycle) - not picked; after resubscribing, set the new end day in the app's settings for %s",
		cfg.EndDate(acct), acct)
}

// Runway is an estimate of how long an account's headroom lasts.
type Runway struct {
	LeftShort int
	LeftLong  *int
	EtaS      *float64 // seconds to the wall at the current burn
	Burning   bool     // false: no burn reading
	PastReset bool     // the shortest window rolls over before the line is reached
	LongS     *float64 // how long the week lasts, when known and the week does not roll over first
	LongLabel string
}

// RunwayOf is status.runway: whichever window runs out first, the shortest at the current burn, the week by LongRunwayS. nil when the
// reading is stale or unknown: no figure is better than one drawn from a reading that stopped describing the account.
func RunwayOf(st *AccountStatus, burn *float64, now float64, ld *Load) *Runway {
	pct := st.ShortTrusted()
	if pct == nil {
		return nil
	}
	left := max(0, ShortBlock-*pct)
	var left7 *int
	if week := st.LongTrusted(); week != nil {
		l := max(0, LongBlock-*week)
		left7 = &l
	}
	longS := LongRunwayS(st, ld, now)
	if burn == nil || *burn <= 0 {
		return &Runway{LeftShort: left, LeftLong: left7, LongS: longS, LongLabel: st.Long.Label()}
	}
	eta := float64(left) / *burn
	var past bool
	switch {
	case st.Short.Reset != 0:
		past = now+eta > float64(st.Short.Reset)
	case st.Short.Minutes != 0:
		past = eta > float64(st.Short.Minutes*60)
	}
	return &Runway{LeftShort: left, LeftLong: left7, EtaS: &eta, Burning: true, PastReset: past, LongS: longS, LongLabel: st.Long.Label()}
}

// Describe is the runway in words.
func (r *Runway) Describe() string {
	if r == nil {
		return ""
	}
	if r.LeftShort == 0 {
		return "at the wall"
	}
	if r.LeftLong != nil && *r.LeftLong == 0 {
		return fmt.Sprintf("at the %s wall", r.LongLabel)
	}
	var five *float64
	if r.Burning && !r.PastReset {
		five = r.EtaS
	}
	if r.LongS != nil && (five == nil || *r.LongS < *five) {
		return fmt.Sprintf("~%s to %s", spanF(*r.LongS), r.LongLabel)
	}
	if !r.Burning {
		return "no recent burn"
	}
	if r.PastReset {
		return "outlasts reset"
	}
	return fmt.Sprintf("~%s to wall", spanF(*r.EtaS))
}

// WeekEndsIn is the seconds until the week stops mattering: its reset, or the end of the subscription when that comes first. ok false
// where the reset is unknown and no end is set.
func WeekEndsIn(st *AccountStatus, now float64) (float64, bool) {
	var ends []float64
	if st.Long.Reset != 0 {
		ends = append(ends, float64(st.Long.Reset))
	}
	if st.SubEnd != 0 {
		ends = append(ends, st.SubEnd)
	}
	if len(ends) == 0 {
		return 0, false
	}
	m := ends[0]
	for _, t := range ends[1:] {
		m = math.Min(m, t)
	}
	return m - now, true
}

// LongPace is this account's weekly usage against its weekly timer: its longest window's figure as a multiple of the usage a uniform
// pace through the week would have reached by now. Below 1 is behind that pace, the "use it or lose it" case. Infinity for a figure or
// a reset we do not know, or a week already rolled over: never sorted as low usage.
func LongPace(st *AccountStatus) float64 {
	week := st.LongTrusted()
	if week == nil || st.Long.Reset == 0 {
		return posInf
	}
	now := st.NowS()
	remaining := float64(st.Long.Reset) - now
	if remaining <= 0 {
		return posInf
	}
	length := float64(st.Long.Minutes * 60)
	if st.LastWeekFirst && st.SubEnd != 0 && now < st.SubEnd && st.SubEnd < float64(st.Long.Reset) {
		// The subscription ends inside this week, so the week is the shorter span from its start to that end, with all its allowance
		// still spendable: the same account sits further behind its pace, and goes first.
		length = st.SubEnd - (float64(st.Long.Reset) - length)
		remaining = st.SubEnd - now
	}
	elapsed := math.Max(length-remaining, 1.0) / length
	return float64(*week) / (elapsed * 100)
}

// AgentBurn is what one more agent costs, percent of the shortest window per second: the measured cost of agent's kind, a typical
// agent's when none is named, the starting value where nothing is measured.
func AgentBurn(ld *Load, agent string) float64 {
	if ld != nil && ld.Costs != nil {
		return ld.Costs.PerS(ld.Account, agent)
	}
	return TypicalAgentBurnPerS
}

// measuredBurn is what one more agent costs when samples say so (Costs.Measured); ok false for the starting value or no costs.
func measuredBurn(ld *Load, agent string) (float64, bool) {
	if ld == nil || ld.Costs == nil {
		return TypicalAgentBurnPerS, false
	}
	return ld.Costs.Measured(ld.Account, agent)
}

func loadBurn(ld *Load) float64 {
	if ld != nil && ld.Burn != nil {
		return *ld.Burn
	}
	return 0
}

// LongStretchS is how long the week's headroom lasts with one more agent on this account: the shortest window's arithmetic on the
// week, converted by the plan's measured ratio. ok false where the week or the ratio is unknown.
func LongStretchS(st *AccountStatus, ld *Load, agent string) (float64, bool) {
	week := st.LongTrusted()
	if week == nil || ld == nil || ld.Costs == nil {
		return 0, false
	}
	ratio, ok := ld.Costs.LongPerShort(ld.Account)
	if !ok {
		return 0, false
	}
	fill := loadBurn(ld) * ratio
	return StretchS(float64(LongBlock-*week), fill, AgentBurn(ld, agent)*ratio), true
}

// LongRunwayS is how long the week lasts, for the runway column: at the current fill when the account is filling; an idle account
// shows the week only where the pick shut it out on the week. nil where unknown, or the week rolls over first.
func LongRunwayS(st *AccountStatus, ld *Load, now float64) *float64 {
	week := st.LongTrusted()
	if week == nil || ld == nil || ld.Costs == nil {
		return nil
	}
	ratio, ok := ld.Costs.LongPerShort(ld.Account)
	if !ok {
		return nil
	}
	fill := loadBurn(ld) * ratio
	var lasts float64
	switch {
	case fill > 0:
		lasts = float64(LongBlock-*week) / fill
	case NoLongRoom(st, ld, "") != "":
		lasts, _ = LongStretchS(st, ld, "")
	default:
		return nil
	}
	resetIn, ok := WeekEndsIn(st, now)
	if !ok {
		resetIn = float64(st.Long.Minutes * 60)
	}
	if resetIn <= lasts {
		return nil
	}
	return &lasts
}

// NoLongRoom is why the week cannot carry one more agent: it would run out in less than a start is worth and before the week rolls
// over. "" when it can. A stale week reading is unknown here, so only the shortest window is judged.
func NoLongRoom(st *AccountStatus, ld *Load, agent string) string {
	lasts, ok := LongStretchS(st, ld, agent)
	if !ok || lasts >= MinStretchS {
		return ""
	}
	left, haveLeft := WeekEndsIn(st, st.NowS())
	if haveLeft && left <= lasts {
		return ""
	}
	ends := "resets"
	if st.SubEnd != 0 && (st.Long.Reset == 0 || st.SubEnd < float64(st.Long.Reset)) {
		ends = "ends"
	}
	reset := ""
	if haveLeft && left > 0 {
		reset = fmt.Sprintf(", %s in %s", ends, spanF(left))
	}
	return fmt.Sprintf("%s %d%%%s: one more agent lasts ~%s of it - under the %s a start is worth",
		st.Long.Label(), *st.LongTrusted(), reset, spanF(lasts), spanF(MinStretchS))
}

func perMin(rate float64, places int) string { return fmt.Sprintf("%.*f%%/min", places, rate*60) }

func stretchReason(st *AccountStatus, ld *Load, lasts float64, pct int, agent string) string {
	agents := ""
	if ld != nil {
		agents = fmt.Sprintf(", %d/%d agents", ld.Active, ld.Cap)
	}
	return fmt.Sprintf("%s %d%%, fill %s%s, one more agent (%s) lasts ~%s", st.Short.Label(), pct, perMin(loadBurn(ld), 1), agents,
		perMin(AgentBurn(ld, agent), 2), spanF(lasts))
}

// NoRoom is (why it cannot take one more agent, how long it would last): the cap, then, with the launch pace hold on, the week's stretch
// and the shortest window's: it must last to its reset with the newcomer (FiveHourShare), or a while where the reset is unknown, when the
// fill pct is known. The one test Unfit and the pick share; agent prices the newcomer by its own kind.
func NoRoom(st *AccountStatus, ld *Load, pct *int, agent string) (string, *float64) {
	if ld != nil && ld.Cap <= 0 {
		return fmt.Sprintf("takes no agents: its cap is 0 (%d mid-turn)", ld.Active), nil
	}
	if ld != nil && ld.AtCap() {
		return fmt.Sprintf("at its cap: %d of %d agents mid-turn", ld.Active, ld.Cap), nil
	}
	if ld == nil || !ld.Hold { // the launch pace hold is off: the stretch is shown, and holds nothing
		if pct == nil {
			return "", nil
		}
		lasts := StretchS(float64(ShortBlock-*pct), loadBurn(ld), AgentBurn(ld, agent))
		return "", &lasts
	}
	if weekly := NoLongRoom(st, ld, agent); weekly != "" {
		return weekly, nil
	}
	if pct == nil {
		return "", nil
	}
	lasts := StretchS(float64(ShortBlock-*pct), loadBurn(ld), AgentBurn(ld, agent))
	// With the reset known and the newcomer's cost measured, a start is worth it when the account lasts to the reset with it; without,
	// when it lasts a while.
	per, measured := measuredBurn(ld, agent)
	if share, ok := FiveHourShare(st, loadBurn(ld), per, st.NowS()); ok && measured {
		if share < 1 {
			return stretchReason(st, ld, lasts, *pct, agent) + " - short of its reset", &lasts
		}
	} else if lasts < MinStretchS {
		return stretchReason(st, ld, lasts, *pct, agent) + fmt.Sprintf(" - under the %s a start is worth", spanF(MinStretchS)), &lasts
	}
	return "", &lasts
}

// fitFacts are the two things Fit asks of what is known: a reading at the wall (a stale one too, while its window lasts), and no room
// on a fresh fill. A missing or stale reading is the account nobody has run on lately, and the session about to start writes a fresh one.
func fitFacts(st *AccountStatus, ld *Load, agent string) (wall, noRoom string) {
	pct, week := st.ShortTrusted(), st.LongTrusted()
	full := pct != nil && *pct >= ShortBlock
	// A stale reading at the wall is still a floor until its window has rolled over, but one older than the window cannot describe it.
	if !full && st.Short.Pct != nil && *st.Short.Pct >= ShortBlock {
		if st.Short.Reset != 0 {
			full = float64(st.Short.Reset) > st.NowS()
		} else {
			age := st.AgeS()
			full = age != nil && *age < float64(st.Short.Minutes*60)
		}
	}
	if full {
		return fmt.Sprintf("%s usage at the wall (%d%%)", st.Short.Label(), *st.Short.Pct), ""
	}
	if week != nil && *week >= LongBlock {
		return fmt.Sprintf("%s usage %d%% at the wall", st.Long.Label(), *week), ""
	}
	if pct != nil && !st.Stale() {
		why, _ := NoRoom(st, ld, pct, agent)
		return "", why
	}
	why, _ := NoRoom(st, ld, nil, agent)
	return "", why
}

// Unfit is why a session should not be started here, when what is known says so ("" otherwise): the first reason verdict.Admit
// gives against its Fits tier.
func Unfit(st *AccountStatus, ld *Load, agent string) string {
	wall, noRoom := fitFacts(st, ld, agent)
	a := verdict.Admit(verdict.Account{Assignable: true, Wall: wall, NoRoomFit: noRoom})
	if a.Fits {
		return ""
	}
	return a.Reasons[0]
}

// FitAdmission is verdict.Admit's Fits tier for one account: not this vendor's, then what is known of its reading. The pick's own
// tiers are left to Pick.
func (e *Env) FitAdmission(acct, vendor, agent string, ld *Load) verdict.Admission {
	wall, noRoom := fitFacts(e.Read(acct), ld, agent)
	return verdict.Admit(verdict.Account{NotFor: e.NotFor(acct, vendor), Assignable: true, Wall: wall, NoRoomFit: noRoom})
}

// AccountVerdict is whether a session may start on this account and the sentence for it. ranked is Pick's reasons for this account's
// own vendor; an account outside the ranking falls back to what TakesWork says.
func (e *Env) AccountVerdict(name string, st *AccountStatus, ld *Load, ranked []Reason, loggedIn bool) (bool, string) {
	if !loggedIn {
		return false, "not logged in"
	}
	theirs := team.AccountVendor(e.Home, name)
	if err := team.CheckVendor(theirs); err != nil {
		return false, err.Error()
	}
	if never := e.NotFor(name, theirs); never != "" {
		return false, never
	}
	active := 0
	if ld != nil {
		active = ld.Active
	}
	allowed, why := st.TakesWork(active)
	for _, r := range ranked {
		if r.Account == name {
			why = r.Why
			break
		}
	}
	return allowed && Unfit(st, ld, "") == "", why
}

// Reason is one account's line in a pick: why it was refused, or where it ranks. Kind is what refused it, "" for a candidate.
type Reason struct{ Account, Why, Kind string }

// The kinds of Reason.Kind.
const (
	KindEnded       = "subscription_ended"
	KindWeekCap     = "week_cap"
	KindFiveHourCap = "five_hour_cap"
	KindLogin       = "login"
	KindPaused      = "paused"
	KindHeld        = "held" // the agent cap or the launch pace hold
	KindOther       = "other"
)

// StaleEndNote is added to an ended account's reason while it is logged in and its usage reads fresh: the subscription was likely
// renewed and the end day not changed.
const StaleEndNote = " - but it is logged in and its usage reads fresh, so this end day may be out of date"

// outrightReason is the reason of an account refused before its room is weighed; an ended one notes an end day that may be stale.
func (e *Env) outrightReason(n string, facts verdict.Account, read func(string) *AccountStatus) Reason {
	why := verdict.Admit(facts).Reasons[0]
	if !facts.Removing && !facts.Excluded && facts.NotFor == e.PausedWhy(n) {
		return Reason{n, why, KindPaused}
	}
	if facts.Removing || facts.Excluded || facts.NotFor != e.EndedWhy(n) {
		return Reason{n, why, KindOther}
	}
	if in, known := e.loginState(n); known && in {
		if !read(n).Stale() {
			why += StaleEndNote
		}
	}
	return Reason{n, why, KindEnded}
}

func pickReason(st *AccountStatus, ld *Load, lasts *float64, picked bool, agent string) string {
	week := st.LongTrusted()
	weekS := "unknown"
	if week != nil {
		weekS = fmt.Sprintf("%d%%", *week)
	}
	tag := ""
	if picked {
		tag = " <- picked"
	}
	active := 0
	if ld != nil {
		active = ld.Active
	}
	if ok, why := st.RolledOver(); ok {
		return why + tag
	}
	if ok, why := st.IdleFloor(active); ok {
		return why + tag
	}
	when := "unknown"
	if st.Long.Reset != 0 {
		when = timefmt.Ago(int64(math.Max(0, float64(st.Long.Reset)-st.NowS())))
	}
	paceS := "unknown pace"
	if p := LongPace(st); !math.IsInf(p, 1) {
		paceS = fmt.Sprintf("%.2fx its weekly pace", p)
	}
	// Neither stale case above fired, so the account is live and its trusted figure is there; 0 stands for a fail-closed miss.
	trusted := 0
	if t := st.ShortTrusted(); t != nil {
		trusted = *t
	}
	var base string
	if lasts != nil {
		base = stretchReason(st, ld, *lasts, trusted, agent)
	} else {
		base = fmt.Sprintf("%s %d%%", st.Short.Label(), trusted)
	}
	return fmt.Sprintf("%s; %s %s (resets in %s, %s)", base, st.Long.Label(), weekS, when, paceS) + tag
}

// PickOpts are pick_account's keyword arguments.
type PickOpts struct {
	Candidates []string // nil: every account not known to be logged out
	// Bootstrap makes a logged-in account with no status file at all pickable as a last resort: nothing else could ever give it one.
	Bootstrap bool
	Exclude   map[string]string // accounts to skip and why
	Load      map[string]*Load  // nil reads it
	Agent     string
	Prefer    string // the account a repeated caller picked last time, so a hair of rounding does not swap the lead
	// ReadStatus replaces Read as the source of each candidate's status: a replay of a recorded sample.
	ReadStatus func(acct string) *AccountStatus
	// LiveRefresh asks one stale candidate fresh before it is refused or admitted on an old floor; only a caller about to place an
	// agent sets it, a display must not cost a network call.
	LiveRefresh bool
	Vendor      string // which vendor to pick for when no agent names one; Agent wins
}

// Pick is status.pick_account: among the accounts that can take one more agent, the one furthest behind its weekly pace goes first.
// Returns the choice ("" when none can, and the caller must refuse: the fail-closed path) and every account's reason, in candidate
// order.
func (e *Env) Pick(q db.Q, o PickOpts) (string, []Reason, error) {
	read := o.ReadStatus
	if read == nil {
		read = e.Read
	}
	names := o.Candidates
	if names == nil {
		for _, a := range paths.ListAccounts(e.Home) {
			if in, known := e.loginState(a); !known || in {
				names = append(names, a)
			}
		}
	}
	picking := o.Vendor
	if picking == "" {
		picking = team.DefaultVendor
	}
	if o.Agent != "" {
		cfg, err := e.agentConfig(o.Agent)
		if err != nil { // unreadable settings pick nowhere
			var out []Reason
			for _, n := range names {
				out = append(out, Reason{n, fmt.Sprintf("agent settings unreadable (%v)", err), KindOther})
			}
			return "", out, nil
		}
		picking, _, _ = team.Parts(cfg)
	}
	loads := o.Load
	if loads == nil {
		var err error
		if loads, err = e.ReadAll(q, ""); err != nil {
			return "", nil, err
		}
	}
	var reasons []Reason
	type cand struct {
		name  string
		st    *AccountStatus
		ld    *Load
		lasts *float64
	}
	var usable []cand
	var blank []string
	refreshLeft := LiveRefreshMaxPerCall
	for _, n := range names {
		why, excluded := o.Exclude[n]
		facts := verdict.Account{Removing: paths.Removing(e.Home, n), Excluded: excluded, ExcludeWhy: why, NotFor: e.NotFor(n, picking)}
		// Before anything about room: neither kind has any to weigh, and both would otherwise reach the bootstrap list.
		if facts.Outright() {
			reasons = append(reasons, e.outrightReason(n, facts, read))
			continue
		}
		st := read(n)
		ld := loads[n]
		active := 0
		if ld != nil {
			active = ld.Active
		}
		if in, known := e.loginState(n); o.LiveRefresh && refreshLeft > 0 && e.Refresh != nil && known && in {
			// The two figures the rest of this function treats specially - a carried_old reading (refused below) and a floor IdleFloor
			// would admit on (sized below) - are asked fresh once, in place, before either decision is made on the old one.
			floor, _ := st.IdleFloor(active)
			if st.ShortSource() == "carried_old" || (st.ShortTrusted() == nil && floor) {
				refreshLeft--
				if e.Refresh(n) {
					st = read(n)
				}
			}
		}
		var kind string
		facts.Assignable, facts.AssignWhy, kind = st.takesWork(active)
		facts.NoReading = !st.Exists
		var lasts *float64
		if facts.Assignable {
			// A stale account whose window has rolled over reads as a fresh one; a stale one still inside its window is judged on the
			// floor it was admitted on, not on a fill of 0, which would prefer the account we cannot see over one we can see has room.
			pct := 0
			if t := st.ShortTrusted(); t != nil {
				pct = *t
			} else if ok, _ := st.IdleFloor(active); ok {
				pct = *st.Short.Pct
			}
			var fill *int
			if rolled, _ := st.RolledOver(); !rolled {
				fill = &pct
			}
			facts.NoRoom, lasts = NoRoom(st, ld, fill, o.Agent)
		}
		switch a := verdict.Admit(facts); {
		case a.Best:
			usable = append(usable, cand{n, st, ld, lasts})
		case a.NeverRun && o.Bootstrap:
			blank = append(blank, n)
			reasons = append(reasons, Reason{n, a.Reasons[0] + " (bootstrap candidate, used only if nothing else qualifies)", ""})
		case a.NeverRun:
			reasons = append(reasons, Reason{n, facts.AssignWhy, KindOther})
		default:
			if facts.Assignable { // refused on its room: the cap or the pace hold
				kind = KindHeld
			}
			reasons = append(reasons, Reason{n, a.Reasons[0], kind})
		}
	}
	if len(usable) == 0 {
		if len(blank) > 0 {
			return blank[0], reasons, nil
		}
		return "", reasons, nil
	}
	type keyed struct {
		cand
		bucket   float64
		incumb   int
		headroom int
		conf     int
	}
	keys := make([]keyed, len(usable))
	for i, c := range usable {
		pace := LongPace(c.st)
		bucket := pace
		if !math.IsInf(pace, 1) {
			// A dead zone, not the raw figure; the name is the final tiebreak because it never moves.
			bucket = math.RoundToEven(pace/PaceHysteresis) * PaceHysteresis
			if o.Prefer == c.name {
				// A full step's head start: a challenger only unseats the last pick by clearing the dead zone outright, not by tying it.
				bucket -= PaceHysteresis
			}
		}
		incumb := 1
		if o.Prefer == c.name {
			incumb = 0
		}
		head := 0
		if h := c.st.ShortHeadroom(); h != nil {
			head = *h
		}
		keys[i] = keyed{c, bucket, incumb, -head, -confidence[c.st.ShortSource()]}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		switch {
		case a.bucket != b.bucket:
			return a.bucket < b.bucket
		case a.incumb != b.incumb:
			return a.incumb < b.incumb
		case a.headroom != b.headroom:
			return a.headroom < b.headroom
		case a.conf != b.conf:
			return a.conf < b.conf
		}
		return a.name < b.name
	})
	// The week that runs out soonest is spent first; the older key above orders what has no readable week.
	ranked := make([]Ranked, len(keys))
	for i, k := range keys {
		active := 0
		if k.ld != nil {
			active = k.ld.Active
		}
		u, known := Urgency(k.st)
		ranked[i] = Ranked{Name: k.name, Urgency: u, Known: known, Active: active}
	}
	if best, ok := BestByUrgency(ranked); ok {
		sort.SliceStable(keys, func(i, j int) bool { return keys[i].name == best.Name && keys[j].name != best.Name })
	}
	picked := keys[0].name
	for _, k := range keys {
		reasons = append(reasons, Reason{k.name, pickReason(k.st, k.ld, k.lasts, k.name == picked, o.Agent), ""})
	}
	order := map[string]int{}
	for i, n := range names {
		order[n] = i
	}
	sort.SliceStable(reasons, func(i, j int) bool { return order[reasons[i].Account] < order[reasons[j].Account] })
	return picked, reasons, nil
}

package usagepoll

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/timefmt"
)

// Reader is what a poll asks the CLI.
type Reader interface {
	Read(ctx context.Context, acct string) (r *usage.Reading, authFailed bool)
}

// Poller asks the accounts that are due for a reading, on each tick of the daemon's loop (daemon/usage_poll.poll_usage).
type Poller struct {
	DB    db.Q
	Env   *usage.Env
	Home  string
	Meter Reader
	// Recover earns a dead or ended account back with one real turn and records the reading itself (lifecycle.Recoverer.Recover).
	Recover  func(acct string) (ok bool, msg string)
	LoggedIn func(acct string) bool
	// Accounts lists the account names; nil is every directory under Home/accounts.
	Accounts func() []string
	// Now is unix seconds; nil is the wall clock.
	Now func() int64
	Log func(format string, args ...any)
}

func (p *Poller) logf(format string, args ...any) {
	if p.Log != nil {
		p.Log(format, args...)
	}
}

func (p *Poller) now() int64 {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now().Unix()
}

// Tick asks at most one account for a reading that succeeds. An account the founder asked to read (AskRead) goes first, then the oldest
// reading: with one success per tick a fixed order let the accounts early in the list take every slot whenever ticks ran slow, and the
// rest went unread and were refused work as stale. A failing account keeps its own cursor moving and is backed off exponentially, so it
// costs less of the tick the longer it keeps failing.
func (p *Poller) Tick(ctx context.Context) {
	now := p.now()
	accts := p.accounts()
	at := map[string]int64{}
	for _, a := range accts {
		if s := p.Env.Read(a).Short.At; s != nil {
			at[a] = *s
		}
	}
	asked := map[string]bool{}
	for _, a := range accts {
		asked[a] = p.asked(a)
	}
	sort.SliceStable(accts, func(i, j int) bool {
		if asked[accts[i]] != asked[accts[j]] {
			return asked[accts[i]]
		}
		return at[accts[i]] < at[accts[j]]
	})
	for _, a := range accts {
		if ctx.Err() != nil || p.pollAccount(ctx, a, now) {
			return
		}
	}
}

func (p *Poller) accounts() []string {
	if p.Accounts != nil {
		return append([]string(nil), p.Accounts()...)
	}
	return paths.ListAccounts(p.Home)
}

// Due is why this account's limits need asking about, "" when they do not (usage_due). The figures move when a turn is taken and at no
// other time, and a turn is something the hooks already record; an account nobody works on is asked only when its cadence comes round.
func (p *Poller) Due(acct string, st *usage.AccountStatus, now int64) string {
	if !st.Exists || st.Short.At == nil {
		return "no reading yet"
	}
	if age := now - *st.Short.At; age >= states.UsageMaxAgeS {
		return "last read " + timefmt.Ago(age) + " ago"
	}
	for _, w := range []usage.AcctWindow{st.Short, st.Long} {
		if w.Reset != 0 && w.Reset <= now {
			return "its " + w.Label() + " window has ended"
		}
	}
	// Both are whole seconds, so a turn inside the reading's second cannot be placed on either side of it: asked, not assumed, because
	// this gates handing out work.
	if last, ok, err := db.LastAccountActivity(p.DB, acct); err == nil && ok && last >= *st.Short.At {
		return "a turn since the last reading"
	}
	return ""
}

func (p *Poller) backoff(acct string, now int64) {
	var streak int64
	if v, _, err := db.Meta(p.DB, "usage-fail:"+acct); err == nil {
		streak, _ = strconv.ParseInt(v, 10, 64)
	}
	streak++
	wait := int64(states.UsagePollS)
	for i := int64(1); i < streak && wait < states.UsageFailBackoffCapS; i++ {
		wait *= 2
	}
	if wait > states.UsageFailBackoffCapS {
		wait = states.UsageFailBackoffCapS
	}
	_ = db.SetMeta(p.DB, "usage-fail:"+acct, strconv.FormatInt(streak, 10))
	_ = db.SetCursor(p.DB, "usage-poll:"+acct, now+wait)
	p.logf("  usage: %s: could not ask (next try in %ds)", acct, wait)
}

func (p *Poller) succeeded(acct string, now int64) {
	_ = db.ClearMeta(p.DB, "usage-fail:"+acct)
	_ = db.SetCursor(p.DB, "usage-poll:"+acct, now+states.UsagePollS)
}

// EndedWindow is the (label, reset) whose end is worth a real turn, ok false when none. A window the fresh reading still reports
// exhausted has not reset, whatever the older figure said, and a turn on it is the spend the wall exists to stop.
func EndedWindow(st *usage.AccountStatus, r *usage.Reading, now int64) (label string, reset int64, ok bool) {
	fresh := map[int]usage.ReadingWindow{}
	for _, w := range r.Windows {
		fresh[w.Minutes] = w
	}
	for _, held := range []usage.AcctWindow{st.Short, st.Long} {
		if held.Reset == 0 || held.Reset > now {
			continue
		}
		if w, has := fresh[held.Minutes]; has && w.Pct != nil && *w.Pct >= usage.ShortBlock && (w.Reset == 0 || w.Reset > now) {
			continue
		}
		return held.Label(), held.Reset, true
	}
	return "", 0, false
}

// readAskKey is the meta name of the founder's ask to read the account now; its value is when it was asked, for a reader of the table.
func readAskKey(acct string) string { return "usage-read-now:" + acct }

// AskRead is account.read_usage: read the account at the next tick. Asks before that read ends are one.
func AskRead(q db.Q, acct string, at int64) error {
	return db.SetMeta(q, readAskKey(acct), strconv.FormatInt(at, 10))
}

func (p *Poller) asked(acct string) bool {
	_, ok, err := db.Meta(p.DB, readAskKey(acct))
	return err == nil && ok
}

// answered drops the ask once its read ended, whatever it found.
func (p *Poller) answered(acct string) {
	if err := db.ClearMeta(p.DB, readAskKey(acct)); err != nil {
		p.logf("  usage: %s: could not clear the ask to read it: %v", acct, err)
	}
}

// pollAccount is one account's turn; true when it took the tick's one successful reading.
func (p *Poller) pollAccount(ctx context.Context, acct string, now int64) bool {
	asked := p.asked(acct)
	cur, err := db.Cursor(p.DB, "usage-poll:"+acct)
	backedOff := err != nil || now < cur
	if asked {
		// A read cut short by the daemon stopping is not an answer: the ask waits for the next start.
		defer func() {
			if ctx.Err() == nil {
				p.answered(acct)
			}
		}()
	} else if backedOff {
		return false
	}
	st := p.Env.Read(acct)
	if st.Unmetered {
		return false
	}
	why := "asked from the app"
	if !asked {
		why = p.Due(acct, st, now)
	}
	if why == "" {
		return false
	}
	if !p.LoggedIn(acct) {
		_ = db.SetCursor(p.DB, "usage-poll:"+acct, now+states.LoginCheckCooldownS)
		return false
	}
	// The CLI renews its own token when it starts, so there is no separate renewal step. A reading it cannot give is a dead token until
	// a real turn says otherwise (the turn is what makes the CLI renew).
	r, _ := p.Meter.Read(ctx, acct)
	if r == nil {
		// The real turn waits out the backoff, so pressing retry on a failing account does not spend one per press.
		if backedOff {
			p.backoff(acct, now)
			return false
		}
		ok, msg := p.Recover(acct)
		if ok {
			p.succeeded(acct, now)
			p.logf("  usage: %s", msg)
			return true
		}
		p.backoff(acct, now)
		return false
	}
	if label, reset, ended := EndedWindow(st, r, now); ended {
		key := "reset-ping:" + acct + ":" + label
		if v, _, _ := db.Meta(p.DB, key); v != strconv.FormatInt(reset, 10) {
			// A stale reading can still report success right up to the boundary, and nothing else forces the account to roll over: spend
			// the same real turn a dead token gets. A refusal is not a failure of this poll, the reading just fetched stands. Once per
			// reset, so a server that keeps naming the old one does not get a turn on every poll.
			if ok, msg := p.Recover(acct); ok {
				_ = db.SetMeta(p.DB, key, strconv.FormatInt(reset, 10))
				p.succeeded(acct, now)
				p.logf("  usage: %s", msg)
				return true
			}
		}
	}
	p.succeeded(acct, now)
	usage.Record(p.Home, *r)
	p.logf("  usage: %s: %s - %s", acct, why, Describe(r))
	return true
}

// Describe is the reading as one line for a log: each window by length, shortest first.
func Describe(r *usage.Reading) string {
	ws := append([]usage.ReadingWindow(nil), r.Windows...)
	sort.SliceStable(ws, func(i, j int) bool { return ws[i].Minutes < ws[j].Minutes })
	s := ""
	for i, w := range ws {
		if i > 0 {
			s += " "
		}
		if w.Pct == nil {
			s += fmt.Sprintf("%s ?", usage.WindowLabel(w.Minutes))
		} else {
			s += fmt.Sprintf("%s %d%%", usage.WindowLabel(w.Minutes), *w.Pct)
		}
	}
	return s
}

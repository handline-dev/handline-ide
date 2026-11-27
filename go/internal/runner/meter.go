package runner

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/states"
)

// Meter asks an account's CLI for its plan limits, and spends one real turn on it (usagepoll.Meter): only this machine holds the logins
// of its accounts, so only it can read them.
type Meter interface {
	Read(ctx context.Context, acct string) (r *usage.Reading, authFailed bool)
	RunModel(ctx context.Context, acct, prompt, model string) (ok bool, out, errType string)
}

// The one real turn that earns a refused login back, as the master spends it (lifecycle.RecoverPrompt, RecoverModel).
const (
	recoverPrompt = "hi"
	recoverModel  = "haiku"
)

// metered is what this runner knows of one account folder's limits, while it is logged in as email: its newest reading, whether its
// login was refused even after a real turn, and when to ask next, later after each failure in a row. Kept in memory: a runner has no
// board.
type metered struct {
	email       string
	reading     *usage.Reading
	loginFailed bool
	next        time.Time
	fails       int
}

type meters struct {
	sync.Mutex
	by map[string]*metered
}

// meter reads one due account every BeatEvery: the heartbeat after it carries the reading.
func (r *Runner) meter() {
	defer r.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-r.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	tick := time.NewTicker(BeatEvery)
	defer tick.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-tick.C:
		}
		r.MeterOne(ctx)
	}
}

// MeterOne asks the account whose reading is oldest of those due (none yet, or states.UsagePollS old, and not backed off) for its limits.
// A refused login gets one real turn, which makes the CLI renew its token, and is asked again; still refused, it is marked so until a
// reading comes.
func (r *Runner) MeterOne(ctx context.Context) {
	if r.cfg.Meter == nil {
		return
	}
	now := time.Now()
	r.meters.Lock()
	if r.meters.by == nil {
		r.meters.by = map[string]*metered{}
	}
	var due []string
	at := map[string]int64{}
	for _, a := range loggedInAccounts(r.cfg.Home) {
		m := r.meters.by[a.Name]
		if m == nil || m.email != a.Email { // logged in as another email: nothing known of it
			m = &metered{email: a.Email}
			r.meters.by[a.Name] = m
		}
		if !now.Before(m.next) {
			due = append(due, a.Name)
			if m.reading != nil {
				at[a.Name] = m.reading.At
			}
		}
	}
	r.meters.Unlock()
	if len(due) == 0 {
		return
	}
	sort.SliceStable(due, func(i, j int) bool { return at[due[i]] < at[due[j]] })
	acct := due[0]
	reading, refused := r.cfg.Meter.Read(ctx, acct)
	if reading == nil && refused {
		if ok, out, _ := r.cfg.Meter.RunModel(ctx, acct, recoverPrompt, recoverModel); ok {
			reading, refused = r.cfg.Meter.Read(ctx, acct)
		} else {
			r.cfg.Log("runner: usage: %s: the turn that renews its login did not run: %s", acct, out)
		}
	}
	if ctx.Err() != nil {
		return
	}
	now = time.Now() // the asking can take minutes
	r.meters.Lock()
	defer r.meters.Unlock()
	m := r.meters.by[acct]
	if reading != nil {
		m.reading, m.loginFailed, m.fails = reading, false, 0
		m.next = now.Add(states.UsagePollS * time.Second)
		return
	}
	m.loginFailed = m.loginFailed || refused
	m.fails++
	wait := time.Duration(states.UsagePollS) * time.Second
	for i := 1; i < m.fails && wait < states.UsageFailBackoffCapS*time.Second; i++ {
		wait *= 2
	}
	m.next = now.Add(min(wait, states.UsageFailBackoffCapS*time.Second))
	r.cfg.Log("runner: usage: %s: could not ask (next try in %s, login refused: %v)", acct, m.next.Sub(now).Round(time.Second), refused)
}

// reading is the account folder's newest reading while logged in as email, on the wire, and whether its login was refused.
func (r *Runner) reading(name, email string) (*BeatReading, bool) {
	r.meters.Lock()
	defer r.meters.Unlock()
	m := r.meters.by[name]
	if m == nil || m.email != email {
		return nil, false
	}
	if m.reading == nil {
		return nil, m.loginFailed
	}
	out := &BeatReading{AgeS: max(0, time.Now().Unix()-m.reading.At)}
	for _, w := range m.reading.Windows {
		out.Windows = append(out.Windows, BeatWindow{Minutes: w.Minutes, Pct: w.Pct, Reset: w.Reset})
	}
	if x := m.reading.Extra; x != nil {
		out.Extra = &BeatExtra{On: x.On, Cap: x.Cap, Spent: x.Spent, Currency: x.Currency}
	}
	return out, m.loginFailed
}

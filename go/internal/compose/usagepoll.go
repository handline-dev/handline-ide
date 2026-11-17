package compose

import (
	"context"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/accounts/usagepoll"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/scheduler/lifecycle"
)

// newPoller is the sweep that keeps every account's status.json reading fresh. ctx ends a question in flight when the loop stops.
func newPoller(ctx context.Context, cfg Config, uenv *usage.Env, q db.Q) *usagepoll.Poller {
	meter := &usagepoll.Meter{Home: cfg.Home, Claude: cfg.CLI, Env: cfg.Env, Now: cfg.Now}
	accounts := team.Accounts{Home: cfg.Home}
	rec := &lifecycle.Recoverer{Home: cfg.Home, Ports: recoverPorts{ctx: ctx, home: cfg.Home, meter: meter, env: uenv, accounts: accounts,
		q: q, now: nowOf(cfg.Now)}}
	return &usagepoll.Poller{DB: q, Env: uenv, Home: cfg.Home, Meter: meter, Recover: rec.Recover, LoggedIn: accounts.LoggedIn,
		Now: func() int64 { return nowOf(cfg.Now)().Unix() }, Log: cfg.Log}
}

// recoverPorts is lifecycle.RecoverPorts over the meter, the usage readings and the accounts.
type recoverPorts struct {
	ctx      context.Context
	home     string
	meter    *usagepoll.Meter
	env      *usage.Env
	accounts team.Accounts
	q        db.Q
	now      func() time.Time
}

var _ lifecycle.RecoverPorts = recoverPorts{}

func (r recoverPorts) UsageRecoverEnabled(acct string) bool {
	return r.env.CurrentConfig().RecoverEnabled(acct)
}

// RenewsByTurn: only the Claude CLI renews its token by taking a turn.
func (r recoverPorts) RenewsByTurn(acct string) (bool, string, error) {
	v := team.AccountVendor(r.home, acct)
	if err := team.CheckVendor(v); err != nil {
		return false, v, err
	}
	return v == team.DefaultVendor, v, nil
}

func (r recoverPorts) LoggedIn(acct string) bool { return r.accounts.LoggedIn(acct) }

func (r recoverPorts) LongTrusted(acct string) (int, string, bool) {
	st := r.env.Read(acct)
	pct := st.LongTrusted()
	if pct == nil {
		return 0, "", false
	}
	return *pct, st.Long.Label(), true
}

func (r recoverPorts) LongBlock() int { return usage.LongBlock }

func (r recoverPorts) RunModel(prompt, acct, model string) (bool, string, string) {
	return r.meter.RunModel(r.ctx, acct, prompt, model)
}

// Stall is the stall a failed agent turn raises (login.go), with no agent: the event is what tells the founder.
func (r recoverPorts) Stall(acct, errType, why string) error {
	first, err := r.accounts.StallAccount(acct, errType, why, r.now())
	if err != nil || !first {
		return err
	}
	_, err = db.AddEventFor(r.q, "", acct, "login_expired", db.KV{K: "account", V: acct}, db.KV{K: "error", V: errType}, db.KV{K: "why", V: why})
	return err
}

// ReadUsage returns an untyped nil for no reading: the port's Reading is an interface, and a nil *usage.Reading in it is not nil.
func (r recoverPorts) ReadUsage(acct string) (lifecycle.Reading, bool) {
	rd, authFailed := r.meter.Read(r.ctx, acct)
	if rd == nil {
		return nil, authFailed
	}
	return rd, authFailed
}

func (r recoverPorts) Record(reading lifecycle.Reading) {
	usage.Record(r.home, *reading.(*usage.Reading))
}

func (r recoverPorts) Describe(reading lifecycle.Reading) string {
	return usagepoll.Describe(reading.(*usage.Reading))
}

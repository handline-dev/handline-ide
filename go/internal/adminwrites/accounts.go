package adminwrites

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/accounts/usagepoll"
	"github.com/handline/handline/go/accounts/weblogin"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/textsafe"
	"github.com/handline/handline/go/protocol/rpc"
)

// account.set: an account's cap, alias, usage recovery, month cycle and pause, in its row; account.wake ends its stall. account.set
// writes what a retry writes again. settings.trust stays with the Python daemon; account.add and account.remove are in accountadmin.go.

const labelMax = 40

func capParam(p map[string]any) (n int64, given bool, err error) {
	if p["cap"] == nil {
		return 0, false, nil
	}
	n, ok := rpc.AsInt(p["cap"])
	if !ok || n < 1 {
		return 0, false, badParam("cap is a whole number, at least 1", "cap")
	}
	return n, true, nil
}

func flagParam(p map[string]any, key string) (v, given bool, err error) {
	if p[key] == nil {
		return false, false, nil
	}
	b, ok := p[key].(bool)
	if !ok {
		return false, false, badParam(key+" is true or false", key)
	}
	return b, true, nil
}

func (e *Env) accountSet(c *rpc.Conn, p map[string]any) (any, error) {
	acct, err := e.accountNamed(p)
	if err != nil {
		return nil, err
	}
	n, hasCap, err := capParam(p)
	if err != nil {
		return nil, err
	}
	recover, hasRecover, err := flagParam(p, "recover")
	if err != nil {
		return nil, err
	}
	cycleOn, hasCycle, err := flagParam(p, "cycle")
	if err != nil {
		return nil, err
	}
	paused, hasPaused, err := flagParam(p, "paused")
	if err != nil {
		return nil, err
	}
	label, hasLabel := p["label"].(string)
	if p["label"] != nil && (!hasLabel || textsafe.HasUnsafe(strings.TrimSpace(label)) || len([]rune(strings.TrimSpace(label))) > labelMax) {
		return nil, badParam(fmt.Sprintf("label is one plain line of at most %d characters", labelMax), "label")
	}
	label = strings.TrimSpace(label)
	endGiven := false
	var end string
	if v, in := p["cycle_end"]; in {
		endGiven = true
		if v != nil {
			s, ok := v.(string)
			if _, perr := time.Parse("2006-01-02", s); !ok || perr != nil {
				return nil, badParam("cycle_end is a date as YYYY-MM-DD, or null", "cycle_end")
			}
			end = s
		}
	}
	cfg, err := e.readAccount(acct)
	if err != nil {
		return nil, refused(err.Error(), "")
	}
	var said []string
	if hasLabel && label != currentLabel(cfg, acct) {
		if label != "" {
			cfg.set("label_"+acct, label)
			said = append(said, "alias "+label)
		} else {
			cfg.set("label_"+acct, nil)
			said = append(said, "alias cleared")
		}
	}
	if hasCap && !ownCapIs(cfg, acct, n) {
		cfg.set("active_cap_"+acct, n)
		said = append(said, fmt.Sprintf("%s: at most %d agents mid-turn", acct, n))
	}
	if hasRecover && recover != (cfg.get("usage_recover_"+acct) != false) {
		if recover {
			cfg.set("usage_recover_"+acct, nil)
		} else {
			cfg.set("usage_recover_"+acct, false)
		}
		said = append(said, "usage recovery "+map[bool]string{true: "on", false: "off"}[recover])
	}
	if endGiven {
		if end == "" {
			cfg.set("sub_end_"+acct, nil)
			said = append(said, acct+": end day cleared")
		} else {
			cfg.set("month_cycle_"+acct, true)
			cfg.set("sub_end_"+acct, end)
			said = append(said, fmt.Sprintf("%s: last day %s", acct, end))
		}
	}
	if hasCycle {
		if cycleOn && cfg.get("sub_end_"+acct) == nil {
			// A cycle with no end day never ends, so turning one on only means something with a day.
			return nil, refused("turning the month cycle on needs an end day: give cycle_end", "state")
		}
		cfg.set("month_cycle_"+acct, cycleOn)
		said = append(said, fmt.Sprintf("%s: month cycle %s", acct, map[bool]string{true: "on", false: "off"}[cycleOn]))
	}
	newPause := hasPaused && paused && cfg.get("paused_"+acct) != true
	if hasPaused && paused != (cfg.get("paused_"+acct) == true) {
		if paused {
			cfg.set("paused_"+acct, true)
			said = append(said, acct+": paused")
		} else {
			cfg.set("paused_"+acct, nil)
			said = append(said, acct+": resumed")
		}
	}
	if len(said) > 0 {
		if err := usage.SetAccount(e.DB, acct, cfg.changed); errors.Is(err, actor.ErrNoOwner) {
			return nil, refused("this install has no owner yet: start the daemon once", "state")
		} else if err != nil {
			return nil, refused(err.Error(), "")
		}
	}
	if newPause {
		// Each pause is news: the founder hears again if this one also leaves its agents nowhere to go.
		_ = db.ClearMeta(e.DB, db.NoRoomToldKey(acct))
	}
	out := strings.Join(said, "; ")
	if out == "" {
		out = "nothing changed"
	}
	return wrote(c, out, nil)
}

// accountWake is account.wake: a STALLED account's stall ends, so agents may be placed on it again and the agents stalled with it
// are released (compose liftLogin), as session.wake clears a stalled agent. One that is not stalled is refused.
func (e *Env) accountWake(c *rpc.Conn, p map[string]any) (any, error) {
	acct, err := e.accountNamed(p)
	if err != nil {
		return nil, err
	}
	accounts := team.Accounts{Home: e.Home}
	if accounts.Stalled(acct) == nil {
		return nil, refused(acct+": not STALLED", "state")
	}
	accounts.ClearStall(acct)
	// The failures before it say nothing of the account now (handoff.LastFailure).
	if _, err := db.AddEventFor(e.DB, c.Caller.Actor(), acct, "account_woken", db.KV{K: "account", V: acct}); err != nil {
		return nil, refused(err.Error(), "")
	}
	return wrote(c, acct+": STALLED -> usable", nil)
}

// account.read_usage asks the poller to read the account at its next tick (usagepoll.AskRead), so the one read per tick holds.
func (e *Env) accountReadUsage(c *rpc.Conn, p map[string]any) (any, error) {
	acct, err := e.accountNamed(p)
	if err != nil {
		return nil, err
	}
	if !usage.Metered(e.Home, acct) {
		return nil, refused(acct+": its vendor has no limits to read", "state")
	}
	if !(team.Accounts{Home: e.Home}).LoggedIn(acct) {
		return nil, refused(acct+": not logged in, or STALLED (account.wake)", "state")
	}
	if err := usagepoll.AskRead(e.DB, acct, time.Now().Unix()); err != nil {
		return nil, refused(err.Error(), "")
	}
	return wrote(c, acct+": usage is read at the next tick", nil)
}

// accountRow is account.set's view of one account's row: get and set by Config's key (usage.AccountKeys), cur and changed by column.
type accountRow struct {
	acct    string
	cur     usage.Config
	changed map[string]any
}

func (e *Env) readAccount(acct string) (*accountRow, error) {
	rows, err := usage.ReadAccountRows(e.DB)
	if err != nil {
		return nil, err
	}
	cur := rows[acct]
	if cur == nil {
		cur = map[string]any{}
	}
	return &accountRow{acct: acct, cur: cur, changed: map[string]any{}}, nil
}

// column is the accounts column a Config key of this account is held in.
func (r *accountRow) column(k string) string {
	acct, col := usage.AccountSetting(k)
	if acct != r.acct {
		panic("not a setting of " + r.acct + ": " + k)
	}
	return col
}

func (r *accountRow) get(k string) any { return r.cur[r.column(k)] }

func (r *accountRow) set(k string, v any) {
	col := r.column(k)
	if v == nil {
		delete(r.cur, col)
	} else {
		r.cur[col] = v
	}
	r.changed[col] = v
}

// currentLabel is the alias shown for the account, "" when it is shown under its own name.
func currentLabel(cfg *accountRow, acct string) string {
	if s, ok := cfg.get("label_" + acct).(string); ok && strings.TrimSpace(s) != "" {
		if s != acct {
			return s
		}
	}
	return ""
}

func ownCapIs(cfg *accountRow, acct string, n int64) bool {
	switch v := cfg.get("active_cap_" + acct).(type) {
	case int64:
		return v == n
	case int:
		return int64(v) == n
	}
	return false
}

// account.login starts the vendor's login as a child with no terminal and answers the URL it printed. The code comes later, by
// account.login_code; the end of the login is the accounts topic's. Nothing the child prints is kept but that URL, which is not logged.
func (e *Env) accountLogin(c *rpc.Conn, p map[string]any) (any, error) {
	acct, err := e.accountNamed(p)
	if err != nil {
		return nil, err
	}
	l, err := e.Logins.Start(acct)
	var no *weblogin.Refusal
	if errors.As(err, &no) {
		return nil, refused(no.Reason, "state")
	}
	if err != nil {
		return nil, refused("the login could not be started", "")
	}
	// The URL is not kept in the request record: the first answer carries it, and a retry's is refreshed from the login if it still waits.
	out, err := wrote(c, "login started on "+acct, map[string]any{"login_id": l.ID, "state": l.State})
	if m, ok := out.(map[string]any); ok && err == nil {
		m["url"] = l.URL
	}
	return out, err
}

// refreshLogin gives a retried account.login the login's URL while it still waits, and its state as it is now.
func (e *Env) refreshLogin(_ *rpc.Conn, reply map[string]any) error {
	id, _ := reply["login_id"].(string)
	if l, ok := e.Logins.ByID(id); ok {
		reply["state"] = l.State
		if l.State == weblogin.Waiting {
			reply["url"] = l.URL
		}
	}
	return nil
}

// account.login_cancel kills the login's process group. Cancelling a login that ended is accepted.
func (e *Env) accountLoginCancel(c *rpc.Conn, p map[string]any) (any, error) {
	id, ok := p["login_id"].(string)
	if !ok || id == "" {
		return nil, badParam("login_id is a login's id", "login_id")
	}
	if err := e.Logins.Cancel(id); err != nil {
		return nil, notFound("login", id, "no such login: "+id)
	}
	return wrote(c, "login cancelled", nil)
}

// account.login_code hands the one-time code to the login's stdin. The code is write-only: the method is Unkept, so no reply or params are
// stored for a retry, and the answer, the event and every error leave it out.
func (e *Env) accountLoginCode(_ *rpc.Conn, p map[string]any) (any, error) {
	id, ok := p["login_id"].(string)
	if !ok || id == "" {
		return nil, badParam("login_id is a login's id", "login_id")
	}
	code, ok := p["code"].(string)
	if !ok {
		return nil, badParam("code is a string", "code")
	}
	err := e.Logins.SendCode(id, code)
	var no *weblogin.Refusal
	switch {
	case errors.Is(err, weblogin.ErrUnknown):
		return nil, notFound("login", id, "no such login: "+id)
	case errors.Is(err, weblogin.ErrBadCode):
		return nil, badParam("code is one line of text", "code")
	case errors.As(err, &no):
		return nil, refused(no.Reason, "state")
	case err != nil:
		return nil, refused("the code could not be handed to the login", "")
	}
	return map[string]any{"accepted": true, "said": "code handed to the login"}, nil
}

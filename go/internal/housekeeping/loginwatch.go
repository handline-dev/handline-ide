package housekeeping

import (
	"strings"
	"time"

	"github.com/handline/handline/go/accounts/usage"
	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
)

// LoginCheckEveryS is how often the logins are looked at: asking takes the machine's keychain, which what reads the warning must not.
const LoginCheckEveryS = 60

// LoginWatch keeps a db.LoginNeed for each account that wants a login or stands stalled, but one past its last day, which is over.
type LoginWatch struct {
	Home string
	// State is the login check; nil asks the account's vendor.
	State   func(acct string) (loggedIn, known bool)
	checked int64
}

// Check writes only what changed, so an unchanged login tells no one.
func (w *LoginWatch) Check(q db.Q, now int64) error {
	if now-w.checked < LoginCheckEveryS {
		return nil
	}
	w.checked = now
	accounts := team.Accounts{Home: w.Home}
	state := w.State
	if state == nil {
		state = accounts.LoginState
	}
	cfg := usage.LoadConfig(w.Home)
	today := time.Unix(now, 0).Format("2006-01-02")
	wanted := map[string]bool{}
	for _, a := range paths.ListAccounts(w.Home) {
		if in, known := state(a); !known || in {
			continue
		}
		if end := cfg.EndDate(a); end != "" && end < today {
			continue
		}
		wanted[a] = true
		need := db.LoginNeed{Since: now}
		stall := accounts.Stalled(a)
		if stall != nil {
			need.Since = int64(stall.At)
			if word := team.StallWord(stall.Error); word != team.StallWord("") {
				need.Unusable = word
			}
		}
		if v, _, err := db.Meta(q, db.LoginNeedPrefix+a); err != nil {
			return err
		} else if kept, ok := db.ReadLoginNeed(v); ok {
			need.Since = kept.Since
		}
		if err := db.SetLoginNeed(q, a, need); err != nil {
			return err
		}
	}
	names, err := db.MetaNames(q, db.LoginNeedPrefix)
	if err != nil {
		return err
	}
	for _, name := range names {
		if !wanted[strings.TrimPrefix(name, db.LoginNeedPrefix)] {
			if err := db.ClearMeta(q, name); err != nil {
				return err
			}
		}
	}
	return nil
}

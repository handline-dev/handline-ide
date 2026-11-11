package usage

import (
	"database/sql"
	"strings"
	"time"

	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
)

// AccountKeys are the per-account settings, by the prefix Config keys them under (prefix + account name) and the accounts column
// that holds them. They come from the account's row, never from config.yaml.
var AccountKeys = []struct{ Prefix, Column string }{
	{"label_", "label"}, {"active_cap_", "active_cap"}, {"paused_", "paused"},
	{"usage_recover_", "usage_recover"}, {"month_cycle_", "month_cycle"}, {"sub_end_", "sub_end"},
}

var boolColumn = map[string]bool{"paused": true, "usage_recover": true, "month_cycle": true}

func accountKey(k string) bool {
	for _, a := range AccountKeys {
		if strings.HasPrefix(k, a.Prefix) {
			return true
		}
	}
	return false
}

// withAccountRows drops the per-account keys config.yaml may hold and puts in each account row's settings in their place.
func withAccountRows(home string, m Config) Config {
	for k := range m {
		if accountKey(k) {
			delete(m, k)
		}
	}
	d, err := store.SharedIfThere(paths.DBFile(home))
	if d == nil || err != nil {
		return m
	}
	rows, err := ReadAccountRows(d)
	if err != nil {
		return m
	}
	for name, set := range rows {
		for col, v := range set {
			for _, a := range AccountKeys {
				if a.Column == col {
					m[a.Prefix+name] = v
				}
			}
		}
	}
	return m
}

// ReadAccountRows is each account's settings that are not the default, by account name and column, as Config holds them: a bool
// column as a bool, an integer as int64, text as a string. A false paused is the default and left out.
func ReadAccountRows(q interface {
	Query(string, ...any) (*sql.Rows, error)
}) (map[string]map[string]any, error) {
	rows, err := q.Query("SELECT name, label, active_cap, paused, usage_recover, month_cycle, sub_end FROM accounts")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]any{}
	for rows.Next() {
		var name string
		var label, subEnd sql.NullString
		var capN, recover, cycle sql.NullInt64
		var paused bool
		if err := rows.Scan(&name, &label, &capN, &paused, &recover, &cycle, &subEnd); err != nil {
			return nil, err
		}
		set := map[string]any{}
		if label.Valid {
			set["label"] = label.String
		}
		if capN.Valid {
			set["active_cap"] = capN.Int64
		}
		if paused {
			set["paused"] = true
		}
		if recover.Valid {
			set["usage_recover"] = recover.Int64 != 0
		}
		if cycle.Valid {
			set["month_cycle"] = cycle.Int64 != 0
		}
		if subEnd.Valid {
			set["sub_end"] = subEnd.String
		}
		out[name] = set
	}
	return out, rows.Err()
}

// SetAccount writes the account's settings by column, a nil value back to the default, making its row (owned by the install's
// owner) when it has none. column is one of AccountKeys', never input text.
func SetAccount(q actor.Q, name string, set map[string]any) error {
	owner, err := actor.Owner(q)
	if err != nil {
		return err
	}
	if _, err := q.Exec("INSERT INTO accounts(name, owner, created_at) VALUES(?, ?, ?) ON CONFLICT(name) DO NOTHING",
		name, owner, time.Now().Unix()); err != nil {
		return err
	}
	for col, v := range set {
		if v == nil && col == "paused" {
			v = false
		}
		if b, ok := v.(bool); ok && boolColumn[col] {
			v = map[bool]int{true: 1, false: 0}[b]
		}
		if _, err := q.Exec("UPDATE accounts SET "+col+" = ? WHERE name = ?", v, name); err != nil {
			return err
		}
	}
	return nil
}

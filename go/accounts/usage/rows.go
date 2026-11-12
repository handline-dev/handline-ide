package usage

import (
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/handline/handline/go/data/db"
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

// AccountSetting is the account and column a per-account Config key is held in; acct is "" for any other key.
func AccountSetting(k string) (acct, col string) {
	for _, a := range AccountKeys {
		if strings.HasPrefix(k, a.Prefix) {
			return strings.TrimPrefix(k, a.Prefix), a.Column
		}
	}
	return "", ""
}

func prefixOf(col string) string {
	for _, a := range AccountKeys {
		if a.Column == col {
			return a.Prefix
		}
	}
	return ""
}

// withAccountRows drops the per-account keys config.yaml may hold and puts in each account row's settings in their place. A home
// with no database yet has no rows; a database that cannot be read is an error, with m holding no account's settings.
func withAccountRows(home string, m Config) (Config, error) {
	for k := range m {
		if acct, _ := AccountSetting(k); acct != "" {
			delete(m, k)
		}
	}
	d, err := store.SharedIfThere(paths.DBFile(home))
	if d == nil || err != nil {
		return m, err
	}
	rows, err := ReadAccountRows(d)
	if err != nil {
		return m, err
	}
	for name, set := range rows {
		for col, v := range set {
			m[prefixOf(col)+name] = v
		}
	}
	return m, nil
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

// SetAccount writes the account's settings by column (one of AccountKeys'), a nil value back to the default, in one statement that
// makes its row (owned by the install's owner) when it has none. A column or value of the wrong kind is an error and writes nothing.
func SetAccount(q actor.Q, name string, set map[string]any) error {
	cols, vals := []string{}, []any{}
	for col, v := range set {
		sv, err := columnValue(col, v)
		if err != nil {
			return fmt.Errorf("account %s: %w", name, err)
		}
		cols, vals = append(cols, col), append(vals, sv)
	}
	owner, err := actor.Owner(q)
	if err != nil {
		return err
	}
	update := "NOTHING"
	if len(cols) > 0 {
		sets := make([]string, len(cols))
		for i, c := range cols {
			sets[i] = c + " = excluded." + c
		}
		update = "UPDATE SET " + strings.Join(sets, ", ")
	}
	all := append([]string{"name", "owner", "created_at"}, cols...)
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(all)), ", ")
	_, err = q.Exec("INSERT INTO accounts("+strings.Join(all, ", ")+") VALUES("+marks+") ON CONFLICT(name) DO "+update,
		append([]any{name, owner, db.Now()}, vals...)...)
	return err
}

// columnValue is v as col stores it: text for label and sub_end, a whole number of at least 1 for active_cap, 0 or 1 for the flags;
// nil is NULL but for paused, whose default is 0.
func columnValue(col string, v any) (any, error) {
	if prefixOf(col) == "" {
		return nil, fmt.Errorf("no account setting %q", col)
	}
	if v == nil {
		if col == "paused" {
			return 0, nil
		}
		return nil, nil
	}
	switch col {
	case "label", "sub_end":
		if s, ok := v.(string); ok {
			return s, nil
		}
	case "active_cap":
		if f, ok := v.(float64); ok && f == math.Trunc(f) && f >= 1 && f <= math.MaxInt32 { // a number read from JSON
			v = int64(f)
		}
		if n, ok := posInt(v); ok {
			return n, nil
		}
	default:
		if b, ok := v.(bool); ok {
			return map[bool]int{true: 1, false: 0}[b], nil
		}
	}
	return nil, fmt.Errorf("%s cannot be %v (%T)", col, v, v)
}

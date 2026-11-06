package usage

import (
	"encoding/json"
	"strconv"

	"github.com/handline/handline/go/data/db"
)

// The usage_samples log: one row per account reading, which the account factors (factor.go) and the forecasts are read from.

const (
	// SegMinS: a stretch of work is measured over at least this long, so one call is a small part of it, and closed at SegMaxS so a run
	// that never pauses still yields samples (dollars.go).
	SegMinS = 600
	SegMaxS = 1800
	// KeepTotal samples are kept in all, the oldest dropped.
	KeepTotal = 300
	// RefreshEveryS: the daemon recomputes the dollar pace this often.
	RefreshEveryS = 600

	// UsageSampleRetentionDays is db.USAGE_SAMPLE_RETENTION_DAYS.
	UsageSampleRetentionDays = 14
)

func resetOf(w AcctWindow) *int64 {
	if w.Reset == 0 {
		return nil
	}
	r := w.Reset
	return &r
}

type sampleRow map[string]any

func readSamples(q db.Q, since int64) ([]sampleRow, error) {
	return querySamples(q, "SELECT * FROM usage_samples WHERE ts >= ? ORDER BY ts, id", since)
}

func querySamples(q db.Q, query string, args ...any) ([]sampleRow, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []sampleRow
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := sampleRow{}
		for i, c := range cols {
			switch v := vals[i].(type) {
			case int64:
				r[c] = json.Number(strconv.FormatInt(v, 10))
			case []byte:
				r[c] = string(v)
			default:
				r[c] = v
			}
		}
		if w, ok := r["windows"].(string); ok {
			r["windows"] = decodeWindows(w) // once here, not each time entries reads the row
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func truthy(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	f, ok := num(v)
	return ok && f != 0
}

// sampleWindow is one window of a usage_samples row, as windows.entry spells it.
type sampleWindow struct {
	Minutes    int    `json:"minutes"`
	UsedPct    *int   `json:"used_percent"`
	ResetsAt   *int64 `json:"resets_at"`
	ObservedAt *int64 `json:"observed_at"`
	Carried    bool   `json:"carried"`
	Lost       bool   `json:"lost"`
	Readable   *bool  `json:"readable,omitempty"`
}

// RecordSample is db.record_usage_sample: one row of what st actually held, at ts (0 is now). Every figure is written as st has it, an
// unknown one as NULL, never 0, so a reader reconstructs the status this was taken from. loggedOut is whether the account could not be
// used then (team.Accounts.GoneSince).
func (e *Env) RecordSample(q db.Q, acct string, st *AccountStatus, ts int64, loggedOut bool) error {
	if ts == 0 {
		ts = int64(e.now())
	}
	ws := []sampleWindow{}
	for _, w := range []AcctWindow{st.Short, st.Long} {
		if w.Minutes == 0 && w.Readable {
			continue
		}
		sw := sampleWindow{Minutes: w.Minutes, UsedPct: w.Pct, ResetsAt: resetOf(w), ObservedAt: w.At, Carried: w.Carried, Lost: w.Lost}
		if !w.Readable {
			f := false
			sw.Readable = &f
		}
		ws = append(ws, sw)
	}
	b, err := json.Marshal(ws)
	if err != nil {
		return err
	}
	var errText any
	if st.Error != "" {
		errText = st.Error
	}
	var collected any
	if st.CollectedAt != nil {
		collected = *st.CollectedAt
	}
	_, err = q.Exec("INSERT INTO usage_samples(account, ts, exists_, error, collected_at, windows, logged_out) VALUES(?,?,?,?,?,?,?)",
		acct, ts, st.Exists, errText, collected, string(b), loggedOut)
	return err
}

// PruneSamples is db.prune_usage_samples: delete samples older than the retention window and report how many.
func (e *Env) PruneSamples(q db.Q) (int64, error) {
	return exec(q, "DELETE FROM usage_samples WHERE ts < ?", int64(e.now())-UsageSampleRetentionDays*86400)
}

func exec(q db.Q, query string, args ...any) (int64, error) {
	res, err := q.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

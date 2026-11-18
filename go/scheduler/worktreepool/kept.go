package worktreepool

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/protected"
	"github.com/handline/handline/go/scheduler/worktreesetup"
)

// KeptNS is the protected namespace of how many desks each project keeps: one entry per repo, named by worktreesetup.Key.
const KeptNS = "desks_kept"

// The modes of Kept, and the most desks Manual may keep.
const (
	Auto      = "auto"
	Manual    = "manual"
	ManualMax = 50
)

// PruneHour is the local hour from which the nightly prune runs, once a night.
const PruneHour = 3

// UnusedS is how long a desk beyond the kept number has gone unused before the nightly prune removes it.
const UnusedS = 3 * 24 * 3600

// Kept is how many desks a project keeps: Auto (AutoKept), the default, or Manual with Value.
type Kept struct {
	Mode  string `json:"mode"`
	Value int    `json:"value,omitempty"`
}

// GetKept is repo's; none, or one that cannot be read, is Auto.
func GetKept(st *protected.Store, repo string) Kept {
	k := Kept{Mode: Auto}
	if v, ok := st.Get(KeptNS, worktreesetup.Key(repo)); ok && json.Unmarshal([]byte(v), &k) == nil && k.Mode == Manual {
		return k
	}
	return Kept{Mode: Auto}
}

// PutKept records repo's; Auto removes the entry.
func PutKept(st *protected.Store, repo string, k Kept) error {
	switch {
	case k.Mode == Auto:
		return st.Delete(KeptNS, worktreesetup.Key(repo))
	case k.Mode != Manual:
		return fmt.Errorf("mode is %s or %s", Auto, Manual)
	case k.Value < 1 || k.Value > ManualMax:
		return fmt.Errorf("value is 1 to %d desks", ManualMax)
	}
	b, err := json.Marshal(k)
	if err != nil {
		return err
	}
	return st.Put(KeptNS, worktreesetup.Key(repo), string(b))
}

// AutoKept is Auto's number for a project whose agents are agents: for each of the 7 whole local days before today on which the
// machine ran agents, the most of these agents whose sessions ran in one minute (load_minutes, which holds a week), averaged, rounded
// up, plus one. ok is false until load_minutes reaches back into the first of those days: nothing is pruned on a guess.
func AutoKept(q db.Q, agents []string, now time.Time) (n int, ok bool, err error) {
	y, m, d := now.Date()
	since, until := time.Date(y, m, d-7, 0, 0, 0, 0, now.Location()).Unix(), time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix()
	var first sql.NullInt64
	if err := q.QueryRow("SELECT MIN(minute) FROM load_minutes").Scan(&first); err != nil || !first.Valid || first.Int64 >= since+24*3600 {
		return 0, false, err
	}
	day := func(minute int64) string { return time.Unix(minute, 0).In(now.Location()).Format(time.DateOnly) }
	days := map[string]int{}
	ran, err := db.Collect[int64](q.Query("SELECT DISTINCT minute FROM load_minutes WHERE minute >= ? AND minute < ?", since, until))(func(r *sql.Rows) (m int64, err error) {
		return m, r.Scan(&m)
	})
	if err != nil {
		return 0, false, err
	}
	for _, m := range ran {
		days[day(m)] = 0
	}
	if len(days) == 0 {
		return 0, false, nil
	}
	if len(agents) > 0 {
		args := []any{since, until}
		for _, a := range agents {
			args = append(args, a)
		}
		type at struct {
			minute int64
			n      int
		}
		rows, err := db.Collect[at](q.Query("SELECT minute, COUNT(DISTINCT agent) FROM load_minutes WHERE minute >= ? AND minute < ? AND agent IN (?"+
			strings.Repeat(",?", len(agents)-1)+") GROUP BY minute", args...))(func(r *sql.Rows) (a at, err error) {
			return a, r.Scan(&a.minute, &a.n)
		})
		if err != nil {
			return 0, false, err
		}
		for _, r := range rows {
			days[day(r.minute)] = max(days[day(r.minute)], r.n)
		}
	}
	sum := 0
	for _, peak := range days {
		sum += peak
	}
	return int(math.Ceil(float64(sum)/float64(len(days)))) + 1, true, nil
}

// Agents are the keys of the agents whose workspace folder is repo: the ones that take its desks.
func Agents(q db.Q, repo string) ([]string, error) {
	type agent struct{ key, folder string }
	all, err := db.Collect[agent](q.Query("SELECT 'a' || a.id, w.folder FROM agents a JOIN workspaces w ON w.id = a.workspace WHERE w.folder != ''"))(
		func(r *sql.Rows) (a agent, err error) { return a, r.Scan(&a.key, &a.folder) })
	if err != nil {
		return nil, err
	}
	var out []string
	for _, a := range all {
		if a.folder == "~" || strings.HasPrefix(a.folder, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				a.folder = filepath.Join(home, a.folder[1:])
			}
		}
		if samePath(a.folder, repo) {
			out = append(out, a.key)
		}
	}
	return out, nil
}

// KeptNumber is how many desks repo keeps now: Manual's value, or AutoKept over its agents; ok is false when Auto has no week to go by.
func KeptNumber(q db.Q, st *protected.Store, repo string, now time.Time) (n int, ok bool, err error) {
	if k := GetKept(st, repo); k.Mode == Manual {
		return k.Value, true, nil
	}
	agents, err := Agents(q, repo)
	if err != nil {
		return 0, false, err
	}
	return AutoKept(q, agents, now)
}

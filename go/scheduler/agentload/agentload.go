// Package agentload is each agent's load over its last 7 days, found and never set: how long its heavy commands ran (heavy_runs) and
// the most memory its processes held in a minute (load_minutes), a simulator it booted included. Placement reads it at a session's start.
package agentload

import (
	"cmp"
	"database/sql"
	"slices"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/policy"
	"github.com/handline/handline/go/scheduler/session"
)

// Window is how far back a load is read.
const Window = policy.HeavyKeep

// Load is one agent's load over the Window.
type Load struct {
	Agent  string  // its key
	HeavyS int64   // seconds its heavy commands ran
	Shapes []Shape // what they were, the longest first
	// MemoryPeak is the most bytes its processes held in one minute, what it started (a simulator, a browser) included.
	MemoryPeak int64
	Started    []string // the presets among its runs that leave something running: @simulator, @browser
}

// Shape is one kind of heavy command and how long its runs took together.
type Shape struct {
	Name    string
	Seconds int64
	Runs    int
}

// Of is agent's load (its key) at now, epoch seconds.
func Of(q db.Q, agent string, now int64) (Load, error) {
	all, err := read(q, now, agent)
	if err != nil {
		return Load{}, err
	}
	if l, ok := all[agent]; ok {
		return *l, nil
	}
	return Load{Agent: agent}, nil
}

// All is the load of every agent with one in the Window, the heaviest first.
func All(q db.Q, now int64) ([]Load, error) {
	all, err := read(q, now, "")
	if err != nil {
		return nil, err
	}
	out := make([]Load, 0, len(all))
	for _, l := range all {
		out = append(out, *l)
	}
	slices.SortFunc(out, func(a, b Load) int {
		return cmp.Or(cmp.Compare(b.HeavyS, a.HeavyS), strings.Compare(a.Agent, b.Agent))
	})
	return out, nil
}

// read is the loads in the Window, of agent only unless it is "".
func read(q db.Q, now int64, agent string) (map[string]*Load, error) {
	since := now - int64(Window/time.Second)
	loads := map[string]*Load{}
	get := func(a string) *Load {
		if loads[a] == nil {
			loads[a] = &Load{Agent: a}
		}
		return loads[a]
	}
	shapes := map[string]map[string]*Shape{}
	add := func(a, shape string, secs int64) {
		get(a).HeavyS += secs
		if shapes[a] == nil {
			shapes[a] = map[string]*Shape{}
		}
		s := shapes[a][shape]
		if s == nil {
			s = &Shape{Name: shape}
			shapes[a][shape] = s
		}
		s.Seconds += secs
		s.Runs++
	}
	only := "" // one agent's rows by its index: an OR of both cases would scan them all
	if agent != "" {
		only = " AND agent = ?2"
	}
	runs, err := q.Query("SELECT agent, seconds, shape FROM heavy_runs WHERE ended >= ?1"+only, since, agent)
	if err != nil {
		return nil, err
	}
	for runs.Next() {
		var a, shape string
		var secs int64
		if err := runs.Scan(&a, &secs, &shape); err != nil {
			runs.Close()
			return nil, err
		}
		add(a, shape, secs)
	}
	if err := runs.Close(); err != nil {
		return nil, err
	}
	mem, err := q.Query("SELECT agent, MAX(rss_max) FROM load_minutes WHERE minute >= ?1 AND agent <> ''"+only+" GROUP BY agent", since, agent)
	if err != nil {
		return nil, err
	}
	for mem.Next() {
		var a string
		var peak int64
		if err := mem.Scan(&a, &peak); err != nil {
			mem.Close()
			return nil, err
		}
		get(a).MemoryPeak = peak
	}
	if err := mem.Close(); err != nil {
		return nil, err
	}
	for a, byName := range shapes {
		l := loads[a]
		for _, s := range byName {
			l.Shapes = append(l.Shapes, *s)
			if s.Name == "@simulator" || s.Name == "@browser" {
				l.Started = append(l.Started, s.Name)
			}
		}
		slices.SortFunc(l.Shapes, func(x, y Shape) int {
			return cmp.Or(cmp.Compare(y.Seconds, x.Seconds), strings.Compare(x.Name, y.Name))
		})
		slices.Sort(l.Started)
	}
	return loads, nil
}

// simulatorNear is how far apart a simulator command's end and its simulator's start may be.
const simulatorNear = 10 * 60

// SimulatorOf is the agent (its key) whose simulator command ended nearest to start, a simulator process's, and within simulatorNear
// of it; "" for none: a simulator runs under launchd, not under the agent that booted it.
func SimulatorOf(q db.Q, start int64) (string, error) {
	var a string
	err := q.QueryRow("SELECT agent FROM heavy_runs WHERE shape = '@simulator' AND ended BETWEEN ?1 - ?2 AND ?1 + ?2 ORDER BY ABS(ended - ?1) LIMIT 1",
		start, simulatorNear).Scan(&a)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return a, err
}

// KeepBackground keeps, at a Stop, agent's heavy background shells of the last day that have ended (session.EndedBackground) in
// heavy_runs by its key, each once.
func KeepBackground(q db.Q, agent string, now int64) error {
	runs, err := session.EndedBackground(q, agent, now-24*3600)
	if err != nil || len(runs) == 0 {
		return err
	}
	key := agent
	if k, err := paths.AgentKeyIn(q, agent); err != nil {
		return err
	} else if k != "" {
		key = k
	}
	for _, r := range runs {
		if _, err := q.Exec("INSERT OR IGNORE INTO heavy_runs(agent, ended, seconds, shape, job) VALUES(?, ?, ?, ?, ?)", key, r.Ended, r.Seconds, r.Shape, r.Job); err != nil {
			return err
		}
	}
	return nil
}

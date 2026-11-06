// Package procload knows every process on the machine and whose it is: the agent that started it, found by the HANDLINE_AGENT in the
// process's environment (which survives a child that daemonizes and is reparented to init) or, failing that, by an ancestor that is
// the agent's terminal pane or already owned. It samples each process's CPU (the change in CPU time over the time between two
// samples) and memory, keeps the last Keep of samples in memory, rolls each agent up by the minute into the database for a week, and
// serves it as one read. It also reads the listening sockets every ListenEvery seconds and gives each agent the servers it holds. It only reads: no limit is applied from here.
package procload

import (
	"database/sql"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/handline/handline/go/scheduler/sysres"
)

const (
	agentEnv = "HANDLINE_AGENT"
	// Every is how often the processes are sampled.
	Every = 5 * time.Second
	// Keep is how long the samples, and a process's last row once it ends, stay in memory.
	Keep = 15 * time.Minute
	// Retain is how long the per-minute rows stay in the database.
	Retain = 7 * 24 * time.Hour
	// unownedMax is how many processes no agent owns the read lists (the heaviest by CPU, then memory).
	unownedMax = 20
	topN       = 3
)

// How a process was found to be an agent's.
const (
	ViaEnv  = "env"  // HANDLINE_AGENT in its environment
	ViaTree = "tree" // a descendant of the agent's terminal pane or of a process found by its environment
)

// Proc is one process at a sample.
type Proc struct {
	PID, PPID int
	Agent     string // "" when no agent owns it
	Via       string // ViaEnv or ViaTree; "" when unowned
	Cmd       string
	Start     int64   // epoch seconds
	CPU       float64 // percent of one core over the time since the last sample
	RSS       int64   // resident bytes
	At        int64   // the sample this row is from
	Gone      bool    // it ended after At: this is its last row
}

// Use is a sum over processes.
type Use struct {
	CPU       float64
	RSS       int64
	Processes int
}

type key struct {
	pid   int
	start int64
}

type cpuMark struct {
	secs float64
	at   int64
}

type sample struct {
	At      int64
	Machine sysres.Machine
	Agents  map[string]Use // "" is every unowned process
}

type minuteAcc struct {
	minute         int64
	n              int
	cpuSum, cpuMax float64
	rssSum, rssMax int64
	procsMax       int
}

// Tracker samples the processes. Set the fields before Sample or Run.
type Tracker struct {
	// Panes is each agent's terminal pane process, the root of its tree; nil or empty leaves only the environment to go by.
	Panes func() map[string]int
	// Table is the process table; nil runs ps.
	Table func() ([]Raw, error)
	// AgentOf is the HANDLINE_AGENT in a process's environment (ok false when it cannot be read); nil reads it from the system.
	AgentOf func(pid int) (agent string, ok bool)
	// Machine reads the machine's memory and load; nil reads this one.
	Machine func() (sysres.Machine, error)
	// Listen reads the machine's listening TCP sockets; nil runs lsof.
	Listen func() ([]Listener, error)
	// Disk is the free and total bytes of the volume holding a path; nil is statfs.
	Disk func(path string) (free, total int64, err error)
	Home string
	// DB receives the per-minute rows; nil keeps none.
	DB  *sql.DB
	Log func(string)

	mu        sync.Mutex
	marks     map[key]cpuMark
	owners    map[key]string // the environment's answer, kept per process: "" is read and has none
	sticky    map[key]string // an owner found once by the tree, kept: a child whose parent ended and was reparented stays its agent's
	cur       []Proc
	listeners []Listener
	listenAt  int64
	servers   map[string][]Server
	ended     map[key]Proc
	ring      []sample
	machine   sysres.Machine
	at        int64
	acc       map[string]*minuteAcc
	cost      time.Duration // the last sample's own time
	costSum   time.Duration
	samples   int
	lastPrun  int64
}

func (t *Tracker) agentOf(pid int) (string, bool) {
	if t.AgentOf != nil {
		return t.AgentOf(pid)
	}
	return agentOf(pid)
}

// Sample measures every process once; now is epoch seconds.
func (t *Tracker) Sample(now int64) error {
	began := time.Now()
	table := t.Table
	if table == nil {
		table = psTable
	}
	rows, err := table()
	if err != nil {
		return err
	}
	var panes map[string]int
	if t.Panes != nil {
		panes = t.Panes()
	}
	machine := t.Machine
	if machine == nil {
		machine = sysres.Read
	}
	m, merr := machine()
	if merr != nil && t.Log != nil {
		t.Log("procload: " + merr.Error())
	}
	m.Cores = runtime.NumCPU()
	disk := t.Disk
	if disk == nil {
		disk = sysres.StatfsFree
	}
	m.DiskFree, m.DiskTotal, _ = disk(t.Home)
	var listeners []Listener // read before the lock, like the table: lsof can take seconds
	if t.listenDue(now) {
		listeners = t.readListeners()
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.marks == nil {
		t.marks, t.owners, t.sticky, t.ended, t.acc = map[key]cpuMark{}, map[key]string{}, map[key]string{}, map[key]Proc{}, map[string]*minuteAcc{}
	}
	procs := t.attribute(rows, panes, now)
	seen := make(map[key]bool, len(procs))
	for _, p := range procs {
		seen[key{p.PID, p.Start}] = true
	}
	for _, old := range t.cur {
		k := key{old.PID, old.Start}
		if !seen[k] {
			if old.Agent != "" {
				old.Gone = true
				t.ended[k] = old
			}
			delete(t.marks, k)
			delete(t.owners, k)
			delete(t.sticky, k)
		}
	}
	for k, p := range t.ended {
		if now-p.At > int64(Keep/time.Second) {
			delete(t.ended, k)
		}
	}
	t.cur, t.machine, t.at = procs, m, now
	if listeners != nil {
		t.listeners, t.listenAt = listeners, now
	}
	t.servers = serversOf(t.listeners, t.cur, panes, os.Getpid())
	by := map[string]Use{}
	for _, p := range procs {
		u := by[p.Agent]
		u.CPU, u.RSS, u.Processes = u.CPU+p.CPU, u.RSS+p.RSS, u.Processes+1
		by[p.Agent] = u
	}
	t.ring = append(t.ring, sample{At: now, Machine: m, Agents: by})
	if over := len(t.ring) - int(Keep/Every); over > 0 {
		t.ring = append(t.ring[:0:0], t.ring[over:]...)
	}
	t.roll(now, by)
	t.cost = time.Since(began)
	t.costSum += t.cost
	t.samples++
	return nil
}

// attribute turns the table into processes with their owner and CPU, and forgets the marks of processes that are not in it.
func (t *Tracker) attribute(rows []Raw, panes map[string]int, now int64) []Proc {
	byPID := make(map[int]*Raw, len(rows))
	for i := range rows {
		byPID[rows[i].PID] = &rows[i]
	}
	paneAgent := map[int]string{}
	for agent, pid := range panes {
		paneAgent[pid] = agent
	}
	type owner struct{ agent, via string }
	known := map[int]owner{}
	// The environment decides first; it is read once per process, since a process's environment does not change.
	for i := range rows {
		r := &rows[i]
		k := key{r.PID, r.Start}
		agent, ok := t.owners[k]
		if !ok {
			var read bool
			if agent, read = t.agentOf(r.PID); read {
				t.owners[k] = agent
			}
		}
		if agent != "" {
			known[r.PID] = owner{agent, ViaEnv}
		} else if a := t.sticky[k]; a != "" {
			known[r.PID] = owner{a, ViaTree}
		}
	}
	var resolve func(pid int, depth int) owner
	resolve = func(pid int, depth int) owner {
		if o, ok := known[pid]; ok {
			return o
		}
		o := owner{}
		if a, ok := paneAgent[pid]; ok {
			o = owner{a, ViaTree}
		} else if r := byPID[pid]; r != nil && depth < 64 && r.PPID > 1 && r.PPID != pid {
			if up := resolve(r.PPID, depth+1); up.agent != "" {
				o = owner{up.agent, ViaTree}
			}
		}
		known[pid] = o
		return o
	}
	live := make(map[key]bool, len(rows))
	out := make([]Proc, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		k := key{r.PID, r.Start}
		live[k] = true
		o := resolve(r.PID, 0)
		if o.agent != "" {
			t.sticky[k] = o.agent
		}
		cpu := 0.0
		if prev, ok := t.marks[k]; ok && now > prev.at {
			cpu = (r.CPUSecs - prev.secs) / float64(now-prev.at) * 100
		} else if !ok {
			age := now - r.Start
			if age < 1 {
				age = 1
			}
			cpu = r.CPUSecs / float64(age) * 100
		}
		if cpu < 0 {
			cpu = 0
		}
		t.marks[k] = cpuMark{r.CPUSecs, now}
		out = append(out, Proc{PID: r.PID, PPID: r.PPID, Agent: o.agent, Via: o.via, Cmd: r.Cmd, Start: r.Start, CPU: cpu, RSS: r.RSS, At: now})
	}
	for k := range t.marks {
		if !live[k] {
			delete(t.marks, k)
		}
	}
	for k := range t.owners {
		if !live[k] {
			delete(t.owners, k)
		}
	}
	for k := range t.sticky {
		if !live[k] {
			delete(t.sticky, k)
		}
	}
	return out
}

// roll adds a sample to its minute and writes the minute out when the next one begins.
func (t *Tracker) roll(now int64, by map[string]Use) {
	minute := now - now%60
	for agent, a := range t.acc {
		if a.minute != minute {
			t.flush(agent, a)
			delete(t.acc, agent)
		}
	}
	for agent, u := range by {
		a := t.acc[agent]
		if a == nil {
			a = &minuteAcc{minute: minute}
			t.acc[agent] = a
		}
		a.n++
		a.cpuSum += u.CPU
		a.rssSum += u.RSS
		a.cpuMax = max(a.cpuMax, u.CPU)
		a.rssMax = max(a.rssMax, u.RSS)
		a.procsMax = max(a.procsMax, u.Processes)
	}
	if t.DB != nil && now-t.lastPrun >= 3600 {
		t.lastPrun = now
		if _, err := t.DB.Exec("DELETE FROM load_minutes WHERE minute < ?", now-int64(Retain/time.Second)); err != nil && t.Log != nil {
			t.Log("procload: " + err.Error())
		}
	}
}

func (t *Tracker) flush(agent string, a *minuteAcc) {
	if t.DB == nil || a.n == 0 {
		return
	}
	_, err := t.DB.Exec(`INSERT OR REPLACE INTO load_minutes(minute, agent, cpu_avg, cpu_max, rss_avg, rss_max, procs_max) VALUES (?,?,?,?,?,?,?)`,
		a.minute, agent, a.cpuSum/float64(a.n), a.cpuMax, a.rssSum/int64(a.n), a.rssMax, a.procsMax)
	if err != nil && t.Log != nil {
		t.Log("procload: " + err.Error())
	}
}

// Run samples every Every until stop is closed.
func (t *Tracker) Run(stop <-chan struct{}) {
	sysres.Loop(stop, Every, func(now int64) {
		if err := t.Sample(now); err != nil && t.Log != nil {
			t.Log("procload: " + err.Error())
		}
	})
}

// Point is one minute of an agent.
type Point struct {
	Minute         int64
	CPUAvg, CPUMax float64
	RSSAvg, RSSMax int64
	ProcsMax       int
}

// Series is each agent's minutes since from (epoch seconds), oldest first; "" is the processes no agent owns.
func (t *Tracker) Series(from int64) (map[string][]Point, error) {
	out := map[string][]Point{}
	if t.DB == nil {
		return out, nil
	}
	rows, err := t.DB.Query(`SELECT minute, agent, cpu_avg, cpu_max, rss_avg, rss_max, procs_max FROM load_minutes WHERE minute >= ? ORDER BY minute`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var agent string
		var p Point
		if err := rows.Scan(&p.Minute, &agent, &p.CPUAvg, &p.CPUMax, &p.RSSAvg, &p.RSSMax, &p.ProcsMax); err != nil {
			return nil, err
		}
		out[agent] = append(out[agent], p)
	}
	return out, rows.Err()
}

// AgentUse is one agent in a view.
type AgentUse struct {
	Agent string
	Use
	Env, Tree int // how many of its processes were found by each way
	Top       []Proc
}

// View is the latest sample as the read serves it.
type View struct {
	At        int64
	Machine   sysres.Machine
	Total     Use
	Agents    []AgentUse // the heaviest first
	Unowned   AgentUse
	Processes []Proc // every owned process, those that ended in the last Keep too, then the heaviest unowned ones
	SampleMS  float64
	Samples   int
}

func heavier(a, b Proc) bool {
	if a.CPU != b.CPU {
		return a.CPU > b.CPU
	}
	if a.RSS != b.RSS {
		return a.RSS > b.RSS
	}
	return a.PID < b.PID
}

// View is the latest sample; ok false before the first.
func (t *Tracker) View() (View, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.samples == 0 {
		return View{}, false
	}
	v := View{At: t.at, Machine: t.machine, Samples: t.samples, SampleMS: float64(t.costSum) / float64(t.samples) / 1e6}
	all := append([]Proc(nil), t.cur...)
	for _, p := range t.ended {
		all = append(all, p)
	}
	sort.Slice(all, func(i, j int) bool { return heavier(all[i], all[j]) })
	group := map[string]*AgentUse{}
	var unowned []Proc
	for _, p := range all {
		if p.Agent == "" {
			if !p.Gone {
				unowned = append(unowned, p)
			}
		} else {
			g := group[p.Agent]
			if g == nil {
				g = &AgentUse{Agent: p.Agent}
				group[p.Agent] = g
			}
			if !p.Gone {
				g.CPU, g.RSS, g.Processes = g.CPU+p.CPU, g.RSS+p.RSS, g.Processes+1
				if p.Via == ViaEnv {
					g.Env++
				} else {
					g.Tree++
				}
			}
			if len(g.Top) < topN {
				g.Top = append(g.Top, p)
			}
			v.Processes = append(v.Processes, p)
		}
		if !p.Gone {
			v.Total.CPU, v.Total.RSS, v.Total.Processes = v.Total.CPU+p.CPU, v.Total.RSS+p.RSS, v.Total.Processes+1
		}
	}
	for _, p := range unowned {
		v.Unowned.CPU, v.Unowned.RSS, v.Unowned.Processes = v.Unowned.CPU+p.CPU, v.Unowned.RSS+p.RSS, v.Unowned.Processes+1
	}
	if len(unowned) > unownedMax {
		v.Unowned.Top = unowned[:unownedMax]
	} else {
		v.Unowned.Top = unowned
	}
	v.Processes = append(v.Processes, v.Unowned.Top...)
	for _, g := range group {
		v.Agents = append(v.Agents, *g)
	}
	sort.Slice(v.Agents, func(i, j int) bool {
		a, b := v.Agents[i], v.Agents[j]
		if a.CPU != b.CPU {
			return a.CPU > b.CPU
		}
		if a.RSS != b.RSS {
			return a.RSS > b.RSS
		}
		return a.Agent < b.Agent
	})
	return v, true
}

package runload

import (
	"encoding/json"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/session"
	"github.com/handline/handline/go/scheduler/sysres"
)

// metaKey is where the state is kept across a restart.
const metaKey = "run_load"

const (
	// shapesFor is how long the shapes read from the database are used before they are read again.
	shapesFor = 2 * time.Minute
	// backgroundBack is how far back a background shell's start is looked for.
	backgroundBack = 4 * time.Hour
	// loopFresh is how long the daemon loop's last time stands for the loop: a loop that stopped reporting says nothing.
	loopFresh = 2 * time.Minute
)

// Host is what the machine itself says, for the person to read beside the causes; none of it makes the machine slow by itself.
type Host struct {
	LoadPerCore float64
	// Pressure is the kernel's memory pressure: "normal", "warn" or "critical", "" when it is not read.
	Pressure string
	// CompressedPerS is how many memory pages a second the kernel compressed since the last look.
	CompressedPerS float64
}

// Watcher looks at the machine every Every seconds and answers the hook's question before a heavy run. Set the exported fields
// before Run; nil ones read this machine.
type Watcher struct {
	Home string
	DB   db.Q
	// Config is the policy as config.yaml has it now; nil is Observe. It is read at each look and each reading of the status, never
	// for a hook's question, which is answered from the last one read.
	Config func() Config
	// Procs is the latest reading of the process table (procload.Tracker.View), ok false before the first; nil finds no run's
	// processes, and then nothing is lowered and a run ends only by its hook or its time limit.
	Procs func() ([]procload.Proc, bool)
	// Machine is the machine's latest measure (sysres.Sampler.LatestMachine).
	Machine func() *sysres.Machine
	// ReadHost is the memory pressure level and the pages compressed since boot.
	ReadHost func() (pressure int, compressions int64, ok bool)
	// Lower gives the process pid that began at start a lower priority (LowNice).
	Lower func(pid int, start int64) error
	// Slept is how long the machine was asleep between two looks; nil reads it off the two clocks a time carries.
	Slept func(prev, now time.Time) time.Duration
	Log   func(string, ...any)
	Now   func() time.Time

	mu       sync.Mutex
	loaded   bool
	state    State
	sig      Signals
	host     Host
	cfg      Config // as last read
	cores    int
	runs     []Run
	readAt   time.Time // when the runs were last read
	lookedAt time.Time // when the state was last stepped
	shapes   Shapes
	shapesAt time.Time
	found    Found
	gone     map[string]bool // runs whose shell ended with no hook telling, until their note goes
	unsure   map[string]bool // runs going while the machine slept: their time is not all theirs, so they are never late
	lowered  map[procKey]bool
	asks     []*ask
	grants   map[string]time.Time
	pages    int64
	pagesAt  time.Time

	loopMu sync.Mutex // the loop's clock has its own lock, so a tick never waits on a look
	loop   loopClock
}

type loopClock struct {
	began, ended time.Time
	took         time.Duration
}

func (w *Watcher) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *Watcher) logf(format string, args ...any) {
	if w.Log != nil {
		w.Log(format, args...)
	}
}

func (w *Watcher) config() Config {
	if w.Config == nil {
		return Config{Policy: Observe}
	}
	return w.Config()
}

func (w *Watcher) coresNow() int {
	if w.Machine != nil {
		if m := w.Machine(); m != nil && m.Cores > 0 {
			return m.Cores
		}
	}
	return runtime.NumCPU()
}

// LoopBegan and LoopEnded are told by the daemon's loop at each turn of it, so a loop that takes long is seen while it still runs.
func (w *Watcher) LoopBegan() {
	w.loopMu.Lock()
	w.loop.began = w.now()
	w.loopMu.Unlock()
}

func (w *Watcher) LoopEnded(took time.Duration) {
	w.loopMu.Lock()
	w.loop.began, w.loop.ended, w.loop.took = time.Time{}, w.now(), took
	w.loopMu.Unlock()
}

// loopMs is how long the loop last took, or has taken so far when that is longer; 0 when it has not reported for loopFresh.
func (w *Watcher) loopMs(now time.Time) int64 {
	w.loopMu.Lock()
	defer w.loopMu.Unlock()
	var ms int64
	if !w.loop.ended.IsZero() && now.Sub(w.loop.ended) <= loopFresh {
		ms = w.loop.took.Milliseconds()
	}
	if !w.loop.began.IsZero() {
		ms = max(ms, now.Sub(w.loop.began).Milliseconds())
	}
	return ms
}

// Run looks every Every seconds until stop is closed.
func (w *Watcher) Run(stop <-chan struct{}) {
	sysres.Loop(stop, Every*time.Second, func(int64) { w.Look() })
}

// load reads the kept state once. The caller holds mu.
func (w *Watcher) load() {
	if w.loaded {
		return
	}
	w.loaded = true
	w.found, w.gone, w.unsure, w.lowered, w.grants = Found{}, map[string]bool{}, map[string]bool{}, map[procKey]bool{}, map[string]time.Time{}
	w.cores = runtime.NumCPU()
	if w.DB == nil {
		return
	}
	if v, ok, err := db.Meta(w.DB, metaKey); err == nil && ok && v != "" {
		_ = json.Unmarshal([]byte(v), &w.state)
	}
}

// seen is one reading of the runs and the settings, taken with no lock held: it reads files, the database and the process table.
type seen struct {
	at          time.Time
	cfg         Config
	cores       int
	shapes      Shapes // nil when they were not read this time
	starts      []session.OpenStart
	backgrounds []Background
	procs       []procload.Proc
	procsOK     bool
}

func (w *Watcher) gather(now time.Time, shapes bool) seen {
	s := seen{at: now, cfg: w.config(), cores: w.coresNow(), starts: session.OpenStarts(w.Home)}
	if w.Procs != nil {
		s.procs, s.procsOK = w.Procs()
	}
	if w.DB == nil {
		return s
	}
	if shapes {
		read, err := ReadShapes(w.DB, now.Unix())
		if err != nil {
			w.logf("run load: reading the shapes: %v (the last read stands)", err)
		}
		s.shapes = read
	}
	open, err := session.OpenBackgrounds(w.DB, now.Add(-backgroundBack).Unix())
	if err != nil {
		w.logf("run load: reading the background shells: %v", err)
	}
	keys := map[string]string{}
	for _, b := range open {
		key, known := keys[b.Agent]
		if !known {
			if key, _ = paths.AgentKeyIn(w.DB, b.Agent); key == "" {
				key = b.Agent
			}
			keys[b.Agent] = key
		}
		s.backgrounds = append(s.backgrounds, Background{Agent: key, Job: b.Job, Shape: b.Shape, Started: b.Started})
	}
	return s
}

// take makes s what the watcher knows: the runs going, their processes, and the turns still standing for a run not yet seen.
// The caller holds mu.
func (w *Watcher) take(s seen) {
	w.readAt, w.cfg, w.cores = s.at, s.cfg, s.cores
	if s.shapes != nil {
		w.shapes, w.shapesAt = s.shapes, s.at
	}
	live := map[string]bool{}
	for _, st := range s.starts {
		live[st.ID] = true
	}
	for _, b := range s.backgrounds {
		live[b.Job] = true
	}
	for _, ids := range []map[string]bool{w.gone, w.unsure} {
		for id := range ids {
			if !live[id] {
				delete(ids, id)
			}
		}
	}
	runs := Active(s.starts, s.backgrounds, w.shapes, w.gone, s.at)
	var heavy []procKey
	if s.procsOK {
		var ended map[string]bool
		ended, heavy = Match(runs, s.procs, w.found)
		if len(ended) > 0 {
			kept := runs[:0]
			for _, r := range runs {
				if ended[r.ID] {
					w.gone[r.ID] = true
					continue
				}
				kept = append(kept, r)
			}
			runs = kept
		}
	}
	for i := range runs {
		if w.unsure[runs[i].ID] {
			runs[i].Late = false
		}
	}
	w.runs = runs
	for id, at := range w.grants { // a turn stands for its run until the run's note is read, or it never came
		if live[id] || s.at.Sub(at) > grantFor {
			delete(w.grants, id)
		}
	}
	if w.state.LowersPriority(w.cfg) {
		w.lower(heavy)
	} else if len(w.lowered) > 0 {
		w.lowered = map[procKey]bool{}
	}
}

// lower gives each of procs LowNice once. The caller holds mu.
func (w *Watcher) lower(procs []procKey) {
	low := w.Lower
	if low == nil {
		low = lower
	}
	there := map[procKey]bool{}
	for _, k := range procs {
		there[k] = true
		if w.lowered[k] {
			continue
		}
		w.lowered[k] = true
		if err := low(k.pid, k.start); err != nil {
			w.logf("run load: lowering the priority of process %d: %v", k.pid, err)
		}
	}
	for k := range w.lowered {
		if !there[k] {
			delete(w.lowered, k)
		}
	}
}

// sleptMin is how long the machine must have slept between two looks for the runs going to be set aside: their time on the clock
// is then not the time they ran, and they would all look late at once.
const sleptMin = 30 * time.Second

// asleep is how long the machine slept between prev and now: what the wall clock passed that the running clock did not.
func asleep(prev, now time.Time) time.Duration {
	return now.Round(0).Sub(prev.Round(0)) - now.Sub(prev)
}

// Look reads the machine once and steps the state. Everything slow is read before the lock is taken, so a hook's question and a
// reading of the status never wait on a file, the database or another program.
func (w *Watcher) Look() {
	now := w.now()
	w.mu.Lock()
	w.load()
	shapes := w.shapes == nil || now.Sub(w.shapesAt) >= shapesFor
	prev := w.lookedAt
	w.mu.Unlock()

	s := w.gather(now, shapes)
	timedOut := 0
	if w.DB != nil {
		if err := w.DB.QueryRow("SELECT COUNT(*) FROM heavy_runs WHERE timed_out = 1 AND ended >= ?", now.Unix()-TimeoutWindow).Scan(&timedOut); err != nil {
			w.logf("run load: reading the runs that timed out: %v", err)
		}
	}
	read := w.ReadHost
	if read == nil {
		read = readHost
	}
	level, pages, hostOK := read()
	slept := w.Slept
	if slept == nil {
		slept = asleep
	}
	woke := !prev.IsZero() && slept(prev, now) >= sleptMin

	w.mu.Lock()
	w.take(s)
	w.lookedAt = now
	if woke { // the runs going were going before the sleep: none of them says the machine is slow, and the count starts over
		for i := range w.runs {
			w.unsure[w.runs[i].ID], w.runs[i].Late = true, false
		}
		w.state.Signs = 0
	}
	sig := Signals{LoopMs: w.loopMs(now), Cores: w.cores, TimedOut: timedOut}
	for _, r := range w.runs {
		if r.Heavy {
			sig.Heavy++
		}
		if r.Late {
			sig.Late++
		}
	}
	w.sig = sig
	w.host = Host{}
	if w.Machine != nil {
		if m := w.Machine(); m != nil && m.Cores > 0 {
			w.host.LoadPerCore = m.Load1 / float64(m.Cores)
		}
	}
	if hostOK {
		w.host.Pressure = pressures[level]
		if secs := now.Sub(w.pagesAt).Seconds(); !w.pagesAt.IsZero() && secs > 0 && pages >= w.pages {
			w.host.CompressedPerS = float64(pages-w.pages) / secs
		}
		w.pages, w.pagesAt = pages, now
	}
	next := Step(w.state, sig, now.Unix())
	moved := next.Slow != w.state.Slow || next.Cap != w.state.Cap
	keep := moved || !reflect.DeepEqual(next.kept(), w.state.kept())
	if moved {
		w.logf("run load: slow=%v causes=%v cap=%d (late runs %d, timed out %d, loop %dms, heavy runs %d)", next.Slow, next.Causes, next.Cap,
			sig.Late, sig.TimedOut, sig.LoopMs, sig.Heavy)
	}
	w.state = next
	w.mu.Unlock()

	if keep && w.DB != nil {
		kept, _ := json.Marshal(next.kept())
		if err := db.SetMeta(w.DB, metaKey, string(kept)); err != nil {
			w.logf("run load: keeping the state: %v", err)
		}
	}
}

var pressures = map[int]string{1: "normal", 2: "warn", 4: "critical"}

// View is what the watcher knows now.
type View struct {
	// Looked is whether the machine has been looked at yet.
	Looked  bool
	Config  Config
	State   State
	Signals Signals
	Host    Host
	// Cap is the most heavy runs that go at once now, 0 when there is no cap.
	Cap     int
	Runs    []Run
	Waiting []Waiting
}

// Waiting is a heavy run held before its start.
type Waiting struct {
	Agent, Shape string
	Since        int64
}

// View is the state as it stood at the last look, under the settings as they are now, and who waits now.
func (w *Watcher) View() View {
	now, cfg := w.now(), w.config()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.load()
	w.forget(now)
	w.cfg = cfg
	v := View{Looked: !w.lookedAt.IsZero(), Config: cfg, State: w.state, Signals: w.sig, Host: w.host, Cap: w.state.CapFor(cfg, w.cores),
		Runs: append([]Run(nil), w.runs...)}
	for _, a := range w.asks {
		v.Waiting = append(v.Waiting, Waiting{Agent: a.agent, Shape: a.shape, Since: a.since.Unix()})
	}
	return v
}

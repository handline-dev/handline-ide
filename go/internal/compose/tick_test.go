package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/states"
	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/tick"
)

func TestAnAgentWithTheRenewalSwitchOnIsNotStalledForWantingACacheClock(t *testing.T) {
	p, _, _, wt := wired(t)
	d := p.db
	os.WriteFile(filepath.Join(p.home, "agents", "alice", "agent.yaml"), []byte("vendor: claude\nstructured: true\ncache_renewal: true\nidle_fold: true\n"), 0o644)
	// A session that is open and idle, with a status naming a transcript that is not there: the clock is unreadable, so no renewal is due.
	if err := dbpkg.SetResumeSession(d, "alice", "s1"); err != nil {
		t.Fatal(err)
	}
	d.Exec("UPDATE agent_state SET closed_at = NULL, state = 'IDLE' WHERE agent = 'alice'")
	_ = wt
	tk := p.Tick(cfgOf(p), TickConfig{})
	if tk.Tick.Cost == nil || tk.Tick.Cost.Ports == nil {
		t.Fatal("the cost pilot has no ports")
	}
	for i := 0; i < 4; i++ {
		if _, err := tk.Tick.Run(); err != nil {
			t.Fatal(err)
		}
	}
	row, _ := dbpkg.GetAgentState(d, "alice")
	if row.State == "STALLED" || row.FailedAttempts != 0 {
		t.Fatalf("the agent was stalled by the pilot: %+v", row)
	}
	cp := costPorts{}
	if cp.ReadCache("") != nil || cp.ReadCache("/nonexistent") != nil {
		t.Fatal("a missing transcript has a clock")
	}
}

func TestShutdownWaitsForTheTickInHandBeforeTheDatabaseCloses(t *testing.T) {
	p, _, _, _ := wired(t)
	stop := make(chan struct{})
	tk := p.Tick(cfgOf(p), TickConfig{Stop: stop})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	tk.Tick.Passes = []tick.NamedSweep{{Name: "slow", Run: func(dbpkg.Q, int64) error {
		once.Do(func() { close(entered) })
		<-release // a close or a wake in the middle
		return nil
	}}}
	done := tk.Start()
	<-entered
	close(stop)
	select {
	case <-done:
		t.Fatal("the loop returned while a tick was still running")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the loop did not return after the tick finished")
	}
}

func TestTheTickRunsItsPassesWithoutHarmingAnAgent(t *testing.T) {
	p, _, _, _ := wired(t)
	tk := p.Tick(cfgOf(p), TickConfig{})
	if len(tk.Tick.Passes) != 15 {
		t.Fatalf("passes: %d", len(tk.Tick.Passes))
	}
	var logs []string
	cfg := cfgOf(p)
	cfg.Log = func(format string, a ...any) { logs = append(logs, fmt.Sprintf(format, a...)) }
	tk = p.Tick(cfg, TickConfig{})
	for i := 0; i < 3; i++ {
		if _, err := tk.Tick.Run(); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range logs {
		if strings.Contains(l, "timebox:") || strings.Contains(l, "schedules:") || strings.Contains(l, "notice-digest:") || strings.Contains(l, "task-queue:") || strings.Contains(l, "task-holds:") || strings.Contains(l, "subscriptions:") || strings.Contains(l, "wait-for:") || strings.Contains(l, "apply-settings:") || strings.Contains(l, "proposal-held:") {
			t.Fatalf("a pass failed: %s", l)
		}
	}
}

func TestTheFirstTickAfterMidnightRollsTheDigestOfAnAgent(t *testing.T) {
	p, _, _, _ := wired(t)
	dir := filepath.Join(p.home, "agents", "alice", "journal")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-10-09.md"), []byte("# alice · 2026-10-09\nupdated: 10:00\n\n## In progress\n- the port\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 9, 23, 59, 0, 0, time.Local)
	cfg := cfgOf(p)
	cfg.Now = func() time.Time { return now }
	tk := p.Tick(cfg, TickConfig{Terminals: true})
	run := func() {
		t.Helper()
		if _, err := tk.Tick.Run(); err != nil {
			t.Fatal(err)
		}
	}
	run() // the first tick only learns the day
	if _, err := os.Stat(filepath.Join(dir, "2026-10-10.md")); err == nil {
		t.Fatal("rolled before midnight")
	}
	now = now.Add(2 * time.Minute)
	run()
	raw, err := os.ReadFile(filepath.Join(dir, "2026-10-10.md"))
	if err != nil || !strings.Contains(string(raw), "the port") {
		t.Fatalf("today's digest: %v %q", err, raw)
	}
}

func TestTheLastContextIsReadPastALineLongerThanAScannerAllows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	small := `{"type":"assistant","message":{"model":"m","usage":{"input_tokens":1}}}` + "\n"
	big := `{"type":"assistant","message":{"model":"m2","usage":{"input_tokens":7,"cache_read_input_tokens":3},"content":"` + strings.Repeat("x", 2<<20) + `"}}` + "\n"
	os.WriteFile(path, []byte(small+big), 0o644)
	if used, model, ok := lastContext(path); !ok || used != 10 || model != "m2" {
		t.Fatalf("%d %q %v", used, model, ok)
	}
}

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func reply(model string, tokens int) string {
	return fmt.Sprintf(`{"type":"assistant","message":{"model":%q,"usage":{"input_tokens":%d}}}`, model, tokens)
}

func TestTheNewestReplyWinsWhenToolResultsAfterItSpanSeveralChunks(t *testing.T) {
	result := `{"type":"user","message":{"content":"` + strings.Repeat("r", 3*chunkBytes) + `"}}`
	path := writeTranscript(t, reply("old", 1), reply("new", 5), result, result, "")
	if used, model, ok := lastContext(path); !ok || used != 5 || model != "new" {
		t.Fatalf("%d %q %v", used, model, ok)
	}
}

func TestTheFirstLineOfAWholeFileIsARepliesLineEvenWithoutANewline(t *testing.T) {
	if used, model, ok := lastContext(writeTranscript(t, reply("m", 4))); !ok || used != 4 || model != "m" {
		t.Fatalf("%d %q %v", used, model, ok)
	}
}

func TestASidechainReplyAfterTheMainOneIsPassedOver(t *testing.T) {
	side := `{"type":"assistant","isSidechain":true,"message":{"model":"side","usage":{"input_tokens":9}}}`
	if used, model, ok := lastContext(writeTranscript(t, reply("main", 2), side)); !ok || used != 2 || model != "main" {
		t.Fatalf("%d %q %v", used, model, ok)
	}
}

func TestAnAssistantLineWithoutUsageIsNotAReading(t *testing.T) {
	bare := `{"type":"assistant","message":{"model":"bare"}}`
	if used, model, ok := lastContext(writeTranscript(t, reply("used", 3), bare)); !ok || used != 3 || model != "used" {
		t.Fatalf("%d %q %v", used, model, ok)
	}
}

func TestAUserLineWithAnAssistantRoleIsNotAReply(t *testing.T) {
	user := `{"type":"user","message":{"role":"assistant","model":"u","usage":{"input_tokens":8}}}`
	if used, model, ok := lastContext(writeTranscript(t, reply("a", 6), user)); !ok || used != 6 || model != "a" {
		t.Fatalf("%d %q %v", used, model, ok)
	}
}

func TestTheLineTheTailCutsIntoIsNotRead(t *testing.T) {
	cut := reply("cut", 1) // the tail starts right where this reply does, inside a longer line
	filler := strings.Repeat("f", tailBytes-len(cut)-1)
	if _, _, ok := lastContext(writeTranscript(t, strings.Repeat("p", chunkBytes)+cut, filler)); ok {
		t.Fatal("the fragment at the cut was read as a reply")
	}
}

// countingReader is a transcript in memory that counts the bytes read from it.
type countingReader struct {
	data []byte
	read int
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	c.read += len(p)
	return copy(p, c.data[off:]), nil
}

func TestALineOfManyChunksIsReadOnceToFindItsStartAndOnceWhole(t *testing.T) {
	long := `{"type":"user","message":{"content":"` + strings.Repeat("l", 40*chunkBytes) + `"}}`
	data := []byte(reply("before", 7) + "\n" + long)
	r := &countingReader{data: data}
	if used, model, ok := newestReply(r, int64(len(data))); !ok || used != 7 || model != "before" {
		t.Fatalf("%d %q %v", used, model, ok)
	}
	if limit := 2*len(long) + 2*chunkBytes; r.read > limit {
		t.Fatalf("read %d bytes for a %d-byte line, more than %d", r.read, len(long), limit)
	}
}

func TestAReplyOnlyFurtherBackThanTheTailIsNotRead(t *testing.T) {
	filler := strings.Repeat(`{"type":"user"}`+"\n", tailBytes/16+chunkBytes)
	if _, _, ok := lastContext(writeTranscript(t, reply("far", 1), filler)); ok {
		t.Fatal("a reply before the tail was read")
	}
}

func TestAReplyLongerThanTheTailIsNotReadFromTheFileStart(t *testing.T) {
	huge := `{"type":"assistant","message":{"model":"huge","usage":{"input_tokens":2},"content":"` + strings.Repeat("h", tailBytes) + `"}}`
	if _, _, ok := lastContext(writeTranscript(t, huge, `{"type":"user"}`)); ok {
		t.Fatal("a line that starts before the tail was read")
	}
}

func TestTheLoopEndsForARestartWhenDevModeFindsAChangedExecutableThatStarts(t *testing.T) {
	p, _, _, _ := wired(t)
	exe := filepath.Join(t.TempDir(), "handlined")
	os.WriteFile(exe, []byte("old"), 0o755)
	clock := time.Unix(1_800_000_000, 0)
	cfg := cfgOf(p)
	cfg.Now = func() time.Time { return clock }
	tk := p.Tick(cfg, TickConfig{Stop: make(chan struct{})})
	tk.dev.Exe, tk.dev.Check = exe, func(string) error { return nil }
	tk.dev.Baseline()
	tk.Loop.Sleep = func(time.Duration) bool { clock = clock.Add(31 * time.Second); return true }
	os.WriteFile(paths.ConfigFile(p.home), []byte("dev_mode: true\n"), 0o644)
	os.WriteFile(exe, []byte("a new build"), 0o755) // another size: a changed executable
	select {
	case <-tk.Start():
	case <-time.After(20 * time.Second):
		t.Fatal("the loop never ended")
	}
	if !tk.Restarting() {
		t.Fatal("it ended without asking to restart")
	}
}

func TestTheTickRelinksAccountsToSharedItemsAddedAfterTheyWereRegistered(t *testing.T) {
	p, _, _, _ := wired(t)
	cdir, _ := team.AccountConfigDir(p.home, "a1")
	os.MkdirAll(filepath.Join(p.home, "shared", ".claude"), 0o755)
	os.WriteFile(filepath.Join(p.home, "shared", ".claude", "CLAUDE.md"), []byte("x"), 0o644)
	tk := p.Tick(cfgOf(p), TickConfig{})
	got, err := tk.Tick.Sweeps.RelinkAccounts()
	if err != nil || len(got["a1"]) == 0 {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(cdir, "CLAUDE.md")); err != nil {
		t.Fatal("not linked")
	}
}

func TestTheLoopRunsTheDescriptorCheckAndLogRotationAfterEachTick(t *testing.T) {
	p, _, _, _ := wired(t)
	var mu sync.Mutex
	var logged []string
	cfg := cfgOf(p)
	cfg.Log = func(f string, a ...any) { mu.Lock(); logged = append(logged, fmt.Sprintf(f, a...)); mu.Unlock() }
	log := filepath.Join(p.home, "logs", "daemon.err.log")
	os.MkdirAll(filepath.Dir(log), 0o755)
	os.WriteFile(log, make([]byte, 11<<20), 0o644)
	tk := p.Tick(cfg, TickConfig{Stop: make(chan struct{})})
	tk.Loop.MaxTicks = 1
	<-tk.Start()
	if st, _ := os.Stat(log); st.Size() != 0 {
		t.Fatal("the log over its limit was not rotated")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(logged, "\n"), "fds: ") {
		t.Fatalf("no descriptor count: %q", logged)
	}
}

// passRuns reports whether the named pass reaches the db: on a closed one it errors or panics, and .
func passRuns(t *testing.T, name string) (ran bool) {
	p, _, _, _ := wired(t)
	tk := p.Tick(cfgOf(p), TickConfig{})
	p.db.Close()
	for _, s := range tk.Tick.Passes {
		if s.Name == name {
			defer func() {
				if recover() != nil {
					ran = true
				}
			}()
			return s.Run(p.db, time.Now().Unix()) != nil
		}
	}
	t.Fatalf("no pass %s", name)
	return
}

func TestEveryPassTheTickMakesReachesTheDatabase(t *testing.T) {
	for _, name := range []string{"task-queue", "task-holds", "subscriptions", "notice-digest", "wait-for"} {
		if !passRuns(t, name) {
			t.Errorf("%s did not run", name)
		}
	}
}

func TestTheSubscriptionsPassRuns(t *testing.T) {
	p, _, _, _ := wired(t)
	t.Setenv("PATH", t.TempDir()) // the plan question must not reach a real claude
	tk := p.Tick(cfgOf(p), TickConfig{})
	for _, s := range tk.Tick.Passes {
		if s.Name == "subscriptions" {
			if err := s.Run(p.db, time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
		}
	}
	var n int
	p.db.QueryRow("SELECT COUNT(*) FROM cursors WHERE name LIKE 'plan_checked:%'").Scan(&n)
	if n == 0 {
		t.Fatal("the pass left no plan cursor")
	}
}

func TestTheTickRunsTheNoticeSweepAsALevelOfPersistedState(t *testing.T) {
	p, _, _, _ := wired(t)
	d := p.db
	key := "stopped_told:fatal:alice"
	tick := func() {
		t.Helper()
		if _, err := p.Tick(cfgOf(p), TickConfig{}).Tick.Run(); err != nil { // a new tick each time: nothing is remembered in memory
			t.Fatal(err)
		}
	}
	if err := dbpkg.SetAgentState(d, "alice", states.Stalled, 0, "boom"); err != nil {
		t.Fatal(err)
	}
	tick()
	if v, _ := dbpkg.Cursor(d, key); v != 1 {
		t.Fatalf("a FATAL agent is marked as told once the tick has run: %d", v)
	}
	var said int
	count := func() {
		d.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'notified'").Scan(&said)
	}
	count()
	before := said
	tick()
	if count(); said != before {
		t.Fatal("the same spell is told once, however many ticks follow")
	}
	if err := dbpkg.SetAgentState(d, "alice", states.Idle, 0, ""); err != nil {
		t.Fatal(err)
	}
	tick()
	if v, _ := dbpkg.Cursor(d, key); v != 0 {
		t.Fatalf("the mark ends with the spell: %d", v)
	}
}

func TestTheStartRunsReconcileMigrateStartWaitingReloadInThatOrder(t *testing.T) {
	var calls []string
	ports := startPorts{
		Reconcile:    func() ([]string, error) { calls = append(calls, "reconcile"); return nil, nil },
		Migrate:      func() error { calls = append(calls, "migrate"); return nil },
		StartWaiting: func() []string { calls = append(calls, "start-waiting"); return nil },
		Reload:       func() error { calls = append(calls, "reload"); return nil },
	}
	runStartup(func(string) {}, ports)
	if got := strings.Join(calls, ","); got != "reconcile,migrate,start-waiting,reload" {
		t.Fatalf("%s", got)
	}
}

func TestAReconcileThatFailsEndsTheStartBeforeAnyAgentIsStarted(t *testing.T) {
	var calls, logs []string
	ports := startPorts{
		Reconcile: func() ([]string, error) {
			calls = append(calls, "reconcile")
			return []string{"x: resumed"}, fmt.Errorf("database is locked")
		},
		Migrate:      func() error { calls = append(calls, "migrate"); return nil },
		StartWaiting: func() []string { calls = append(calls, "start-waiting"); return nil },
		Reload:       func() error { calls = append(calls, "reload"); return nil },
	}
	runStartup(func(s string) { logs = append(logs, s) }, ports)
	if strings.Join(calls, ",") != "reconcile" {
		t.Fatalf("%v", calls)
	}
	if all := strings.Join(logs, "\n"); !strings.Contains(all, "database is locked") || !strings.Contains(all, "x: resumed") {
		t.Fatalf("%q", logs)
	}
}

func TestTheDiskWatchEntryRaisesOneCardFromTheTaskBoard(t *testing.T) {
	p, _, _, _ := wired(t)
	cfg := cfgOf(p)
	cfg.DiskFree = func(string) (int64, int64, error) { return 9 << 30, 500 << 30, nil }
	tk := p.Tick(cfg, TickConfig{Stop: make(chan struct{})})
	var watch *tick.NamedSweep
	for i, h := range tk.Loop.Housekeeping {
		if h.Name == "disk watch" {
			watch = &tk.Loop.Housekeeping[i]
		}
	}
	if watch == nil {
		t.Fatal("the loop has no disk watch")
	}
	told := func() int {
		var n int
		if err := p.db.QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'disk_low'").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	now := time.Now().Unix()
	if err := watch.Run(p.db, now); err != nil {
		t.Fatal(err)
	}
	if told() != 1 {
		t.Fatalf("one disk_low event after a low volume: %d", told())
	}
	var inbox, tasks int
	p.db.QueryRow("SELECT COUNT(*) FROM requests WHERE target = ?", dbpkg.Human).Scan(&inbox)
	p.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE owner = ?", dbpkg.Human).Scan(&tasks)
	if inbox != 0 || tasks != 0 {
		t.Fatalf("a low volume is a notification, not a message (%d) or a task (%d)", inbox, tasks)
	}
	if err := watch.Run(p.db, now+601); err != nil {
		t.Fatal(err)
	}
	if told() != 1 {
		t.Fatalf("a second check at the same level told again: %d", told())
	}
}

func TestTheNoticeSweepsRunInTheGoTick(t *testing.T) {
	p, _, _, _ := wired(t)
	d := p.db
	// alice is closed, so no digest is typed to her: the stopped sweep's mark says whether the sweeps ran
	if err := dbpkg.SetAgentState(d, "alice", "STALLED", 0, "gone wrong"); err != nil {
		t.Fatal(err)
	}
	tk := p.Tick(cfgOf(p), TickConfig{})
	for _, s := range tk.Tick.Passes {
		if s.Name == "notice-digest" {
			if err := s.Run(d, time.Now().Unix()); err != nil {
				t.Fatal(err)
			}
		}
	}
	marked, err := dbpkg.Cursor(d, "stopped_told:fatal:alice")
	if err != nil {
		t.Fatal(err)
	}
	if marked == 0 {
		t.Fatal("the notice sweep left no stopped mark")
	}
}

func TestTheStartupBringsUpWaitingWorkAndSettlesAnOldToldSayButNotAnOwedOneBeforeAnyTick(t *testing.T) {
	p, _, _, _ := wired(t)
	d := p.db
	say, err := dbpkg.AddRequest(d, dbpkg.NewRequest{Requester: "carol", Target: "bob", Body: "old", Kind: "say"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpkg.MarkDelivered(d, say); err != nil { // told by the daemon before
		t.Fatal(err)
	}
	owed, err := dbpkg.AddRequest(d, dbpkg.NewRequest{Requester: "carol", Target: "bob", Body: "sent just before the restart", Kind: "say"})
	if err != nil {
		t.Fatal(err)
	}
	tk := p.Tick(cfgOf(p), TickConfig{Terminals: true})
	tk.Startup()
	if row, _ := dbpkg.GetAgentState(d, "alice"); row == nil || row.Terminal.String == "" {
		t.Fatalf("alice has work waiting and no session: startup wakes her in a terminal without waiting for a tick: %+v", row)
	}
	if r, _ := dbpkg.GetRequest(d, say); r == nil || r.State == "queued" {
		t.Fatalf("an old told say is settled at start: %+v", r)
	}
	if r, _ := dbpkg.GetRequest(d, owed); r == nil || r.State != "queued" {
		t.Fatalf("a say not yet told is owed after a restart, not settled: %+v", r)
	}
}

func TestTheTickTypesAnAgentsScheduledMessageWhenItIsDue(t *testing.T) {
	p, _, _, _ := wired(t)
	d := p.db
	os.WriteFile(filepath.Join(p.home, "agents", "alice", "agent.yaml"),
		[]byte("vendor: claude\nschedules: '[{\"cron\": \"* * * * *\", \"message\": \"check the queue\"}]'\n"), 0o644)
	tk := p.Tick(cfgOf(p), TickConfig{})
	var run func(q dbpkg.Q, now int64) error
	for _, s := range tk.Tick.Passes {
		if s.Name == "schedules" {
			run = s.Run
		}
	}
	if run == nil {
		t.Fatal("the tick has no schedules pass")
	}
	t0 := time.Now().Unix()
	t0 -= t0 % 60
	if err := run(d, t0); err != nil {
		t.Fatal(err)
	}
	if err := run(d, t0+120); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := d.QueryRow("SELECT body FROM requests WHERE requester = 'schedule' AND target = 'alice'").Scan(&body); err != nil || body != "[schedule * * * * *] check the queue" {
		t.Fatalf("%q %v", body, err)
	}
}

package procload

import (
	"database/sql"
	"encoding/binary"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/handline/handline/go/scheduler/sysres"
)

func fakeEnv(m map[int]string) func(int) (string, bool) {
	return func(pid int) (string, bool) { a, ok := m[pid]; return a, ok }
}

func tracker(rows *[]Raw, env map[int]string, panes map[string]int) *Tracker {
	return &Tracker{
		Table:   func() ([]Raw, error) { return *rows, nil },
		AgentOf: fakeEnv(env),
		Panes:   func() map[string]int { return panes },
		Machine: func() (sysres.Machine, error) { return sysres.Machine{}, nil },
		Disk:    func(string) (int64, int64, error) { return 0, 0, nil },
	}
}

func find(v View, pid int) Proc {
	for _, p := range v.Processes {
		if p.PID == pid {
			return p
		}
	}
	for _, p := range v.Unowned.Top {
		if p.PID == pid {
			return p
		}
	}
	return Proc{}
}

func TestOwnership(t *testing.T) {
	rows := []Raw{
		{PID: 100, PPID: 1, Start: 1},   // alice's pane
		{PID: 101, PPID: 100, Start: 1}, // under the pane, environment cleared: tree
		{PID: 102, PPID: 101, Start: 1}, // under that: tree
		{PID: 200, PPID: 1, Start: 1},   // reparented to init, still carries the marker: env
		{PID: 201, PPID: 200, Start: 1}, // child of the env-owned one, environment cleared: tree
		{PID: 300, PPID: 1, Start: 1},   // nobody's
		{PID: 400, PPID: 300, Start: 1}, // under nobody's
		{PID: 500, PPID: 100, Start: 1}, // under alice's pane but marked bob: the environment wins
	}
	env := map[int]string{200: "alice", 500: "bob", 101: "", 300: ""}
	tr := tracker(&rows, env, map[string]int{"alice": 100})
	if err := tr.Sample(1000); err != nil {
		t.Fatal(err)
	}
	v, _ := tr.View()
	want := map[int][2]string{101: {"alice", ViaTree}, 102: {"alice", ViaTree}, 200: {"alice", ViaEnv}, 201: {"alice", ViaTree},
		300: {"", ""}, 400: {"", ""}, 500: {"bob", ViaEnv}, 100: {"alice", ViaTree}}
	for pid, w := range want {
		p := find(v, pid)
		if p.PID != pid || p.Agent != w[0] || p.Via != w[1] {
			t.Errorf("%d: %+v, want %v", pid, p, w)
		}
	}
	if v.Unowned.Processes != 2 {
		t.Errorf("unowned %d", v.Unowned.Processes)
	}
	if v.Total.Processes != 8 {
		t.Errorf("total %d: the machine total must add up", v.Total.Processes)
	}
}

func TestCPUIsTheChangeInCPUTimeOverTheTime(t *testing.T) {
	rows := []Raw{{PID: 1000, PPID: 1, Start: 900, CPUSecs: 50}}
	tr := tracker(&rows, map[int]string{1000: "a"}, nil)
	tr.Sample(1000)
	v, _ := tr.View()
	if got := find(v, 1000).CPU; got < 49.9 || got > 50.1 { // new: 50s over its 100s of life
		t.Errorf("first sample %v, want 50", got)
	}
	rows[0].CPUSecs = 55 // 5s of CPU in 5s: one core
	tr.Sample(1005)
	v, _ = tr.View()
	if got := find(v, 1000).CPU; got < 99.9 || got > 100.1 {
		t.Errorf("%v, want 100", got)
	}
	rows[0].CPUSecs = 55 // idle
	tr.Sample(1010)
	v, _ = tr.View()
	if got := find(v, 1000).CPU; got != 0 {
		t.Errorf("%v, want 0", got)
	}
}

func TestAnEndedProcessKeepsItsLastRowUntilItRollsOff(t *testing.T) {
	rows := []Raw{{PID: 7, PPID: 1, Start: 1, CPUSecs: 10, RSS: 1 << 20}}
	tr := tracker(&rows, map[int]string{7: "a"}, nil)
	tr.Sample(1000)
	rows = nil
	tr.Sample(1005)
	v, _ := tr.View()
	p := find(v, 7)
	if !p.Gone || p.Agent != "a" || p.At != 1000 {
		t.Fatalf("%+v", p)
	}
	if len(v.Agents) != 1 || v.Agents[0].Processes != 0 {
		t.Errorf("an ended process counts as running: %+v", v.Agents)
	}
	tr.Sample(1000 + int64(Keep/time.Second) + 1)
	v, _ = tr.View()
	if find(v, 7).PID != 0 {
		t.Error("it did not roll off")
	}
}

func TestTheRingKeepsTheLastKeep(t *testing.T) {
	rows := []Raw{{PID: 7, PPID: 1, Start: 1}}
	tr := tracker(&rows, nil, nil)
	n := int(Keep/Every) + 10
	for i := 0; i < n; i++ {
		tr.Sample(int64(1000 + i*5))
	}
	if len(tr.ring) != int(Keep/Every) || tr.ring[0].At != int64(1000+10*5) {
		t.Errorf("%d samples from %d", len(tr.ring), tr.ring[0].At)
	}
}

func TestTheMinuteRollupIsWrittenWhenTheMinuteEndsAndPrunedAfterAWeek(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.Exec(`CREATE TABLE load_minutes (minute INTEGER NOT NULL, agent TEXT NOT NULL, cpu_avg REAL NOT NULL, cpu_max REAL NOT NULL, rss_avg INTEGER NOT NULL, rss_max INTEGER NOT NULL, procs_max INTEGER NOT NULL, PRIMARY KEY (minute, agent))`)
	rows := []Raw{{PID: 7, PPID: 1, Start: 1000, CPUSecs: 0, RSS: 100 << 10}}
	tr := tracker(&rows, map[int]string{7: "a"}, nil)
	tr.DB = db
	base := int64(1_700_000_040) // a minute boundary
	tr.Sample(base)
	rows[0].CPUSecs = 10 // 200% over 5s
	tr.Sample(base + 5)
	rows[0].CPUSecs = 10
	tr.Sample(base + 10) // 0%
	rows[0].RSS = 300 << 10
	tr.Sample(base + 60) // the next minute: the first is written
	got, err := tr.Series(base)
	if err != nil {
		t.Fatal(err)
	}
	pts := got["a"]
	if len(pts) != 1 || pts[0].Minute != base || pts[0].CPUMax < 199 || pts[0].CPUMax > 201 || pts[0].RSSMax != 100<<10 || pts[0].ProcsMax != 1 {
		t.Fatalf("%+v", got)
	}
	if avg := pts[0].CPUAvg; avg < 50 || avg > 100 {
		t.Errorf("average %v", avg)
	}
	tr.Sample(base + 8*86400) // a week on: the old row goes
	if got, _ = tr.Series(0); len(got["a"]) != 0 {
		t.Errorf("not pruned: %+v", got)
	}
}

func TestProcargs2(t *testing.T) {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, 2)
	buf = append(buf, []byte("/bin/zsh\x00\x00\x00zsh\x00-l\x00PATH=/bin\x00HANDLINE_AGENT=alice\x00HOME=/h\x00")...)
	if got := envValue(buf, "HANDLINE_AGENT"); got != "alice" {
		t.Errorf("%q", got)
	}
	if got := envValue(buf, "NOPE"); got != "" {
		t.Errorf("%q", got)
	}
	if envValue([]byte{1}, "X") != "" || envValue(buf[:6], "X") != "" {
		t.Error("a short buffer")
	}
}

func TestParseTable(t *testing.T) {
	out := " 12 1 2048 0:01.50 Tue Nov  4 09:10:11 2026 /usr/bin/node server.js --port 3000\n" +
		"junk\n 13 12 100 1-02:03:04 Tue Nov  4 09:10:12 2026 sleep 5\n"
	rows := parseTable(out, time.UTC)
	if len(rows) != 2 || rows[0].PID != 12 || rows[0].RSS != 2048*1024 || rows[0].CPUSecs != 1.5 || rows[0].Cmd != "/usr/bin/node server.js --port 3000" {
		t.Fatalf("%+v", rows)
	}
	if rows[1].CPUSecs != 86400+2*3600+3*60+4 || rows[0].Start != time.Date(2026, 11, 4, 9, 10, 11, 0, time.UTC).Unix() {
		t.Errorf("%+v", rows)
	}
}

func TestAChildStaysItsAgentsAfterItsParentEndsAndItIsReparented(t *testing.T) {
	// macOS does not give the environment of its own tools (sh, sleep), so a daemonized one is known only by who its parent was.
	rows := []Raw{{PID: 100, PPID: 1, Start: 1}, {PID: 101, PPID: 100, Start: 1}, {PID: 102, PPID: 101, Start: 1}}
	tr := tracker(&rows, nil, map[string]int{"alice": 100})
	tr.Sample(1000)
	rows = []Raw{{PID: 100, PPID: 1, Start: 1}, {PID: 102, PPID: 1, Start: 1}} // 101 ended; 102 is init's now
	tr.Sample(1005)
	v, _ := tr.View()
	if p := find(v, 102); p.Agent != "alice" || p.Via != ViaTree {
		t.Errorf("%+v", p)
	}
	rows = []Raw{{PID: 102, PPID: 1, Start: 1}} // even the pane is gone
	tr.Sample(1010)
	v, _ = tr.View()
	if p := find(v, 102); p.Agent != "alice" {
		t.Errorf("%+v", p)
	}
	rows = []Raw{{PID: 102, PPID: 1, Start: 2}} // a new process that took the pid is nobody's
	tr.Sample(1015)
	v, _ = tr.View()
	if v.Unowned.Processes != 1 || v.Unowned.Top[0].PID != 102 {
		t.Errorf("%+v", v.Unowned)
	}
}

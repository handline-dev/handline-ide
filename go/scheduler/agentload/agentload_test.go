package agentload

import (
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/session"
	"github.com/handline/handline/go/scheduler/sysres"
)

// bash keeps a Bash call that ran for ran as the hook path does: its shape (session.HeavyShape) when it counts.
func bash(t *testing.T, q *sql.DB, agent, command string, ran time.Duration, at int64) {
	t.Helper()
	if shape := session.HeavyShape(command, ran); shape != "" {
		if _, err := q.Exec("INSERT INTO heavy_runs(agent, ended, seconds, shape) VALUES(?, ?, ?, ?)", agent, at, int64(ran/time.Second), shape); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTwentyMinutesOfCheckReadHeavierThanTwoGoTests(t *testing.T) {
	q, now := storetest.Open(t), time.Now().Unix()
	bash(t, q, "lander", "git fetch -q && tools/check.sh 2>&1 | tail -20", 20*time.Minute, now-3600)
	bash(t, q, "ui", "cd go && go test ./x", 8*time.Second, now-7200)
	bash(t, q, "ui", "cd go && go test ./x", 9*time.Second, now-60)
	bash(t, q, "ui", "git status", 2*time.Second, now-30)         // neither a preset nor past a minute
	bash(t, q, "ui", "hl task wait T1", 5*time.Minute, now-20)    // a wait
	bash(t, q, "old", "tools/check.sh", time.Hour, now-8*24*3600) // past the week
	lander, err := Of(q, "lander", now)
	if err != nil {
		t.Fatal(err)
	}
	ui, _ := Of(q, "ui", now)
	if lander.HeavyS != 1200 || len(lander.Shapes) != 1 || lander.Shapes[0] != (Shape{"check.sh", 1200, 1}) {
		t.Errorf("lander %+v", lander)
	}
	if ui.HeavyS != 17 || len(ui.Shapes) != 1 || ui.Shapes[0] != (Shape{"@build-test", 17, 2}) {
		t.Errorf("ui %+v", ui)
	}
	all, _ := All(q, now)
	if len(all) != 2 || all[0].Agent != "lander" || all[1].Agent != "ui" {
		t.Errorf("%+v", all)
	}
}

func TestABackgroundShellCountsOnceFromItsStartToItsOutputsLastWrite(t *testing.T) {
	q, now := storetest.Open(t), time.Now().Unix()
	dir := t.TempDir()
	old := session.ClaudeTmp
	session.ClaudeTmp = filepath.Join(dir, "claude-501")
	t.Cleanup(func() { session.ClaudeTmp = old })
	out := filepath.Join(dir, "claude-501", "-w-desk", "sess-1", "tasks", "bjob1.output")
	os.MkdirAll(filepath.Dir(out), 0o755)
	os.WriteFile(out, []byte("ok\n\n[exited with code 0]\n"), 0o644)
	end := time.Unix(now-100, 0)
	os.Chtimes(out, end, end)
	running := filepath.Join(dir, "running.output")
	os.WriteFile(running, []byte("still going\n"), 0o644)
	for _, p := range []string{
		`{"command": "tools/check.sh > /tmp/c.log 2>&1", "job": "bjob1", "session": "sess-1", "output": null}`,
		`{"command": "tools/check.sh", "job": "bjob2", "session": "sess-1", "output": "` + running + `"}`,
	} {
		if _, err := q.Exec("INSERT INTO events(ts, agent, kind, payload) VALUES(?, 'a8', 'background_started', ?)", now-1000, p); err != nil {
			t.Fatal(err)
		}
	}
	for _, at := range []int64{now - 50, now - 10} { // two Stops after it ended: kept once
		if err := KeepBackground(q, "a8", at); err != nil {
			t.Fatal(err)
		}
	}
	l, err := Of(q, "a8", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Shapes) != 1 || l.Shapes[0] != (Shape{"check.sh", 900, 1}) {
		t.Errorf("%+v", l)
	}
}

func TestAnAgentThatBootedASimulatorReadsItsMemory(t *testing.T) {
	q, now := storetest.Open(t), time.Now().Unix()
	bash(t, q, "taro", "xcrun simctl boot 'iPhone 16'", 3*time.Second, now-120)
	const gb = 1 << 30
	rows := []procload.Raw{
		{PID: 10, PPID: 1, Start: now - 3600, RSS: gb / 2, Cmd: "claude"},
		{PID: 50, PPID: 1, Start: now - 110, RSS: gb / 4, Cmd: "/Library/Developer/PrivateFrameworks/CoreSimulator.framework/Resources/bin/launchd_sim /Users/x/Library/Developer/CoreSimulator/Devices/U/data/var/run/launchd_bootstrap.plist"},
		{PID: 51, PPID: 50, Start: now - 100, RSS: 2 * gb, Cmd: "/Library/Developer/CoreSimulator/Volumes/iOS/SpringBoard"},
		{PID: 60, PPID: 1, Start: now - 100, RSS: gb, Cmd: "/usr/sbin/somethingelse"},
	}
	tr := &procload.Tracker{
		Table:       func() ([]procload.Raw, error) { return rows, nil },
		AgentOf:     func(int) (string, bool) { return "", true },
		Panes:       func() map[string]int { return map[string]int{"taro": 10} },
		Machine:     func() (sysres.Machine, error) { return sysres.Machine{}, nil },
		Disk:        func(string) (int64, int64, error) { return 0, 0, nil },
		Home:        t.TempDir(),
		DB:          q,
		SimulatorOf: func(start int64) string { a, _ := SimulatorOf(q, start); return a },
	}
	minute := now - now%60
	tr.Sample(minute)
	tr.Sample(minute + 60) // the first minute is written
	l, err := Of(q, "taro", minute+60)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(gb/2 + gb/4 + 2*gb); l.MemoryPeak != want || !slices.Equal(l.Started, []string{"@simulator"}) {
		t.Errorf("peak %d, want %d; started %v", l.MemoryPeak, want, l.Started)
	}
	if a, _ := SimulatorOf(q, now+3600); a != "" {
		t.Errorf("a simulator begun an hour after the last simulator command is %s's", a)
	}
}

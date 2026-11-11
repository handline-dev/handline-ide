package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/scheduler/pause"
)

// healthOfStatus is the health a status.changed carried.
func healthOfStatus(st map[string]any) (level, text string) {
	h := st["health"].(map[string]any)
	return h["level"].(string), h["text"].(string)
}

// The health check tells status.changed only when health's level or text moved: the founder's pause, then an agent an account holds.
func TestTheHealthCheckTellsAChangeOnceAndNothingElse(t *testing.T) {
	d := startStream(t)
	f := readAll(t, pipeTo(t, d, "k"), 2, 3)
	if r := f.ask(2, `{"jsonrpc":"2.0","id":2,"method":"daemon.status","params":{}}`)["result"].(map[string]any); r["health"].(map[string]any)["level"] != "ok" {
		t.Fatalf("health %v", r["health"])
	}
	f.ask(3, `{"jsonrpc":"2.0","id":3,"method":"subscribe","params":{"topics":["status"]}}`)
	if d.checkHealth() {
		t.Fatal("told with nothing changed since the start")
	}
	st, err := pause.Set(d.db, time.Now(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !d.checkHealth() {
		t.Fatal("a pause was not told")
	}
	if level, text := healthOfStatus(f.statusEvent(5 * time.Second)); level != "held" || text != pause.Text(st) {
		t.Fatalf("%s %q", level, text)
	}
	if d.checkHealth() {
		t.Fatal("told twice")
	}
	pause.Clear(d.db)
	teamtest.Seed(t, d.cfg.Home, "bob")
	if _, err := d.db.Exec(`INSERT INTO agent_view(agent, view, updated) VALUES(?, '{"state":"queued","status":{"state":"stopped","reason":"cap","text":"waiting for a slot"}}', 1)`, teamtest.Key(t, d.cfg.Home, "bob")); err != nil {
		t.Fatal(err)
	}
	if !d.checkHealth() {
		t.Fatal("an agent an account holds was not told")
	}
	if level, text := healthOfStatus(f.statusEvent(5 * time.Second)); level != "held" || text != "1 agent waits for an account: no free slot" {
		t.Fatalf("%s %q", level, text)
	}
}

// A status.changed the pause told keeps its health as told, so the check does not tell it again.
func TestAHealthThePauseToldIsNotToldAgain(t *testing.T) {
	d := startStream(t)
	if _, err := pause.Set(d.db, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	d.statusChanged()
	if d.checkHealth() {
		t.Fatal("told again")
	}
}

// The daemon looks at health by itself, every Config.HealthEvery.
func TestTheDaemonLooksAtHealthByItself(t *testing.T) {
	cfg := tlsConfig(t, t.TempDir())
	cfg.Auth, cfg.HealthEvery = pidAuth{}, 20*time.Millisecond
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	f := readAll(t, pipeTo(t, d, "k"), 2)
	f.ask(2, `{"jsonrpc":"2.0","id":2,"method":"subscribe","params":{"topics":["status"]}}`)
	if _, err := pause.Set(d.db, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	if level, _ := healthOfStatus(f.statusEvent(5 * time.Second)); level != "held" {
		t.Fatalf("level %s", level)
	}
}

// Every server of the daemon reads health's since from one clock, so the socket and the network say the same.
func TestEveryServerSharesTheHealthClock(t *testing.T) {
	d := startStream(t)
	if a, b := d.readsEnv().HealthClock, d.readsEnv().HealthClock; a == nil || a != b {
		t.Fatalf("%p %p", a, b)
	}
}

// keep_awake on daemon.status says the mode config.yaml sets and whether the sleep lock is held; the check tells a change of either once.
func TestTheHealthCheckTellsAChangeOfKeepAwake(t *testing.T) {
	cfg := tlsConfig(t, t.TempDir())
	var held atomic.Bool
	since := time.Unix(1_800_000_000, 0)
	cfg.Auth, cfg.KeepAwake = pidAuth{}, func() (bool, time.Time) { return held.Load(), since }
	d, err := Start(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	f := readAll(t, pipeTo(t, d, "k"), 2, 3)
	r := f.ask(2, `{"jsonrpc":"2.0","id":2,"method":"daemon.status","params":{}}`)["result"].(map[string]any)
	if fmt.Sprint(r["keep_awake"]) != "map[held:false mode:off]" {
		t.Fatalf("%v", r["keep_awake"])
	}
	f.ask(3, `{"jsonrpc":"2.0","id":3,"method":"subscribe","params":{"topics":["status"]}}`)
	if d.checkHealth() {
		t.Fatal("told with nothing changed since the start")
	}
	held.Store(true)
	if !d.checkHealth() || d.checkHealth() {
		t.Fatal("the lock taken is told once")
	}
	if a := f.statusEvent(5 * time.Second)["keep_awake"]; fmt.Sprint(a) != "map[held:true mode:off since:2027-01-15T08:00:00Z]" {
		t.Fatalf("%v", a)
	}
	if err := os.WriteFile(filepath.Join(d.cfg.Home, "config.yaml"), []byte("keep_awake: working\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !d.checkHealth() {
		t.Fatal("a new mode is told")
	}
	if a := f.statusEvent(5 * time.Second)["keep_awake"].(map[string]any); a["mode"] != "working" {
		t.Fatalf("%v", a)
	}
}

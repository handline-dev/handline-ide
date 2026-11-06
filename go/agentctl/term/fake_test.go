//go:build fakes

package term

import (
	"strings"
	"testing"
	"time"
)

func waitScreen(t *testing.T, f Fake, h, want string) []string {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if s := f.ReadScreen(h, 10); s != nil && strings.Contains(strings.Join(s.Tail, "\n"), want) {
			return s.Tail
		}
	}
	t.Fatalf("%q never on screen: %v", want, f.ReadScreen(h, 10))
	return nil
}

func TestAFakePaneRunsTheCommandInAShellAndShowsWhatItPrints(t *testing.T) {
	home := t.TempDir()
	b := ForName("fake", Place{Key: home, Env: []string{"HANDLINE_HOME=" + home}})
	f, ok := b.(Fake)
	if !ok || b.Name() != "fake" || !b.Available() {
		t.Fatalf("fake: %#v", b)
	}
	h, why := f.CreateTerminal(`printf 'home %s\n' "$HANDLINE_HOME"; while read -r l; do printf 'got %s\n' "$l"; done`, home, "t", "a.one")
	if !paneRE.MatchString(h) || why != "" {
		t.Fatalf("%q %q", h, why)
	}
	waitScreen(t, f, h, "home "+home)
	if ok, why := f.Send(h, "hello", true); !ok {
		t.Fatal(why)
	}
	waitScreen(t, f, h, "got hello")
	if !f.ListTerminals()[h] || f.PanePid(h) == 0 {
		t.Fatalf("listed %v pid %d", f.ListTerminals(), f.PanePid(h))
	}
	if a, known := f.PaneAgent(h); a != "a.one" || !known {
		t.Fatalf("agent %q %v", a, known)
	}
	if !f.WaitIdle(h, 2000, func(tail []string) bool { return false }) || f.WaitIdle(h, 300, func([]string) bool { return true }) {
		t.Fatal("idle is quiet with no turn on the screen")
	}
	if ok, _ := f.RespawnTerminal(h); !ok {
		t.Fatal("respawn")
	}
	f.Send(h, "echo fresh", true)
	waitScreen(t, f, h, "fresh")
	if ok, _ := f.CloseTerminal(h, true); !ok || f.ListTerminals()[h] || f.ReadScreen(h, 5) != nil {
		t.Fatal("a closed pane is gone")
	}
	if ok, _ := f.Send(h, "x", true); ok {
		t.Fatal("typing into a closed pane")
	}
	if ForName("fake", Place{Key: t.TempDir()}).ListTerminals()[h] {
		t.Fatal("panes are per key")
	}
}

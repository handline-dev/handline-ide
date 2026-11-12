package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/agentctl/term"
)

// capture is a screen of codex-cli 0.157.1 captured from tmux at 120x40 (testdata/<name>.txt), one row per line.
func capture(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func TestWorkingOnCapturedScreens(t *testing.T) {
	for _, c := range []struct {
		screen         string
		working, known bool
	}{
		{"idle", false, true},       // the header box and nothing said yet
		{"typed", false, true},      // a message typed, not yet sent
		{"busy", true, true},        // • Working (0s • esc to interrupt)
		{"streaming", false, false}, // the reply is being written: no Working row, no end time
		{"done", false, true},       // the turn's end time under the reply
		{"approval", false, false},  // a dialog, no composer
		{"login", false, false},
	} {
		w, k := Screen{}.WorkingOnScreen(capture(t, c.screen))
		if w != c.working || k != c.known {
			t.Errorf("%s: working %v known %v, want %v %v", c.screen, w, k, c.working, c.known)
		}
		if (Screen{}).TurnOnScreen(capture(t, c.screen)) != c.working {
			t.Errorf("%s: TurnOnScreen disagrees", c.screen)
		}
	}
}

// A turn is over once the composer is back under its end: "■ Conversation interrupted", which wraps, or from 0.167 how long it
// worked and the time, in one row.
func TestATurnEndIsOver(t *testing.T) {
	for name, tail := range map[string][]string{
		"interrupted": {"• Ran ls", "", "■ Conversation interrupted - tell the model what to do differently. Something went wrong? Hit `/feedback` to report the",
			"issue.", "", "      Tip: Press f3 to search this conversation.", "", "› Ask Codex to do anything", "", "  GPT-5.6-Luna low · /w", "  ? for shortcuts"},
		"0.167 end": {"• Ran python3 probe.py", "  └ unix hello", "", "• unix hello", "", "  Worked for 7s • 00:54", "", "› Ask Codex to do anything", "",
			"  GPT-6-Luna low · /w · Run probe script", "  ← for agents · ? for shortcuts"},
	} {
		if w, k := (Screen{}).WorkingOnScreen(tail); w || !k {
			t.Errorf("%s: working %v known %v", name, w, k)
		}
	}
}

func TestBlockers(t *testing.T) {
	for _, s := range []string{"idle", "typed", "busy", "streaming", "done"} {
		if b := (Screen{}).ScreenBlocker(capture(t, s)); b != nil {
			t.Errorf("%s: %+v", s, b)
		}
	}
	for _, c := range []struct{ screen, title, detail string }{
		{"approval", "Would you like to run the following command?", "Would you like to run the following command? / $ touch ~/cx-probe-outside / 1. Yes, proceed (y) / " +
			"2. Yes, and don't ask again for commands that start with `touch ~/cx-probe-outside` (p) / 3. No, and tell Codex what to do differently (esc)"},
		{"login", "Welcome to Codex, OpenAI's command-line coding agent", "Welcome to Codex, OpenAI's command-line coding agent / 1. Sign in with ChatGPT / " +
			"2. Sign in with Device Code / 3. Provide your own API key"},
	} {
		b := (Screen{}).ScreenBlocker(capture(t, c.screen))
		if b == nil || b.Kind != term.Modal || b.Title != c.title || b.Detail != term.Cut(c.detail, term.DetailChars) {
			t.Errorf("%s: %+v", c.screen, b)
		}
	}
	if b := (Screen{}).ScreenBlocker(capture(t, "trust")); b == nil || b.Kind != term.Modal || !strings.HasPrefix(b.Title, "Trust this folder?") {
		t.Errorf("trust: %+v", b)
	}
	// S1's notes of the screens not captured here: each is a dialog, quoted.
	update := []string{"  ✨ Update available · 0.157.1 → 0.167.2", "", "› 1. Update now", "  2. Skip", "  3. Skip until next version", "", "  Press enter to continue"}
	if b := (Screen{}).ScreenBlocker(update); b == nil || b.Kind != term.Modal || b.Detail != "✨ Update available · 0.157.1 → 0.167.2 / 1. Update now / 2. Skip / 3. Skip until next version" {
		t.Errorf("update: %+v", b)
	}
	if b := (Screen{}).ScreenBlocker([]string{"some output", "", "more"}); b == nil || b.Kind != term.Unreadable {
		t.Errorf("no composer, no dialog: %+v", b)
	}
}

// The trust screen's folder wraps with no break; the inline projects table on the launch line keeps it from showing at all.
func TestTrustScreen(t *testing.T) {
	folder, shown := Screen{}.TrustScreen(capture(t, "trust"))
	want := "/private/tmp/claude-501/-Users-me-src-workspaces-project-handline-wt/56c22399-e8c7-453a-9bef-2c356b425f00/scratchpad/cx/wd"
	if !shown || folder != want {
		t.Fatalf("%q %v", folder, shown)
	}
	if _, shown := (Screen{}).TrustScreen(capture(t, "idle")); shown {
		t.Fatal("idle shows no trust screen")
	}
	if (Screen{}).TrustAnswer() != "" {
		t.Fatal("Trust and continue is already chosen: Enter alone")
	}
}

// A draft that starts with a numbered item is the composer, not a dialog's chosen option.
func TestADraftThatLooksLikeAnOption(t *testing.T) {
	tail := capture(t, "idle")
	for i, ln := range tail {
		if strings.HasPrefix(ln, "› Ask Codex") {
			tail[i] = "› 1. fix the build"
		}
	}
	if b := (Screen{}).ScreenBlocker(tail); b != nil {
		t.Fatalf("%+v", b)
	}
}

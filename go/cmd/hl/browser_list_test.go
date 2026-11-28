package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/internal/browser"
)

// Two pages, newest first as Chrome lists them.
const twoTabs = `[{"id":"p2","type":"page","title":"Docs","url":"https://example.com/docs"},
{"id":"p1","type":"page","title":"Home","url":"https://example.com/"}]`

func TestBrowserListSaysNoBrowserOpenAndStartsNone(t *testing.T) {
	r := newRig(t)
	calls := r.daemon(nil)
	out, errs, code := r.run(asAgent, "browser", "list")
	if code != 0 || out != "no browser open\n" || errs != "" {
		t.Fatalf("exit %d, %q %q", code, out, errs)
	}
	for _, c := range sent(calls()) {
		if strings.HasPrefix(c, "screen.") {
			t.Errorf("a closed browser asked %s", c)
		}
	}
}

func TestBrowserListShowsTheTabsAndWhoHasThePageWithoutActing(t *testing.T) {
	r := newRig(t)
	aBrowserRuns(t, r.home, "a.one", twoTabs)
	const tabsOut = "* 1  Home  https://example.com/\n  2  Docs  https://example.com/docs\n"
	for _, c := range []struct{ status, head, last string }{
		{`{"holder":{"kind":"agent","name":"a.one","since":null},"watchers":[],"machine":null}`, "Browser: headless, on this Mac\n", ""},
		{`{"holder":{"kind":"agent","name":"a.one","since":null},"watchers":["Kim","Ron"],"machine":"Mac mini"}`, "Browser: headless, on Mac mini\n",
			"watched by Kim, Ron\n"},
		{`{"holder":{"kind":"person","name":"Ron","since":"2026-11-28T09:00:00Z"},"watchers":["Ron"],"machine":null}`,
			"Browser: headless, on this Mac\n", "Ron has the page\n"},
		{`{"holder":{"kind":"device","name":"Ron's iPhone","since":"2026-11-28T09:00:00Z"},"watchers":[],"machine":null}`,
			"Browser: headless, on this Mac\n", "Ron's iPhone has the page\n"},
	} {
		calls := r.daemon(map[string]string{"screen.status": c.status, "screen.act": "error[input_holder]:Ron has the page"})
		out, errs, code := r.run(asAgent, "browser", "list")
		if want := c.head + tabsOut + c.last; code != 0 || out != want || errs != "" {
			t.Errorf("%s: exit %d, %q %q", c.status, code, out, errs)
		}
		for _, s := range sent(calls()) {
			if strings.HasPrefix(s, "screen.act") {
				t.Errorf("list acted: %s", s)
			}
		}
	}
	r.daemon(map[string]string{"screen.status": "error:the daemon is starting"})
	out, errs, code := r.run(asAgent, "browser", "list")
	if code != 1 || out != "Browser: headless\n"+tabsOut || !strings.Contains(errs, "who has it are not known") {
		t.Errorf("status refused: exit %d, %q %q", code, out, errs)
	}
	state := filepath.Join(browser.AgentDir(r.home, "a.one"), "state.json")
	var kept map[string]any
	if b, err := os.ReadFile(state); err != nil || json.Unmarshal(b, &kept) != nil {
		t.Fatal(err)
	}
	kept["show"] = true
	b, _ := json.Marshal(kept)
	if err := os.WriteFile(state, b, 0o600); err != nil {
		t.Fatal(err)
	}
	r.daemon(map[string]string{"screen.status": `{"holder":{"kind":"agent","name":"a.one","since":null},"watchers":[],"machine":null}`})
	if out, _, _ := r.run(asAgent, "browser", "list"); !strings.HasPrefix(out, "Browser: shown, on this Mac\n") {
		t.Errorf("shown: %q", out)
	}
}

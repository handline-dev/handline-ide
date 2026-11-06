package session

import (
	"strings"
	"testing"
)

func TestLeaseStarter(t *testing.T) {
	for cmd, want := range map[string]string{
		"xcrun simctl boot 'iPhone 16'": "simulator",
		"open -a Simulator":             "simulator",
		"open -a /Applications/Xcode.app/Contents/Developer/Applications/Simulator.app":    "simulator",
		"xcodebuild test -scheme App -destination 'platform=iOS Simulator,name=iPhone 16'": "simulator",
		"emulator -avd Pixel_8":                        "simulator",
		"$ANDROID_HOME/emulator/emulator @Pixel_8":     "",
		"timeout 600 emulator @Pixel_8":                "simulator",
		"cd web && npx playwright test":                "browser",
		"pnpm exec playwright test --project chromium": "browser",
		"pnpm --filter web exec playwright test":       "browser",
		"'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' --headless=new --screenshot x.png": "browser",
		"chromium --headless --dump-dom https://example.com":                                               "browser",
		"bash -c 'npx playwright test'":                                                                    "browser",
		"env CI=1 npx playwright test":                                                                     "browser",
		"nice -n 10 chromium --headless":                                                                   "browser",
		"sudo -u ci emulator -avd X":                                                                       "simulator",
		// Not starters.
		"npx playwright install chromium":                      "",
		"pnpm add -D playwright":                               "",
		"xcrun simctl list devices":                            "",
		"xcodebuild build -destination 'generic/platform=iOS'": "",
		"open -a Safari":                                       "",
		"echo 'npx playwright test'":                           "",
		"git commit -m \"emulator -avd x\"":                    "",
		"hl use browser -- npx playwright test":                "",
		"hl use simulator -- xcrun simctl boot X":              "",
		"cat <<'EOF'\nnpx playwright test\nEOF":                "",
	} {
		if got, _ := LeaseStarter(cmd); got != want {
			t.Errorf("%q: %q, want %q", cmd, got, want)
		}
	}
}

func TestLeaseGuardRefusesOnlyWithALimitAndNoHold(t *testing.T) {
	in := map[string]any{"command": "npx playwright test"}
	browsers := func(k string) int {
		if k == "browser" {
			return 2
		}
		return 0
	}
	no := func(string) bool { return false }
	if why := LeaseGuard("Bash", in, func(string) int { return 0 }, no); why != "" {
		t.Fatalf("no limit refused: %s", why)
	}
	if why := LeaseGuard("Bash", in, browsers, no); why != "Refused: this starts a browser outside hl use while at most 2 may run at once; nothing ran. Run hl use browser -- npx playwright test." {
		t.Fatalf("refusal: %s", why)
	}
	if why := LeaseGuard("Bash", in, browsers, func(k string) bool { return k == "browser" }); why != "" {
		t.Fatalf("refused under the agent's own hold: %s", why)
	}
	if why := LeaseGuard("Bash", map[string]any{"command": "xcrun simctl boot X"}, browsers, no); why != "" {
		t.Fatalf("a simulator refused under a browser limit: %s", why)
	}
	boot := LeaseGuard("Bash", map[string]any{"command": "xcrun simctl boot X"}, func(string) int { return 1 }, no)
	if !strings.HasSuffix(boot, "Run hl use simulator --hold, then xcrun simctl boot X, then hl use --release.") {
		t.Fatalf("a boot that returns at once: %s", boot)
	}
	if why := LeaseGuard("Read", in, func(string) int { return 1 }, no); why != "" {
		t.Fatalf("another tool refused: %s", why)
	}
}

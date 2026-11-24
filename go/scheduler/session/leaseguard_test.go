package session

import (
	"strings"
	"testing"
)

func TestPreset(t *testing.T) {
	for _, c := range []struct{ cmd, want string }{
		{"xcrun simctl boot 'iPhone 16'", "@simulator"},
		{"open -a Simulator", "@simulator"},
		{"open -a /Applications/Xcode.app/Contents/Developer/Applications/Simulator.app", "@simulator"},
		{"xcodebuild test -scheme App -destination 'platform=iOS Simulator,name=iPhone 16'", "@simulator"},
		{"emulator -avd Pixel_8", "@simulator"},
		{"$ANDROID_HOME/emulator/emulator @Pixel_8", ""},
		{"timeout 600 emulator @Pixel_8", "@simulator"},
		{"cd web && npx playwright test", "@browser"},
		{"hl browser open http://localhost:5173/", "@browser"},
		{"hl browser close", ""},
		{"pnpm exec playwright test --project chromium", "@browser"},
		{"pnpm --filter web exec playwright test", "@browser"},
		{"'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' --headless=new --screenshot x.png", "@browser"},
		{"chromium --headless --dump-dom https://example.com", "@browser"},
		{"bash -c 'npx playwright test'", "@browser"},
		{"env CI=1 npx playwright test", "@browser"},
		{"nice -n 10 chromium --headless", "@browser"},
		{"sudo -u ci emulator -avd X", "@simulator"},
		{"go test ./...", "@build-test"},
		{"cd go && go build ./cmd/x", "@build-test"},
		{"cargo nextest run", "@build-test"},
		{"swift test", "@build-test"},
		{"./gradlew :app:assembleDebug", "@build-test"},
		{"mvn -q package", "@build-test"},
		{"bazel test //...", "@build-test"},
		{"pytest -x tests", "@build-test"},
		{"pnpm test", "@build-test"},
		{"npm run build", "@build-test"},
		{"pnpm --filter web exec vitest run", "@build-test"},
		{"npx jest", "@build-test"},
		{"xcodebuild build -destination 'generic/platform=iOS'", "@build-test"},
		{"timeout 900 go test -race ./...", "@build-test"},
		{"cargo +nightly test", "@build-test"},
		{"bazel --output_base=/tmp/b test //...", "@build-test"},
		{"go -C go test ./...", "@build-test"},
		{"xcodebuild -scheme App", "@build-test"},
		{"xcodebuild build-for-testing -scheme App", "@build-test"},
		{"python3 -m pytest -q", "@build-test"},
		{"npm run test:unit", "@build-test"},
		{"npm --prefix web test", "@build-test"},
		{"yarn workspace web test", "@build-test"},
		{"bun test", "@build-test"},
		{"xcodebuild -scheme App CODE_SIGNING_ALLOWED=NO", "@build-test"},
		{"xcodebuild -scheme App -jobs 4", "@build-test"},
		{"mvn -B -V verify", "@build-test"},
		{"npm -w web test", "@build-test"},
		{"npm --workspace web run test", "@build-test"},
		{"python -B -m pytest", "@build-test"},
		{"uv run pytest", "@build-test"},
		{"poetry run pytest -q", "@build-test"},
		{"cargo --color always test", "@build-test"},
		// Not starters.
		{"go vet ./...", ""},
		{"go mod tidy", ""},
		{"cargo fmt", ""},
		{"pnpm add -D vitest", ""},
		{"npm install", ""},
		{"mvn -v", ""},
		{"gradle -p build tasks", ""},
		{"pytest --version", ""},
		{"xcodebuild -list", ""},
		{"xcodebuild -resolvePackageDependencies -project X.xcodeproj", ""},
		{"gradle clean -x test", ""},
		{"pnpm -w add vitest", ""},
		{"python3 -m http.server", ""},
		{"hl use build -- go test ./...", ""},
		{"npx playwright install chromium", ""},
		{"pnpm add -D playwright", ""},
		{"xcrun simctl list devices", ""},
		{"open -a Safari", ""},
		{"echo 'npx playwright test'", ""},
		{"git commit -m \"emulator -avd x\"", ""},
		{"hl use browser -- npx playwright test", ""},
		{"hl use simulator -- xcrun simctl boot X", ""},
		{"cat <<'EOF'\nnpx playwright test\nEOF", ""}} {
		cmd, want := c.cmd, c.want
		got := ""
		for _, w := range Commands(cmd) {
			if got = preset(w); got != "" {
				break
			}
		}
		if got != want {
			t.Errorf("%q: %q, want %q", cmd, got, want)
		}
	}
}

func TestCoversByWordPrefixPathAndPreset(t *testing.T) {
	for _, c := range []struct {
		cmd, line string
		want      bool
	}{
		{"xcodebuild", "xcodebuild -scheme App build", true},
		{"xcodebuild", "/usr/bin/xcodebuild build", true},
		{"xcodebuild", "xcrun simctl boot X", false},
		{"xcodebuild", "xcodebuild -version", false},
		{"xcodebuild", "xcodebuild -list -project A.xcodeproj", false},
		{"tools/check.sh", "bash -x tools/check.sh", true},
		{"go test", "go test ./...", true},
		{"go test", "go -C go test ./x", true},
		{"go test", "go build ./...", false},
		{"go test", "gofmt -l .", false},
		{"pnpm test", "pnpm test --filter web", true},
		{"tools/check.sh", "tools/check.sh origin/main", true},
		{"tools/check.sh", "./tools/check.sh", true},
		{"tools/check.sh", "bash tools/check.sh origin/main", true},
		{"tools/check.sh", "/Users/x/repo/tools/check.sh", true},
		{"tools/check.sh", "other/tools/check.sh.bak", false},
		{"tools/check.sh", "cat tools/check.sh", false},
		{"tools/check.sh origin/main", "tools/check.sh origin/main", true},
		{"tools/check.sh origin/main", "tools/check.sh", false},
		{"@browser", "npx playwright test", true},
		{"@browser", "npx playwright install", false},
		{"@simulator", "xcrun simctl boot X", true},
		{"@build-test", "cargo test", true},
		{"@build-test", "go version", false},
	} {
		cmds := Commands(c.line)
		if len(cmds) != 1 {
			t.Fatalf("%q: %d commands", c.line, len(cmds))
		}
		if got := Covers(c.cmd, cmds[0]); got != c.want {
			t.Errorf("%q covers %q: %v, want %v", c.cmd, c.line, got, c.want)
		}
	}
}

func TestRunGuardRefusesOnlyACoveredCommandWithNoTurn(t *testing.T) {
	xcode := Run{Kind: "mac:xcode", Name: "xcode", Cmds: []string{"xcodebuild"}, AtOnce: 1, SetBy: "a2", Why: "the fans"}
	in := map[string]any{"command": "cd app && xcodebuild build"}
	no := func(string) bool { return false }
	if why := RunGuard("Bash", in, nil, no); why != "" {
		t.Fatalf("no runs refused: %s", why)
	}
	want := "Refused: xcodebuild build is a command the shared run xcode covers (at most 1 at once, set by a2: the fans), run outside hl use; " +
		"nothing ran. Run hl use xcode -- xcodebuild build."
	if why := RunGuard("Bash", in, []Run{xcode}, no); why != want {
		t.Fatalf("refusal:\n got %s\nwant %s", why, want)
	}
	if why := RunGuard("Bash", in, []Run{xcode}, func(k string) bool { return k == "mac:xcode" }); why != "" {
		t.Fatalf("refused under the agent's own hold: %s", why)
	}
	if why := RunGuard("Bash", map[string]any{"command": "hl use xcode -- xcodebuild build"}, []Run{xcode}, no); why != "" {
		t.Fatalf("refused through hl use: %s", why)
	}
	if why := RunGuard("Bash", map[string]any{"command": "echo 'xcodebuild build'"}, []Run{xcode}, no); why != "" {
		t.Fatalf("a quoted word refused: %s", why)
	}
	if why := RunGuard("Read", in, []Run{xcode}, no); why != "" {
		t.Fatalf("another tool refused: %s", why)
	}
	sim := Run{Kind: "w3:sim", Name: "sim", Cmds: []string{"@simulator"}, AtOnce: 1}
	boot := RunGuard("Bash", map[string]any{"command": "xcrun simctl boot X"}, []Run{xcode, sim}, no)
	if !strings.HasSuffix(boot, "Run hl use sim --hold, then xcrun simctl boot X, then hl use --release.") {
		t.Fatalf("a boot that returns at once: %s", boot)
	}
	both := Run{Kind: "mac:builds", Name: "builds", Cmds: []string{"@build-test"}, AtOnce: 2}
	if why := RunGuard("Bash", in, []Run{both, xcode}, no); !strings.Contains(why, "the shared runs builds and xcode cover") ||
		!strings.HasSuffix(why, "Run hl use builds xcode -- xcodebuild build.") {
		t.Fatalf("two runs: %s", why)
	}
}

func TestRunGuardReadsEveryCommandInAScript(t *testing.T) {
	runs := []Run{{Kind: "mac:b", Name: "b", Cmds: []string{"@browser"}, AtOnce: 1}, {Kind: "mac:t", Name: "t", Cmds: []string{"go test"}, AtOnce: 1}}
	held := map[string]bool{}
	guard := func(cmd string) string {
		return RunGuard("Bash", map[string]any{"command": cmd}, runs, func(k string) bool { return held[k] })
	}
	if got := guard("go build ./x && npx playwright test"); !strings.Contains(got, "shared run b ") {
		t.Fatalf("the browser after a build no run covers: %q", got)
	}
	if got := guard("bash -c 'go test ./x'"); !strings.Contains(got, "shared run t ") {
		t.Fatalf("bash -c: %q", got)
	}
	held["mac:b"] = true
	if got := guard("go build ./x && npx playwright test"); got != "" {
		t.Fatalf("held: %q", got)
	}
}

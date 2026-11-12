package session

import (
	"strings"
	"testing"
)

func TestLeaseStarter(t *testing.T) {
	for _, c := range []struct{ cmd, want string }{
		{"xcrun simctl boot 'iPhone 16'", "simulator"},
		{"open -a Simulator", "simulator"},
		{"open -a /Applications/Xcode.app/Contents/Developer/Applications/Simulator.app", "simulator"},
		{"xcodebuild test -scheme App -destination 'platform=iOS Simulator,name=iPhone 16'", "simulator"},
		{"emulator -avd Pixel_8", "simulator"},
		{"$ANDROID_HOME/emulator/emulator @Pixel_8", ""},
		{"timeout 600 emulator @Pixel_8", "simulator"},
		{"cd web && npx playwright test", "browser"},
		{"pnpm exec playwright test --project chromium", "browser"},
		{"pnpm --filter web exec playwright test", "browser"},
		{"'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' --headless=new --screenshot x.png", "browser"},
		{"chromium --headless --dump-dom https://example.com", "browser"},
		{"bash -c 'npx playwright test'", "browser"},
		{"env CI=1 npx playwright test", "browser"},
		{"nice -n 10 chromium --headless", "browser"},
		{"sudo -u ci emulator -avd X", "simulator"},
		{"go test ./...", "heavy"},
		{"cd go && go build ./cmd/x", "heavy"},
		{"cargo nextest run", "heavy"},
		{"swift test", "heavy"},
		{"./gradlew :app:assembleDebug", "heavy"},
		{"mvn -q package", "heavy"},
		{"bazel test //...", "heavy"},
		{"pytest -x tests", "heavy"},
		{"pnpm test", "heavy"},
		{"npm run build", "heavy"},
		{"pnpm --filter web exec vitest run", "heavy"},
		{"npx jest", "heavy"},
		{"xcodebuild build -destination 'generic/platform=iOS'", "heavy"},
		{"timeout 900 go test -race ./...", "heavy"},
		{"cargo +nightly test", "heavy"},
		{"bazel --output_base=/tmp/b test //...", "heavy"},
		{"go -C go test ./...", "heavy"},
		{"xcodebuild -scheme App", "heavy"},
		{"xcodebuild build-for-testing -scheme App", "heavy"},
		{"python3 -m pytest -q", "heavy"},
		{"npm run test:unit", "heavy"},
		{"npm --prefix web test", "heavy"},
		{"yarn workspace web test", "heavy"},
		{"bun test", "heavy"},
		{"xcodebuild -scheme App CODE_SIGNING_ALLOWED=NO", "heavy"},
		{"xcodebuild -scheme App -jobs 4", "heavy"},
		{"mvn -B -V verify", "heavy"},
		{"npm -w web test", "heavy"},
		{"npm --workspace web run test", "heavy"},
		{"python -B -m pytest", "heavy"},
		{"uv run pytest", "heavy"},
		{"poetry run pytest -q", "heavy"},
		{"cargo --color always test", "heavy"},
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
		{"hl use heavy -- go test ./...", ""},
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
		if st := LeaseStarters(cmd); len(st) > 0 {
			got = st[0].Kind
		}
		if got != want {
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

func TestLeaseGuardNamesATestOrBuildRun(t *testing.T) {
	heavy := func(k string) int {
		if k == "heavy" {
			return 1
		}
		return 0
	}
	got := LeaseGuard("Bash", map[string]any{"command": "go test ./..."}, heavy, func(string) bool { return false })
	if got != "Refused: this starts a test or build run outside hl use while at most 1 may run at once; nothing ran. Run hl use heavy -- go test ./...." {
		t.Fatalf("got %q", got)
	}
}

func TestLeaseGuardReadsEveryStarterInAScript(t *testing.T) {
	limits := map[string]int{"browser": 1}
	held := map[string]bool{}
	guard := func(cmd string) string {
		return LeaseGuard("Bash", map[string]any{"command": cmd}, func(k string) int { return limits[k] }, func(k string) bool { return held[k] })
	}
	// heavy has no limit, so the build lets the script through only as far as the browser after it.
	if got := guard("go build ./x && npx playwright test"); !strings.Contains(got, "starts a browser") {
		t.Fatalf("no limit on heavy: %q", got)
	}
	limits["heavy"], held["heavy"] = 1, true
	if got := guard("go build ./x && npx playwright test"); !strings.Contains(got, "starts a browser") {
		t.Fatalf("heavy held: %q", got)
	}
	held["browser"] = true
	if got := guard("go build ./x && npx playwright test"); got != "" {
		t.Fatalf("both held: %q", got)
	}
}

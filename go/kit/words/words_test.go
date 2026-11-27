package words

import "testing"

func TestOrdinalAndAnd(t *testing.T) {
	t.Parallel()
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd",
		23: "23rd", 31: "31st", 101: "101st", 111: "111th"} {
		if got := Ordinal(n); got != want {
			t.Errorf("%d: %s, want %s", n, got, want)
		}
	}
	for want, items := range map[string][]string{"": nil, "a": {"a"}, "a and b": {"a", "b"}, "a, b and c": {"a", "b", "c"}} {
		if got := And(items); got != want {
			t.Errorf("%v: %q, want %q", items, got, want)
		}
	}
}

func TestCommandIsTheProgramAndItsFirstWord(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]string{
		`W=/x/desk-2; L=/tmp/s.log; xcodebuild test -project $W/a.xcodeproj -destination "platform=iOS Simulator,id=5" > $L 2>&1; echo "exit $?"; grep -E '\*\* TEST' $L | tail -10`: "xcodebuild test",
		"cd apps/web && npx vite dev --port 5173":        "vite dev",
		"pnpm --dir apps/web dev":                        "pnpm",
		"HANDLINE_GO_RACE=1 tools/test-go.sh > log 2>&1": "test-go.sh",
		"go test ./internal/daemon -run X -count 5":      "go test",
		"nohup timeout 600 go run ./cmd/x serve &":       "go run",
		`bash -c "sleep 3; make build"`:                  "make build",
		"bash tools/check.sh origin/main":                "check.sh",
		"L=$(mktemp); swift build":                       "swift build",
		"sleep 600":                                      "sleep",
		"nice -n 10 make all":                            "make all",
		"sudo -u root launchctl list":                    "launchctl list",
		"cd /x":                                          "cd",
		"  ":                                             "",
	} {
		if got := Command(line); got != want {
			t.Errorf("%q: %q, want %q", line, got, want)
		}
	}
}

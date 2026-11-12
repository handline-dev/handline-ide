package session

import (
	"strings"
	"testing"
)

func TestLandGuardHoldsPushesToTheDefaultBranch(t *testing.T) {
	git := func(dir string, args ...string) string {
		switch strings.Join(args, " ") {
		case "symbolic-ref --short refs/remotes/origin/HEAD":
			return "origin/main\n"
		case "rev-parse --abbrev-ref HEAD":
			if dir == "/on-main" {
				return "main\n"
			}
			return "feature\n"
		}
		return ""
	}
	var ran []string
	check := func(dir, command string) (string, bool) {
		ran = append(ran, dir+": "+command)
		return "line\nFAIL it", command != "false"
	}
	by := Land{By: "a8"}
	both := Land{By: "a8", Check: "make check"}
	for _, c := range []struct {
		name, agent, cwd, command string
		rules                     Land
		deny                      string
	}{
		{"nobody's rule", "a1", "/r", "git push origin HEAD:main", Land{}, ""},
		{"another agent to main", "a1", "/r", "git push origin HEAD:main", by, "only a8 pushes to main"},
		{"the named agent", "a8", "/r", "git push origin HEAD:main", by, ""},
		{"a feature branch", "a1", "/r", "git push -u origin feature", by, ""},
		{"a forced refspec", "a1", "/r", "git push --force origin +HEAD:refs/heads/main", by, "only a8"},
		{"a bare push on main", "a1", "/on-main", "git push", by, "only a8"},
		{"a bare push on a feature", "a1", "/r", "git push origin", by, ""},
		{"a delete is not landing", "a1", "/r", "git push origin --delete main", by, ""},
		{"all branches", "a1", "/r", "git push --all", by, "only a8"},
		{"after a cd", "a1", "/r", "cd /on-main && git push", by, "only a8"},
		{"git -C", "a1", "/r", "git -C /on-main push origin", by, "only a8"},
		{"in a chain", "a1", "/r", "echo go; git push origin main && echo done", by, "only a8"},
		{"not a push", "a1", "/r", "git log main", by, ""},
		{"the check passes", "a8", "/r", "git push origin main", both, ""},
	} {
		if got := LandGuard("Bash", map[string]any{"command": c.command}, c.agent, c.cwd, c.rules, git, check); (c.deny == "") != (got == "") || !strings.Contains(got, c.deny) {
			t.Errorf("%s: %q", c.name, got)
		}
	}
	if LandGuard("Write", map[string]any{"command": "git push origin main"}, "a1", "/r", by, git, check) != "" {
		t.Error("only Bash pushes")
	}
	got := LandGuard("Bash", map[string]any{"command": "git push origin main"}, "a8", "/repo", Land{Check: "false"}, git, check)
	if !strings.Contains(got, "land_check `false` failed") || !strings.Contains(got, "FAIL it") || ran[len(ran)-1] != "/repo: false" {
		t.Errorf("a failing check: %q, ran %v", got, ran)
	}
}

func TestLandGuardReadsQuotedRefsAndWrappedPushes(t *testing.T) {
	git := func(dir string, args ...string) string {
		if args[0] == "symbolic-ref" {
			return "origin/main\n"
		}
		return ""
	}
	n := 0
	check := func(string, string) (string, bool) { n++; return "", true }
	by := Land{By: "a8", Check: "c"}
	for command, deny := range map[string]bool{
		`git push origin 'main'`: true, `FOO=1 git push origin main`: true, `env git push origin main`: true,
		`git push --dry-run origin main`: false, `git push origin :main`: false,
	} {
		if got := LandGuard("Bash", map[string]any{"command": command}, "a1", "/r", by, git, check); (got != "") != deny {
			t.Errorf("%s: %q", command, got)
		}
	}
	LandGuard("Bash", map[string]any{"command": "git push origin main && git push origin main"}, "a8", "/r", by, git, check)
	if n != 1 {
		t.Errorf("the check ran %d times for one command", n)
	}
}

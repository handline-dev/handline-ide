package team

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/handline/handline/go/kit/paths"
)

func TestEverySandboxedSessionReachesThePackageRegistriesWithNoSetting(t *testing.T) {
	got := SandboxDomains(Config{})
	for _, h := range []string{"registry.npmjs.org", "pypi.org", "files.pythonhosted.org", "crates.io", "proxy.golang.org",
		"sum.golang.org", "repo.maven.apache.org", "rubygems.org"} {
		if !slices.Contains(got, h) {
			t.Errorf("%s is not open with no setting", h)
		}
	}
	got = SandboxDomains(Config{"sandbox_allow": "2026-11-26 host expo.dev\n2026-11-26 host pypi.org"})
	if got[len(got)-1] != "expo.dev" || len(got) != len(SandboxRegistries)+1 {
		t.Fatalf("an allowed host comes after the registries, once: %v", got[len(SandboxRegistries):])
	}
}

func TestAnAgentHasOnlyTheSandboxAllowancesItsListNames(t *testing.T) {
	if got := SandboxAllowancesOf(Config{}); got.Cert || got.Hosts != nil || got.Commands != nil || got.Writes != nil {
		t.Fatalf("an agent has none until the founder gives them: %+v", got)
	}
	cfg := Config{"sandbox_allow": "2026-11-26 host pypi.org\n\n2026-11-26 command pnpm install\nweaker-network\n" +
		"2026-11-26 write ~/.cargo\n2026-11-27 host pypi.org\n2026-11-26 cert\n2026-11-26 write /etc/../x"}
	got := SandboxAllowancesOf(cfg)
	if !got.Cert || !slices.Equal(got.Hosts, []string{"pypi.org"}) || !slices.Equal(got.Commands, []string{"pnpm install"}) ||
		!slices.Equal(got.Writes, []string{filepath.Join(paths.UserHome(), ".cargo")}) {
		t.Fatalf("each entry once, a line that does not read grants nothing: %+v", got)
	}
	if want := "2026-11-26 host pypi.org\n2026-11-26 command pnpm install\n2026-11-26 write ~/.cargo\n2026-11-26 cert"; SandboxAllowText(SandboxEntries(cfg)) != want {
		t.Fatalf("the list as kept: %q", SandboxAllowText(SandboxEntries(cfg)))
	}
}

func TestASandboxEntryIsCheckedByItsKind(t *testing.T) {
	for _, c := range []struct {
		kind, value string
		ok          bool
	}{
		{"host", "pypi.org", true}, {"host", "*.example.com", true}, {"host", "PyPI.org", false}, {"host", "localhost", false},
		{"host", "*", false}, {"host", "a.b/c", false}, {"host", "", false},
		{"command", "pnpm install", true}, {"command", "go test *", true}, {"command", "*", false}, {"command", "a\nb", false},
		{"command", " pnpm", false}, {"command", "", false},
		{"write", "~/.cargo", true}, {"write", "/opt/cache", true}, {"write", "/", false}, {"write", "~/", false}, {"write", "~", false},
		{"write", "~/.", false}, {"write", "/a/../b", false}, {"write", "relative", false}, {"write", "/a/", false},
		{"write", "//", false}, {"write", "~//", false}, {"write", "~//x", false}, {"write", paths.UserHome(), false},
		{"write", filepath.Dir(paths.UserHome()), false}, {"write", "~/a\u202eb", false}, {"host", "127.0.0.1", false},
		{"host", "169.254.169.254", false}, {"command", "pnpm\u00a0install", false},
		{"cert", "", true}, {"cert", "x", false}, {"root", "", false},
	} {
		if why := SandboxEntryProblem(c.kind, c.value); (why == "") != c.ok {
			t.Errorf("%s %q: ok %v, said %q", c.kind, c.value, c.ok, why)
		}
	}
	for _, text := range []string{"weaker-network", "2026-13-01 cert", "host pypi.org", "2026-11-26 host"} {
		if SandboxAllowProblem(text) == "" {
			t.Errorf("%q passed", text)
		}
	}
	if SandboxAllowProblem("") != "" || SandboxAllowProblem("2026-11-26 cert\n\n2026-11-26 command pnpm install --frozen-lockfile") != "" {
		t.Error("a good list was refused")
	}
}

func TestAnAskAddsOneEntryAndKeepsTheRest(t *testing.T) {
	day := time.Date(2026, 11, 26, 12, 0, 0, 0, time.Local)
	text, added := WithSandboxEntry("2026-11-01 cert\n2026-11-02 host pypi.org", SandboxCommand, "pnpm install", day)
	if !added || text != "2026-11-01 cert\n2026-11-02 host pypi.org\n2026-11-26 command pnpm install" {
		t.Fatalf("%v %q", added, text)
	}
	if again, added := WithSandboxEntry(text, SandboxHost, "pypi.org", day); added || again != text {
		t.Fatalf("an entry already there is no change: %v %q", added, again)
	}
	if got := (SandboxEntry{Kind: SandboxCommand, Value: "pnpm install"}).Phrase(); got != "run pnpm install outside the sandbox" {
		t.Fatal(got)
	}
	if got := (SandboxEntry{Kind: SandboxHost, Value: "*.example.com"}).Phrase(); got != "reach any host under example.com" {
		t.Fatal(got)
	}
}

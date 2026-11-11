package team

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestTheFoundersSwitchTurnsTheSandboxOnWhereTheAgentSaysNothing(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	r := NewRoster(newHome(t))
	if err := r.Store.Put("switches", "sandbox_default", "on"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		cfg  Config
		want bool
	}{{Config{}, true}, {Config{"sandbox": false}, false}, {Config{"sandbox": true}, true}, {Config{"sandbox": "no"}, true}} {
		if got := r.SandboxOn("worker", c.cfg); got != c.want {
			t.Fatalf("%v: on %v, want %v", c.cfg, got, c.want)
		}
	}
	if err := r.Store.Put("switches", "sandbox_default", "off"); err != nil {
		t.Fatal(err)
	}
	if r.SandboxOn("worker", Config{}) {
		t.Fatal("the switch off leaves the old default")
	}
	got := SandboxDomains(Config{"sandbox_domains": " expo.dev,, "})
	if strings.Join(got, ",") != "registry.npmjs.org,proxy.golang.org,sum.golang.org,expo.dev" {
		t.Fatalf("the registries and the hosts its row adds: %v", got)
	}
}

func TestARepoHasOnlyTheSandboxAllowancesTheFounderGaveIt(t *testing.T) {
	t.Setenv("HANDLINE_AGENT", "")
	r := NewRoster(newHome(t))
	ours, theirs := Config{"repo": "/ws/handline/"}, Config{"repo": "/ws/harbor-app"}
	if got := r.RepoAllowancesOf(ours); got.WeakerNetwork || got.Excluded != nil {
		t.Fatalf("a repo has none until the founder gives them: %+v", got)
	}
	if err := r.Store.Put(RepoAllowanceNS, "handline", "weaker-network\npnpm-install\nrm-rf"); err != nil {
		t.Fatal(err)
	}
	if got := r.RepoAllowancesOf(ours); !got.WeakerNetwork || !slices.Equal(got.Excluded, []string{"pnpm install", "pnpm install --frozen-lockfile"}) {
		t.Fatalf("the repo's entry, and a line that names no allowance grants nothing: %+v", got)
	}
	// The CLI takes a pattern as the whole command, or as its start when it ends in " *": an add of a package stays in the sandbox.
	for _, pattern := range r.RepoAllowancesOf(ours).Excluded {
		start, prefix := strings.CutSuffix(pattern, " *")
		if cmd := "pnpm install left-pad"; cmd == pattern || prefix && strings.HasPrefix(cmd, start+" ") {
			t.Fatalf("%q runs %q outside the sandbox", pattern, cmd)
		}
	}
	for _, cfg := range []Config{theirs, {}, {"repo": "/ws/@personal"}} {
		if got := r.RepoAllowancesOf(cfg); got.WeakerNetwork || got.Excluded != nil {
			t.Fatalf("%v: another repo has none: %+v", cfg, got)
		}
	}
}

func TestConfigReaders(t *testing.T) {
	if ContextTokensOf(Config{}) != ContextTokens || ContextTokensOf(Config{"context_tokens": int64(50000)}) != 50000 ||
		ContextTokensOf(Config{"context_tokens": true}) != ContextTokens || ContextTokensOf(Config{"context_tokens": int64(-1)}) != ContextTokens ||
		ContextTokensOf(Config{"context_tokens": "9"}) != ContextTokens {
		t.Error("context_tokens")
	}
	if ContextTokens != 300_000 {
		t.Errorf("the default context-handoff limit is 300,000 tokens, not %d", ContextTokens)
	}
	if ExtraWorktrees(Config{"extra_worktrees": int64(9)}) != 2 || ExtraWorktrees(Config{"extra_worktrees": true}) != 0 ||
		ExtraWorktrees(Config{"extra_worktrees": int64(1)}) != 1 {
		t.Error("extra_worktrees")
	}
	if v, m, e := Parts(Config{"model": int64(4), "effort": "", "vendor": nil}); v != "claude" || m != "4" || e != "medium" {
		t.Errorf("%s %s %s", v, m, e)
	}
	if Kind(Config{"vendor": "opencode", "effort": "xhigh"}) != "opencode sonnet xhi" || Kind(Config{"effort": "odd"}) != "sonnet odd" {
		t.Error("kind")
	}
	if got := MCPConfigArgs(Config{"mcp_config": `{"mcpServers": {}}`}); len(got) != 2 || got[1] != `{"mcpServers":{}}` {
		t.Errorf("%v", got)
	}
	if MCPConfigArgs(Config{"mcp_config": "garbage"}) != nil || MCPConfigArgs(Config{}) != nil {
		t.Error("a value that is not a config adds nothing")
	}
}

func TestMCPConfigTextIsOneLineOfASCIIJSONInOrder(t *testing.T) {
	got, err := MCPConfigText(`{"mcpServers": {"a": {"command": "x", "args": ["é", 1.5, true, null]}}, "other": 1}`)
	want := `{"mcpServers":{"a":{"command":"x","args":["\u00e9",1.5,true,null]}},"other":1}`
	if err != nil || got != want {
		t.Fatalf("%q\n%q %v", got, want, err)
	}
	for _, bad := range []string{`{"nope":1}`, `[1]`, `{"mcpServers": 1}`, `{`} {
		if _, err := MCPConfigText(bad); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
}

func TestBrowserConfigRoundTripsLikeThePython(t *testing.T) {
	home := "/h"
	cfg := Config{"mcp_config": `{"mcpServers":{"keep":{"command":"k"},"browser":{"command":"sh","args":["-c","x --profile-dir-name \"P9\" --allowed-origins \"https://a.com;https://b.org:8080\""]}}}`}
	if BrowserMode(cfg) != "playwright" || !reflect.DeepEqual(BrowserOrigins(cfg), []string{"https://a.com", "https://b.org:8080"}) {
		t.Fatalf("%s %v", BrowserMode(cfg), BrowserOrigins(cfg))
	}
	got, err := BrowserMCPConfig(home, cfg, "playwright", []string{"https://x.io"})
	want := fmt.Sprintf(`{"mcpServers":{"keep":{"command":"k"},"browser":{"command":"sh","args":["-c","PLAYWRIGHT_MCP_EXTENSION_TOKEN=$(cat %s/browser/token) exec %s/browser/node_modules/.bin/playwright-mcp --extension --profile-dir-name \"P9\" --allowed-origins \"https://x.io\""]}}}`, home, home)
	if err != nil || got != want {
		t.Fatalf("%q %v", got, err)
	}
	if off, _ := BrowserMCPConfig(home, cfg, "off", nil); off != `{"mcpServers":{"keep":{"command":"k"}}}` {
		t.Fatalf("%q", off)
	}
	if none, _ := BrowserMCPConfig(home, Config{}, "off", nil); none != "" {
		t.Fatalf("%q", none)
	}
	if _, err := BrowserMCPConfig(home, cfg, "custom", nil); err == nil {
		t.Fatal("a custom browser is not set here")
	}
	if BrowserMode(Config{"chrome": true}) != "chrome" || BrowserMode(Config{}) != "off" ||
		BrowserMode(Config{"mcp_config": `{"mcpServers":{"browser":{"args":["run"]}}}`}) != "custom" {
		t.Fatal("browser modes")
	}
}

func TestParseOrigins(t *testing.T) {
	got, err := ParseOrigins("https://a.com, https://b.com/ ; https://a.com")
	if err != nil || !reflect.DeepEqual(got, []string{"https://a.com", "https://b.com"}) {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range []string{"ftp://a.com", "a.com", "https://a.com/path", "https://a.com;rm -rf"} {
		if _, err := ParseOrigins(bad); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
	if _, err := CheckOrigin("https://A.com", []string{"https://a.com"}); err == nil {
		t.Fatal("a duplicate was taken")
	}
}

package team

import (
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

func TestAnAgentHasOnlyTheSandboxAllowancesItsRowNames(t *testing.T) {
	if got := SandboxAllowancesOf(Config{}); got.WeakerNetwork || got.Excluded != nil {
		t.Fatalf("an agent has none until the founder gives them: %+v", got)
	}
	cfg := Config{"sandbox_allow": "weaker-network pnpm-install rm-rf pnpm-install"}
	if got := SandboxAllowWords(cfg); !slices.Equal(got, []string{"weaker-network", "pnpm-install"}) {
		t.Fatalf("each known word once, in order: %v", got)
	}
	got := SandboxAllowancesOf(cfg)
	if !got.WeakerNetwork || !slices.Equal(got.Excluded, []string{"pnpm install", "pnpm install --frozen-lockfile"}) {
		t.Fatalf("a word that names no allowance grants nothing: %+v", got)
	}
	// The CLI takes a pattern as the whole command, or as its start when it ends in " *": an add of a package stays in the sandbox.
	for _, pattern := range got.Excluded {
		start, prefix := strings.CutSuffix(pattern, " *")
		if cmd := "pnpm install left-pad"; cmd == pattern || prefix && strings.HasPrefix(cmd, start+" ") {
			t.Fatalf("%q runs %q outside the sandbox", pattern, cmd)
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
	if v, m, e := Parts(Config{"model": int64(4), "effort": "", "vendor": nil}); v != "claude" || m != "4" || e != "medium" {
		t.Errorf("%s %s %s", v, m, e)
	}
	if Kind(Config{"vendor": "opencode", "effort": "xhigh"}) != "opencode xhi" || Kind(Config{"effort": "odd"}) != "sonnet odd" {
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

func TestBrowserConfig(t *testing.T) {
	cfg := Config{"mcp_config": `{"mcpServers":{"keep":{"command":"k"},"browser":{"command":"sh","args":["-c","x"]}}}`}
	if BrowserMode(cfg) != "custom" {
		t.Fatal(BrowserMode(cfg))
	}
	for _, mode := range []string{"off", "chrome"} {
		if got, _ := BrowserMCPConfig(cfg, mode); got != `{"mcpServers":{"keep":{"command":"k"}}}` {
			t.Fatalf("%s: %q", mode, got)
		}
	}
	if none, _ := BrowserMCPConfig(Config{}, "off"); none != "" {
		t.Fatalf("%q", none)
	}
	if _, err := BrowserMCPConfig(cfg, "custom"); err == nil {
		t.Fatal("a custom browser is not set here")
	}
	if BrowserMode(Config{"chrome": true}) != "chrome" || BrowserMode(Config{}) != "off" {
		t.Fatal("browser modes")
	}
}

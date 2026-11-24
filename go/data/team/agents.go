// Package team is handline/team: the agents' and accounts' files, the routing policy, standing consent for folders and an
// agent's settings. It reads and writes the same files the Python modules do, and refuses what they refuse.
package team

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/yamlcompat"
)

const (
	DefaultModel = "sonnet"
	// DefaultEffort is a cost lever as real as the model: each agent declares its own.
	DefaultEffort = "medium"
	// ContextTokens is the default context-handoff threshold: an absolute figure, so it means the same on every model.
	ContextTokens = 300_000

	BrowserServer = "browser"
	DefaultMode   = "auto"
)

var (
	ValidEffort = []string{"low", "medium", "high", "xhigh", "max"}
	// EffortShort are the effort names short enough for a column; an unknown value is shown whole.
	EffortShort = map[string]string{"low": "low", "medium": "mid", "high": "high", "xhigh": "xhi", "max": "max"}
	// Modes are Claude's permission modes.
	Modes = []string{"default", "auto", "acceptEdits", "plan"}
)

// Config is an agent's settings, by agents column (repo its workspace's folder).
type Config = map[string]any

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int64:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}

// pyStr is str() of a decoded value.
func pyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case string:
		return x
	case float64:
		return yamlcompat.FloatRepr(x)
	}
	return fmt.Sprint(v)
}

// orDefault is str(cfg.get(k) or def).
func orDefault(cfg Config, k, def string) string {
	if v := cfg[k]; truthy(v) {
		return pyStr(v)
	}
	return def
}

// Parts is (vendor, model, effort) as configured, defaults filled in.
func Parts(cfg Config) (vendor, model, effort string) {
	vendor = orDefault(cfg, "vendor", DefaultVendor)
	return vendor, orDefault(cfg, "model", builtinModel(vendor)), orDefault(cfg, "effort", DefaultEffort)
}

func positiveInt(v any) (int64, bool) {
	n, ok := v.(int64)
	return n, ok && n > 0
}

// ContextTokensOf is the agent's own context-handoff threshold: configured, or the default for an absent, falsy or garbage value.
func ContextTokensOf(cfg Config) int64 {
	if n, ok := positiveInt(cfg["context_tokens"]); ok {
		return n
	}
	return ContextTokens
}

// Chrome is whether this agent's sessions start with the browser integration. Only a real true turns it on.
func Chrome(cfg Config) bool { return cfg["chrome"] == true }

// MCPConfigText is text as one line of JSON, if it is a {"mcpServers": {...}} object (what --mcp-config takes).
func MCPConfigText(text string) (string, error) {
	v, err := decodeOrdered(text)
	if err != nil {
		return "", err
	}
	o, ok := v.(*jobj)
	if !ok {
		return "", errors.New(`not a {"mcpServers": {...}} object`)
	}
	if _, ok := o.vals["mcpServers"].(*jobj); !ok {
		return "", errors.New(`not a {"mcpServers": {...}} object`)
	}
	return encodeCompact(o), nil
}

// MCPConfigArgs is this agent's own MCP servers as --mcp-config <json>. They come from its row, which the founder sets, not from a
// project .mcp.json. A value that is not a valid config adds nothing.
func MCPConfigArgs(cfg Config) []string {
	v, ok := cfg["mcp_config"].(string)
	if !ok {
		return nil
	}
	text, err := MCPConfigText(v)
	if err != nil {
		return nil
	}
	return []string{"--mcp-config", text}
}

// browserCommand is the shell line of the agent's own browser MCP server, or "".
func browserCommand(cfg Config) string {
	text := "{}"
	if truthy(cfg["mcp_config"]) {
		text = pyStr(cfg["mcp_config"])
	}
	v, err := decodeOrdered(text)
	if err != nil {
		return ""
	}
	o, ok := v.(*jobj)
	if !ok {
		return ""
	}
	servers, ok := o.vals["mcpServers"].(*jobj)
	if !ok {
		return ""
	}
	srv, ok := servers.vals[BrowserServer].(*jobj)
	if !ok {
		return ""
	}
	args, ok := srv.vals["args"].([]any)
	if !ok || len(args) == 0 {
		return ""
	}
	return jsonText(args[len(args)-1])
}

// jsonText is str() of a decoded JSON value, for the cases a command line can be.
func jsonText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "None"
	case bool:
		return pyStr(x)
	}
	return fmt.Sprint(v)
}

// BrowserMode is how this agent browses: "chrome" (Claude in Chrome), "custom" (a browser server set by hand, left alone) or "off".
func BrowserMode(cfg Config) string {
	if Chrome(cfg) {
		return "chrome"
	}
	if browserCommand(cfg) == "" {
		return "off"
	}
	return "custom"
}

// BrowserMCPConfig is the mcp_config text for mode: the agent's other MCP servers as they are, its browser server removed; "" when
// nothing is left.
func BrowserMCPConfig(cfg Config, mode string) (string, error) {
	if mode == "custom" {
		return "", errors.New("a custom browser config is set in a terminal, not here")
	}
	servers := newJobj()
	text := "{}"
	if truthy(cfg["mcp_config"]) {
		text = pyStr(cfg["mcp_config"])
	}
	if v, err := decodeOrdered(text); err == nil {
		if o, ok := v.(*jobj); ok {
			if s, ok := o.vals["mcpServers"].(*jobj); ok {
				for _, k := range s.keys {
					servers.set(k, s.vals[k])
				}
			}
		}
	}
	servers.del(BrowserServer) // any mode picked replaces a server set by hand
	if len(servers.keys) == 0 {
		return "", nil
	}
	top := newJobj()
	top.set("mcpServers", servers)
	return encodeCompact(top), nil
}

func pyQuote(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}

// Kind is what an agent is, for a column: the model and its effort, the vendor drawn only when it is not the default's.
func Kind(cfg Config) string {
	vendor, model, effort := Parts(cfg)
	short, ok := EffortShort[effort]
	if !ok {
		short = effort
	}
	var parts []string
	if vendor != DefaultVendor {
		parts = append(parts, vendor)
	}
	if model != "" { // "" is the vendor's own default
		parts = append(parts, model)
	}
	return strings.Join(append(parts, short), " ")
}

// ModeOf is the permission mode a launch will use: its row's mode when it names one, else auto. Anything else reads as the
// default, never as more than was asked.
func (r *Roster) ModeOf(agent string, cfg Config) string {
	if v, ok := cfg["mode"].(string); ok && contains(Modes, v) {
		return v
	}
	return DefaultMode
}

func contains(xs []string, x string) bool {
	for _, e := range xs {
		if e == x {
			return true
		}
	}
	return false
}

// SandboxOn is whether the sandbox is on: what its row's sandbox says when it is set, else the founder's
// default, which is on once the protected switch sandbox_default reads "on" (an agent cannot write the store, which holds its
// instructions) and off before.
func (r *Roster) SandboxOn(agent string, cfg Config) bool {
	if v, ok := cfg["sandbox"].(bool); ok {
		return v
	}
	if r.Store == nil {
		return false
	}
	v, _ := r.Store.Get("switches", "sandbox_default")
	return strings.TrimSpace(v) == "on"
}

// SandboxDomains are the hosts a sandboxed session may reach: the package registries every repo builds from, and those its row
// adds under sandbox_domains, comma separated.
func SandboxDomains(cfg Config) []string {
	out := []string{"registry.npmjs.org", "proxy.golang.org", "sum.golang.org"}
	extra, _ := cfg["sandbox_domains"].(string)
	for _, d := range strings.Split(extra, ",") {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// SandboxAllowances are what a sandboxed session may do beyond the sandbox, because the founder allowed it for the agent
// (sandbox_allow).
type SandboxAllowances struct {
	// WeakerNetwork lets the session reach trustd, which Go on macOS verifies TLS through: without it no Go module downloads.
	WeakerNetwork bool
	// Excluded are the commands that run outside the sandbox.
	Excluded []string
}

// SandboxAllowanceNames are the allowances sandbox_allow may name, each as it changes the session's sandbox.
var SandboxAllowanceNames = map[string]func(*SandboxAllowances){
	"weaker-network": func(a *SandboxAllowances) { a.WeakerNetwork = true },
	"pnpm-install": func(a *SandboxAllowances) {
		a.Excluded = append(a.Excluded, "pnpm install", "pnpm install --frozen-lockfile")
	},
}

// SandboxAllowWords is sandbox_allow as written: its allowance words, each once, in the order given; a word that names no
// allowance is kept out.
func SandboxAllowWords(cfg Config) []string {
	v, _ := cfg["sandbox_allow"].(string)
	var out []string
	for _, w := range strings.Fields(v) {
		if SandboxAllowanceNames[w] != nil && !contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

// SandboxAllowancesOf are the allowances the agent's row gives its sandboxed sessions; none until the founder sets sandbox_allow.
func SandboxAllowancesOf(cfg Config) SandboxAllowances {
	var out SandboxAllowances
	for _, w := range SandboxAllowWords(cfg) {
		SandboxAllowanceNames[w](&out)
	}
	return out
}

func pairsGet(p []yamlcompat.Pair, k string) (any, bool) {
	for _, x := range p {
		if x.Key == k {
			return x.Val, true
		}
	}
	return nil, false
}

func pairsSet(p []yamlcompat.Pair, k string, v any) []yamlcompat.Pair {
	for i := range p {
		if p[i].Key == k {
			p[i].Val = v
			return p
		}
	}
	return append(p, yamlcompat.Pair{Key: k, Val: v})
}

func expandUser(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(paths.UserHome(), strings.TrimPrefix(p, "~"))
	}
	return p
}

// CheckoutProblem is why repo (its workspace's folder, ~ allowed) is no place to make a worktree: "is not an absolute path", "is not a
// folder" or "is not a git checkout"; "" when it is one.
func CheckoutProblem(repo string) string {
	path := expandUser(repo)
	if !filepath.IsAbs(path) {
		return "is not an absolute path"
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return "is not a folder"
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return "is not a git checkout"
	}
	return ""
}

// Querier is what a read needs: a database or a transaction.
type Querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// CurrentAccount is the account this agent's session is on; "" when none is recorded. A session id belongs to one account's
// config directory, so a resume has no other answer to give than this.
func CurrentAccount(db Querier, name string) (string, error) {
	var acct sql.NullString
	err := db.QueryRow("SELECT account FROM agent_state WHERE agent = ?", name).Scan(&acct)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return acct.String, err
}

// AccountReading is which account an agent's usage is landing on and how sure of it we are: recorded (a launch, or the process
// check wrote it), reported (the agent's own fresh snapshot named it) or assumed (a guess before a launch confirmed it).
type AccountReading struct{ Account, Source string }

// ReadAccount is the reading, for reading and never for choosing where a launch goes. reported is the agent's own status
// snapshot's account when it is fresh and open ("" when none): the usage module's, passed in. ok is false when nothing says.
func ReadAccount(db *sql.DB, name string, reported func() string) (AccountReading, bool, error) {
	var acct, source sql.NullString
	err := db.QueryRow("SELECT account, account_source FROM agent_state WHERE agent = ?", name).Scan(&acct, &source)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AccountReading{}, false, err
	}
	has := err == nil && acct.String != ""
	if has && source.String != "assumed" {
		return AccountReading{acct.String, "recorded"}, true, nil
	}
	if reported != nil {
		if a := reported(); a != "" {
			return AccountReading{a, "reported"}, true, nil
		}
	}
	if has {
		return AccountReading{acct.String, "assumed"}, true, nil
	}
	return AccountReading{}, false, nil
}

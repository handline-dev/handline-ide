package team

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/handline/handline/go/kit/paths"
)

// SandboxRegistries are the package registries every sandboxed session reaches with no setting: the package-manager groups of the
// default allowed domains Claude Code on the web publishes (code.claude.com/docs/en/cloud-environments, "Default allowed domains").
var SandboxRegistries = []string{
	// JavaScript and Node
	"registry.npmjs.org", "www.npmjs.com", "www.npmjs.org", "npmjs.com", "npmjs.org", "yarnpkg.com", "registry.yarnpkg.com",
	"jsr.io", "npm.jsr.io",
	// Python
	"pypi.org", "www.pypi.org", "files.pythonhosted.org", "pythonhosted.org", "test.pypi.org", "pypi.python.org", "pypa.io",
	"www.pypa.io",
	// Ruby
	"rubygems.org", "www.rubygems.org", "api.rubygems.org", "index.rubygems.org", "ruby-lang.org", "www.ruby-lang.org",
	"rubyforge.org", "www.rubyforge.org", "rubyonrails.org", "www.rubyonrails.org", "rvm.io", "get.rvm.io",
	// Rust
	"crates.io", "www.crates.io", "index.crates.io", "static.crates.io", "rustup.rs", "static.rust-lang.org", "www.rust-lang.org",
	// Go
	"proxy.golang.org", "sum.golang.org", "index.golang.org", "golang.org", "www.golang.org", "goproxy.io", "pkg.go.dev",
	// JVM
	"maven.org", "repo.maven.org", "central.maven.org", "repo1.maven.org", "repo.maven.apache.org", "maven.google.com",
	"jcenter.bintray.com", "gradle.org", "www.gradle.org", "services.gradle.org", "plugins.gradle.org",
	"plugins-artifacts.gradle.org", "kotlinlang.org", "www.kotlinlang.org", "spring.io", "repo.spring.io",
	// Other package managers
	"packagist.org", "www.packagist.org", "repo.packagist.org", "nuget.org", "www.nuget.org", "api.nuget.org", "pub.dev",
	"api.pub.dev", "hex.pm", "www.hex.pm", "cpan.org", "www.cpan.org", "metacpan.org", "www.metacpan.org", "api.metacpan.org",
	"cocoapods.org", "www.cocoapods.org", "cdn.cocoapods.org", "haskell.org", "www.haskell.org", "hackage.haskell.org",
	"swift.org", "www.swift.org",
}

// The kinds of a sandbox_allow entry, each one of Claude Code's own sandbox settings.
const (
	SandboxHost    = "host"    // a host it may reach: sandbox.network.allowedDomains
	SandboxCommand = "command" // a command that runs outside the sandbox: sandbox.excludedCommands
	SandboxWrite   = "write"   // a folder it may write: sandbox.filesystem.allowWrite
	SandboxCert    = "cert"    // the macOS certificate check Go-built tools need: sandbox.enableWeakerNetworkIsolation
)

// SandboxKinds are the entry kinds, in the order they are told.
var SandboxKinds = []string{SandboxHost, SandboxCommand, SandboxWrite, SandboxCert}

// SandboxEntry is one line of sandbox_allow: `<day> <kind> <value>`, the day the founder allowed it; a cert entry has no value.
type SandboxEntry struct{ Day, Kind, Value string }

var (
	sandboxDay  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	// The last label starts with a letter, so an IP address is no host.
	sandboxHost = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
	// printable is a command or folder as the founder reads it: printable ASCII, so no character shows other than it applies.
	printable = regexp.MustCompile(`^[\x20-\x7e]+$`)
)

// Line is the entry as sandbox_allow keeps it.
func (e SandboxEntry) Line() string { return strings.TrimSpace(e.Day + " " + e.Kind + " " + e.Value) }

// Phrase is what the entry lets the agent do, in plain words: "reach pypi.org".
func (e SandboxEntry) Phrase() string {
	switch e.Kind {
	case SandboxHost:
		if under, ok := strings.CutPrefix(e.Value, "*."); ok {
			return "reach any host under " + under
		}
		return "reach " + e.Value
	case SandboxCommand:
		return "run " + e.Value + " outside the sandbox"
	case SandboxWrite:
		return "write " + e.Value
	case SandboxCert:
		return "pass the macOS certificate check Go-built tools need (go, gh, terraform)"
	}
	return e.Line()
}

// SandboxEntryProblem is why kind and value are not an entry, or "".
func SandboxEntryProblem(kind, value string) string {
	switch kind {
	case SandboxHost:
		if len(value) > 253 || !sandboxHost.MatchString(value) {
			return fmt.Sprintf("%s is not a host: a lowercase name such as pypi.org, or *.example.com for its subdomains", quote(value))
		}
	case SandboxCommand:
		if len(value) > 300 || !printable.MatchString(value) || strings.TrimSpace(value) != value || strings.HasPrefix(value, "*") {
			return fmt.Sprintf("%s is not a command: one line of at most 300 ASCII characters, such as pnpm install, that does not start with *", quote(value))
		}
	case SandboxWrite:
		if why := sandboxWriteProblem(value); why != "" {
			return fmt.Sprintf("%s is not a folder: %s", quote(value), why)
		}
	case SandboxCert:
		if value != "" {
			return "a cert entry takes no value"
		}
	default:
		return fmt.Sprintf("%s is not a kind: one of %s", quote(kind), strings.Join(SandboxKinds, ", "))
	}
	return ""
}

// sandboxWriteProblem is why value is not a folder an agent may be given to write, or "": a clean path from / or ~/ below the home,
// or outside it, but never the disk, the home or a folder that holds the home.
func sandboxWriteProblem(value string) string {
	rest, home := strings.CutPrefix(value, "~/")
	if !home {
		var abs bool
		if rest, abs = strings.CutPrefix(value, "/"); !abs {
			return "start it with / or ~/"
		}
	}
	if len(value) > 400 || !printable.MatchString(value) || rest == "" || path.Clean(rest) != rest || rest == "." ||
		rest == ".." || strings.HasPrefix(rest, "../") || strings.HasPrefix(rest, "/") {
		return "a clean path of ASCII characters such as ~/.cargo"
	}
	if abs := path.Clean(expand(value)); strings.HasPrefix(path.Clean(paths.UserHome())+"/", abs+"/") {
		return "not the disk, the home or a folder that holds it"
	}
	return ""
}

// ParseSandboxEntry reads one line of sandbox_allow.
func ParseSandboxEntry(line string) (SandboxEntry, string) {
	day, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
	kind, value, _ := strings.Cut(rest, " ")
	e := SandboxEntry{day, kind, strings.TrimSpace(value)}
	if _, err := time.Parse(time.DateOnly, day); err != nil || !sandboxDay.MatchString(day) {
		return e, fmt.Sprintf("%s does not start with the day it was allowed (YYYY-MM-DD kind value)", quote(line))
	}
	return e, SandboxEntryProblem(kind, e.Value)
}

// SandboxAllowProblem is why text is not a sandbox_allow list, or "": one entry per line, `<day> <kind> <value>`.
func SandboxAllowProblem(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if _, why := ParseSandboxEntry(l); why != "" {
			return "sandbox_allow is one entry per line, <day> <kind> <value>: " + why
		}
	}
	return ""
}

// sandboxEntriesOf are text's entries in order, each kind and value once (the first kept); a line that does not read is left out.
func sandboxEntriesOf(text string) []SandboxEntry {
	var out []SandboxEntry
	seen := map[[2]string]bool{}
	for _, l := range strings.Split(text, "\n") {
		e, why := ParseSandboxEntry(l)
		if strings.TrimSpace(l) == "" || why != "" || seen[[2]string{e.Kind, e.Value}] {
			continue
		}
		seen[[2]string{e.Kind, e.Value}] = true
		out = append(out, e)
	}
	return out
}

// SandboxEntries are the entries of the agent's sandbox_allow; none until the founder allows one.
func SandboxEntries(cfg Config) []SandboxEntry {
	v, _ := cfg["sandbox_allow"].(string)
	return sandboxEntriesOf(v)
}

// SandboxAllowText is the list as sandbox_allow keeps it, one entry per line.
func SandboxAllowText(entries []SandboxEntry) string {
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.Line()
	}
	return strings.Join(lines, "\n")
}

// NormalSandboxAllow is text with each entry once and blank lines gone, so the same list is no change.
func NormalSandboxAllow(text string) string { return SandboxAllowText(sandboxEntriesOf(text)) }

// WithSandboxEntry is the list with e added on day, and whether it was not there yet: an ask adds and never drops the others.
func WithSandboxEntry(text string, kind, value string, day time.Time) (string, bool) {
	entries := sandboxEntriesOf(text)
	for _, x := range entries {
		if x.Kind == kind && x.Value == value {
			return SandboxAllowText(entries), false
		}
	}
	return SandboxAllowText(append(entries, SandboxEntry{day.Format(time.DateOnly), kind, value})), true
}

// SandboxAllowances are what a sandboxed session may do beyond the sandbox, because the founder allowed it for the agent.
type SandboxAllowances struct {
	Hosts, Commands, Writes []string
	// Cert lets the session reach trustd, which Go-built tools on macOS verify TLS through.
	Cert bool
}

// SandboxAllowancesOf are the allowances the agent's row gives its sandboxed sessions, a write folder under ~ made absolute.
func SandboxAllowancesOf(cfg Config) SandboxAllowances {
	var out SandboxAllowances
	for _, e := range SandboxEntries(cfg) {
		switch e.Kind {
		case SandboxHost:
			out.Hosts = append(out.Hosts, e.Value)
		case SandboxCommand:
			out.Commands = append(out.Commands, e.Value)
		case SandboxWrite:
			out.Writes = append(out.Writes, expand(e.Value))
		case SandboxCert:
			out.Cert = true
		}
	}
	return out
}

// SandboxDomains are the hosts a sandboxed session may reach: the package registries, then those the founder allowed the agent.
func SandboxDomains(cfg Config) []string {
	out := append([]string{}, SandboxRegistries...)
	for _, h := range SandboxAllowancesOf(cfg).Hosts {
		if !contains(out, h) {
			out = append(out, h)
		}
	}
	return out
}

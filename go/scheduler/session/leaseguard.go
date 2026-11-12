package session

import (
	"path"
	"strconv"
	"strings"

	"github.com/handline/handline/go/scheduler/leases"
	"mvdan.cc/sh/v3/syntax"
)

// transparent are the words that run the command after them as it is.
var transparent = map[string]bool{"exec": true, "time": true, "nice": true, "env": true, "command": true, "sudo": true, "caffeinate": true,
	"timeout": true}

var shells = map[string]bool{"bash": true, "sh": true, "zsh": true}

// leaseKind is the lease a command starts ("heavy", "simulator" or "browser"), "" when it starts none. words is one command, its
// leading assignments and transparent words already dropped.
func leaseKind(words []string) string {
	name := path.Base(words[0])
	rest := words[1:]
	switch {
	case name == "xcrun":
		if len(rest) >= 2 && rest[0] == "simctl" && rest[1] == "boot" {
			return "simulator"
		}
	case name == "open":
		for i, r := range rest {
			if r == "-a" && i+1 < len(rest) && strings.HasPrefix(path.Base(strings.TrimSuffix(rest[i+1], "/")), "Simulator") {
				return "simulator"
			}
		}
	case name == "xcodebuild":
		if has(rest, "test") || has(rest, "test-without-building") {
			for i, r := range rest {
				if r == "-destination" && i+1 < len(rest) && strings.Contains(rest[i+1], "Simulator") {
					return "simulator"
				}
			}
		}
	case name == "emulator":
		for _, r := range rest {
			if r == "-avd" || strings.HasPrefix(r, "@") {
				return "simulator"
			}
		}
	case name == "playwright":
		return playwright(rest)
	case runners[name]:
		if i := firstArg(name, rest); i >= 0 && path.Base(rest[i]) == "playwright" {
			return playwright(rest[i+1:])
		}
	case strings.Contains(strings.ToLower(name), "chrom"):
		for _, r := range rest {
			if strings.HasPrefix(r, "--headless") {
				return "browser"
			}
		}
	}
	if heavy(name, rest) {
		return "heavy"
	}
	return ""
}

func has(words []string, w string) bool {
	for _, x := range words {
		if x == w {
			return true
		}
	}
	return false
}

// runners are the package runners whose first argument is a script or a program.
var runners = map[string]bool{"npx": true, "pnpx": true, "bunx": true, "pnpm": true, "npm": true, "yarn": true, "bun": true}

// heavyVerbs are the subcommands that make a tool's run a test or build run, by the tool's name.
var heavyVerbs = map[string]map[string]bool{
	"go":     {"test": true, "build": true},
	"cargo":  {"test": true, "build": true, "bench": true, "nextest": true},
	"swift":  {"test": true, "build": true},
	"bazel":  {"test": true, "build": true},
	"gradle": {"test": true, "build": true, "check": true},
	"mvn":    {"test": true, "package": true, "install": true, "verify": true},
	"xcodebuild": {"build": true, "test": true, "test-without-building": true, "build-for-testing": true, "archive": true,
		"analyze": true},
}

// valueFlags are the flags before a tool's verb that take the next word as their value.
var valueFlags = map[string]map[string]bool{
	"go":     {"-C": true},
	"gradle": {"-p": true, "-b": true, "-c": true, "--project-dir": true, "-x": true, "--exclude-task": true},
	"mvn":    {"-f": true, "-pl": true, "-rf": true},
	"cargo":  {"--color": true, "-Z": true, "--config": true, "-C": true},
	"xcodebuild": {"-scheme": true, "-project": true, "-workspace": true, "-target": true, "-configuration": true, "-sdk": true,
		"-destination": true, "-derivedDataPath": true, "-resultBundlePath": true, "-arch": true, "-xcconfig": true, "-jobs": true,
		"-testPlan": true, "-toolchain": true, "-clonedSourcePackagesDirPath": true, "-only-testing": true, "-skip-testing": true},
}

// xcodebuildOptions are the flags of an xcodebuild build that take no value; with no action, any other flag (-list,
// -resolvePackageDependencies) is not a build.
var xcodebuildOptions = map[string]bool{"-quiet": true, "-allowProvisioningUpdates": true, "-parallelizeTargets": true,
	"-enableCodeCoverage": true, "-skipPackagePluginValidation": true, "-skipMacroValidation": true}

// heavy is whether a command is a test or build run. A command that only asks a tool its version or help is not.
func heavy(name string, rest []string) bool {
	if name == "gradlew" || name == "mvnw" { // the wrappers
		name = strings.TrimSuffix(name, "w")
	}
	for _, r := range rest {
		if r == "--version" || r == "-version" || r == "--help" || r == "-help" || r == "-h" || (r == "-V" && name != "mvn") { // mvn -V builds
			return false
		}
	}
	switch {
	case name == "pytest" || name == "vitest" || name == "jest":
		return true
	case (name == "uv" || name == "poetry") && len(rest) > 1 && rest[0] == "run":
		return heavy(path.Base(rest[1]), rest[2:])
	case strings.HasPrefix(name, "python"):
		for i := 0; i+1 < len(rest); i++ {
			if rest[i] == "-m" {
				return rest[i+1] == "pytest"
			}
		}
		return false
	case runners[name]:
		i := firstArg(name, rest)
		if i < 0 {
			return false
		}
		script := path.Base(rest[i])
		return script == "test" || script == "build" || script == "vitest" || script == "jest" || strings.HasPrefix(script, "test:") ||
			strings.HasPrefix(script, "build:")
	}
	verbs := operands(rest, valueFlags[name])
	switch name {
	case "gradle", "mvn":
		for _, v := range verbs {
			v = v[strings.LastIndex(v, ":")+1:] // gradle's :app:test
			if heavyVerbs[name][v] || strings.HasPrefix(v, "assemble") {
				return true
			}
		}
		return false
	case "xcodebuild":
		for _, v := range verbs {
			if heavyVerbs[name][v] {
				return true
			}
		}
		if len(verbs) > 0 {
			return false
		}
		for _, r := range rest { // no action is build, unless a flag asks for something else
			if strings.HasPrefix(r, "-") && !valueFlags[name][r] && !xcodebuildOptions[r] {
				return false
			}
		}
		return true
	}
	return len(verbs) > 0 && heavyVerbs[name][verbs[0]]
}

// operands are the words that are not flags, KEY=VALUE settings, nor the values of the flags in takesValue.
func operands(words []string, takesValue map[string]bool) []string {
	var out []string
	for i := 0; i < len(words); i++ {
		switch w := words[i]; {
		case takesValue[w]:
			i++
		case strings.HasPrefix(w, "-") || strings.HasPrefix(w, "+") || strings.Contains(w, "="): // a flag, +toolchain, KEY=VALUE
		default:
			out = append(out, w)
		}
	}
	return out
}

// playwright is "browser" unless playwright's arguments only install, list or describe.
func playwright(args []string) string {
	if len(args) > 0 {
		switch args[0] {
		case "install", "install-deps", "uninstall", "--version", "-V", "--help", "-h", "help":
			return ""
		}
	}
	return "browser"
}

// firstArg is the index of the program or script a package runner's arguments run, -1 for none: past the flags (and the values of
// those that take one), any exec, dlx or run, and yarn's workspace and its name, so in `pnpm add playwright` it is add.
func firstArg(runner string, args []string) int {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case runner == "npm" && (a == "-w" || a == "--workspace"): // pnpm's -w takes no value
			i++
		case a == "--filter" || a == "-F" || a == "-C" || a == "--dir" || a == "-p" || a == "--package" || a == "--prefix" || a == "--cwd" ||
			a == "workspace":
			i++
		case strings.HasPrefix(a, "-") || a == "exec" || a == "dlx" || a == "run" || a == "x":
		default:
			return i
		}
	}
	return -1
}

// literal is a word's text when it is fixed before the shell runs (quotes removed), "" when it holds an expansion.
func literal(w *syntax.Word) string {
	var b strings.Builder
	for _, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				l, ok := q.(*syntax.Lit)
				if !ok {
					return ""
				}
				b.WriteString(l.Value)
			}
		default:
			return ""
		}
	}
	return b.String()
}

// dropRunners is words without the leading transparent words, their flags and the values those take, env's NAME=VALUE words and
// timeout's duration.
func dropRunners(w []string) []string {
	for len(w) > 0 && (transparent[w[0]] || strings.HasPrefix(w[0], "-") || strings.Contains(w[0], "=")) {
		runner := w[0]
		w = w[1:]
		if runner == "-n" || runner == "-u" || runner == "-g" || runner == "-k" || runner == "-s" { // these take a value
			if len(w) > 0 {
				w = w[1:]
			}
			continue
		}
		if runner == "timeout" {
			for len(w) > 0 && strings.HasPrefix(w[0], "-") {
				if (w[0] == "-k" || w[0] == "-s") && len(w) > 1 {
					w = w[1:]
				}
				w = w[1:]
			}
			if len(w) > 0 {
				w = w[1:] // the duration
			}
		}
	}
	return w
}

// Starter is one command in a shell script that starts a test or build run, a simulator or a browser: its kind and words.
type Starter struct {
	Kind  string
	Words []string
}

// LeaseStarters are the commands in a shell script that start a test or build run, a simulator or a browser, in order; none when
// there is none or the script cannot be parsed. It reads the script as shell, so words in quotes and heredoc bodies are not commands;
// bash -c and sh -c are read one level deep. A command run through hl use is one call of hl, so what it runs is not read.
func LeaseStarters(script string) []Starter {
	return leaseStarters(script, 0)
}

func leaseStarters(script string, depth int) []Starter {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "")
	if err != nil {
		return nil
	}
	var out []Starter
	syntax.Walk(file, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		var w []string
		for _, a := range call.Args {
			w = append(w, literal(a))
		}
		if w = dropRunners(w); len(w) == 0 {
			return true
		}
		if k := leaseKind(w); k != "" {
			out = append(out, Starter{k, w})
		} else if shells[path.Base(w[0])] && depth == 0 {
			for i, a := range w[1:] {
				if a == "-c" && i+2 < len(w) {
					out = append(out, leaseStarters(w[i+2], depth+1)...)
					break
				}
			}
		}
		return true
	})
	return out
}

// LeaseGuard is the reason to refuse a Bash call that starts a test or build run, a simulator or a browser outside hl use while that
// kind has a limit (limit, 0 for none) and the agent holds none of it (holds), "" to let it through.
func LeaseGuard(tool string, toolInput any, limit func(kind string) int, holds func(kind string) bool) string {
	in, _ := toolInput.(map[string]any)
	command, ok := in["command"].(string)
	if tool != "Bash" || !ok {
		return ""
	}
	for _, st := range LeaseStarters(command) {
		if limit(st.Kind) > 0 && !holds(st.Kind) {
			return refusal(st, limit(st.Kind))
		}
	}
	return ""
}

func refusal(st Starter, limit int) string {
	kind, words := st.Kind, st.Words
	how := "hl use " + kind + " -- " + strings.Join(words, " ")
	if name := path.Base(words[0]); name == "xcrun" || name == "open" { // they return at once and leave the simulator running
		how = "hl use simulator --hold, then " + strings.Join(words, " ") + ", then hl use --release"
	}
	return "Refused: this starts a " + leases.Noun(kind) + " outside hl use while at most " + strconv.Itoa(limit) + " may run at once; nothing ran. Run " +
		how + "."
}

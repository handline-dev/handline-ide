package session

import (
	"path"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// transparent are the words that run the command after them as it is.
var transparent = map[string]bool{"exec": true, "time": true, "nice": true, "env": true, "command": true, "sudo": true, "caffeinate": true,
	"timeout": true}

var shells = map[string]bool{"bash": true, "sh": true, "zsh": true}

// leaseKind is the lease a command starts ("simulator" or "browser"), "" when it starts neither. words is one command, its leading
// assignments and transparent words already dropped.
func leaseKind(words []string) string {
	name := path.Base(words[0])
	rest := words[1:]
	has := func(w string) bool {
		for _, r := range rest {
			if r == w {
				return true
			}
		}
		return false
	}
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
		if has("test") || has("test-without-building") {
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
	case name == "npx" || name == "pnpx" || name == "bunx" || name == "pnpm" || name == "npm" || name == "yarn":
		if i := firstArg(rest); i >= 0 && path.Base(rest[i]) == "playwright" {
			return playwright(rest[i+1:])
		}
	case strings.Contains(strings.ToLower(name), "chrom"):
		for _, r := range rest {
			if strings.HasPrefix(r, "--headless") {
				return "browser"
			}
		}
	}
	return ""
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

// firstArg is the index of the program a package runner's arguments run, -1 for none: past the flags (and the values of those that
// take one) and any exec, dlx or run, so in `pnpm add playwright` it is add.
func firstArg(args []string) int {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--filter" || a == "-F" || a == "-C" || a == "--dir" || a == "-p" || a == "--package":
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

// LeaseStarter is the first command in a shell script that starts a simulator or a browser: its kind and words, "" when there is none
// or the script cannot be parsed. It reads the script as shell, so words in quotes and heredoc bodies are not commands; bash -c and
// sh -c are read one level deep. A command run through hl use is one call of hl, so what it runs is not read.
func LeaseStarter(script string) (kind string, words []string) {
	return leaseStarter(script, 0)
}

func leaseStarter(script string, depth int) (kind string, words []string) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "")
	if err != nil {
		return "", nil
	}
	syntax.Walk(file, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if kind != "" || !ok {
			return kind == ""
		}
		var w []string
		for _, a := range call.Args {
			w = append(w, literal(a))
		}
		if w = dropRunners(w); len(w) == 0 {
			return true
		}
		if k := leaseKind(w); k != "" {
			kind, words = k, w
		} else if shells[path.Base(w[0])] && depth == 0 {
			for i, a := range w[1:] {
				if a == "-c" && i+2 < len(w) {
					kind, words = leaseStarter(w[i+2], depth+1)
					break
				}
			}
		}
		return kind == ""
	})
	return kind, words
}

// LeaseGuard is the reason to refuse a Bash call that starts a simulator or a browser outside hl use while that kind has a limit
// (limit, 0 for none) and the agent holds none of it (holds), "" to let it through.
func LeaseGuard(tool string, toolInput any, limit func(kind string) int, holds func(kind string) bool) string {
	in, _ := toolInput.(map[string]any)
	command, ok := in["command"].(string)
	if tool != "Bash" || !ok {
		return ""
	}
	kind, words := LeaseStarter(command)
	if kind == "" || limit(kind) <= 0 || holds(kind) {
		return ""
	}
	how := "hl use " + kind + " -- " + strings.Join(words, " ")
	if name := path.Base(words[0]); name == "xcrun" || name == "open" { // they return at once and leave the simulator running
		how = "hl use simulator --hold, then " + strings.Join(words, " ") + ", then hl use --release"
	}
	return "Refused: this starts a " + kind + " outside hl use while at most " + strconv.Itoa(limit(kind)) + " may run at once; nothing ran. Run " + how + "."
}

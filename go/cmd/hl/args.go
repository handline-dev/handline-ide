package main

import (
	"fmt"
	"github.com/handline/handline/go/kit/policy"
	"os"
	"strconv"
	"strings"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/timefmt"
)

// parsed is a command line read against one command's options: the positionals in order, each option's last value under
// its long name (every value in each, for an option that may be given more than once), and each flag given.
type parsed struct {
	pos   []string
	opts  map[string]string
	each  map[string][]string
	flags map[string]bool
}

func (p parsed) opt(name string) (string, bool) { v, ok := p.opts[name]; return v, ok }

// argProblem is what was wrong with the command line, for the refusal main prints: parse names the option, a verb the piece it missed.
var argProblem string

// wrongArgs refuses the command line, saying why.
func wrongArgs(format string, a ...any) (int, bool) {
	argProblem = fmt.Sprintf(format, a...)
	return 0, false
}

// tags is every tag given under name, each value a tag or several split by commas (`--tag fe --tag be`, `--tag fe,be`).
func (p parsed) tags(name string) []any {
	var out []any
	for _, v := range p.each[name] {
		for _, t := range strings.Split(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

func (p *parsed) set(name, value string) {
	p.opts[name] = value
	p.each[name] = append(p.each[name], value)
}

// parse reads args with these options: valued names take the next argument or `--name=value`, flags take none, and
// alias maps a short name to its long one. ok is false for a form it does not take: an unknown or abbreviated option,
// `--`, `-h`, a valued option with nothing after it. The command then refuses its arguments with exit 2.
func parse(args []string, valued, flags []string, alias map[string]string) (parsed, bool) {
	p := parsed{opts: map[string]string{}, each: map[string][]string{}, flags: map[string]bool{}}
	isValued := map[string]bool{}
	for _, v := range valued {
		isValued[v] = true
	}
	isFlag := map[string]bool{}
	for _, f := range flags {
		isFlag[f] = true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !looksLikeOption(a) {
			p.pos = append(p.pos, a)
			continue
		}
		name, value, inline := strings.Cut(a, "=")
		if long, ok := alias[name]; ok {
			name = long
		}
		switch {
		case isFlag[name] && !inline:
			p.flags[name] = true
		case isValued[name] && inline:
			p.set(name, value)
		case isValued[name] && i+1 < len(args) && !looksLikeOption(args[i+1]):
			i++
			p.set(name, args[i])
		case isFlag[name]:
			argProblem = name + " takes no value"
			return p, false
		case isValued[name]:
			argProblem = name + " needs a value after it"
			return p, false
		default:
			argProblem = "unknown option " + name
			return p, false
		}
	}
	return p, true
}

// looksLikeOption is whether a is taken for an option: a dash and more, `--` included. "-" alone is a value (stdin).
func looksLikeOption(a string) bool { return len(a) > 1 && a[0] == '-' }

// The checks of the task commands' argument values: each is ok false for a value the command refuses.

func taskRef(text string) (int, bool) {
	t := strings.TrimSpace(text)
	if len(t) < 2 || (t[0] != 'T' && t[0] != 't') || !digits(t[1:]) {
		return 0, false
	}
	n, err := strconv.Atoi(t[1:])
	return n, err == nil
}

func taskRefs(texts []string) ([]int, bool) {
	out := make([]int, 0, len(texts))
	for _, t := range texts {
		n, ok := taskRef(t)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func priority(text string) (int, bool) {
	t := strings.ToUpper(strings.TrimSpace(text))
	t = strings.TrimPrefix(t, "P")
	if t == "1" || t == "2" || t == "3" {
		return int(t[0] - '0'), true
	}
	return 0, false
}

func expects(text string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(text))
	switch t {
	case "sha", "verdict", "decision", "report":
		return t, true
	}
	return "", false
}

// taskTimeout reads a span; how long one may be is the daemon's to say, so a span past it is refused there, with the limit named.
func taskTimeout(text string) (int64, bool) {
	secs, err := timefmt.ParseSpan(text)
	return secs, err == nil
}

// integer reads plain decimal digits; a sign, spaces or anything else is not an integer.
func integer(text string) (int, bool) {
	if !digits(text) {
		return 0, false
	}
	n, err := strconv.Atoi(text)
	return n, err == nil
}

// requestArg reads a message as typed: msg_<hash> or the bare hash is its ref. A hash may be all digits, so only digits shorter than a
// printed name (an id from an old notice) are an id. A #<n> names a pull request: it goes to the daemon as typed, which refuses it and
// says so.
func requestArg(text string) (any, bool) {
	if strings.HasPrefix(text, "#") {
		return text, true
	}
	t := strings.TrimPrefix(strings.ToLower(text), policy.MsgPrefix)
	if digits(t) && len(t) < policy.MsgMinPrefix && !strings.HasPrefix(strings.ToLower(text), policy.MsgPrefix) {
		return integer(t)
	}
	if len(t) >= 6 && len(t) <= 12 && strings.Trim(t, "0123456789abcdef") == "" {
		return t, true
	}
	return nil, false
}

func digits(s string) bool { return s != "" && strings.Trim(s, "0123456789") == "" }

// textArg is a text argument given inline or with --file, never both; "-" as either reads stdin, and so does neither when
// stdin is not a terminal. (text, "") or ("", why not).
func textArg(inline *string, file, label string) (string, string) {
	if file != "" && inline != nil {
		return "", fmt.Sprintf("give %s inline or with --file, not both", label)
	}
	if file == "-" || (inline != nil && *inline == "-") {
		text := readStdin()
		if strings.TrimSpace(text) == "" {
			return "", fmt.Sprintf("%s is required, and stdin (-) was empty", label)
		}
		return text, ""
	}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Sprintf("could not read %s: %v", file, err)
		}
		return string(b), ""
	}
	if inline == nil {
		text := ""
		if !hlclient.StdinIsTerminal() {
			text = readStdin()
		}
		if strings.TrimSpace(text) == "" {
			return "", fmt.Sprintf("%s is required (inline, --file PATH, or on stdin)", label)
		}
		return text, ""
	}
	return *inline, ""
}

func readStdin() string {
	var b strings.Builder
	buf := make([]byte, 64<<10)
	for {
		n, err := os.Stdin.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			return b.String()
		}
	}
}

// shQuote quotes s for a POSIX shell: as it is when every character is safe, else in single quotes (a ' inside as '"'"').
func shQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(r < 128 && (r == '_' || r == '@' || r == '%' || r == '+' || r == '=' || r == ':' || r == ',' || r == '.' ||
			r == '/' || r == '-' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9'))) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// cleanPath is p as a clean path: repeated slashes and "." parts dropped, no trailing slash, "." for nothing.
func cleanPath(p string) string {
	abs := strings.HasPrefix(p, "/")
	var parts []string
	for _, part := range strings.Split(p, "/") {
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}
	out := strings.Join(parts, "/")
	switch {
	case abs:
		return "/" + out
	case out == "":
		return "."
	}
	return out
}

// badArgs answers a command line its parser does not take: asked for with -h or --help it is printed and exits 0,
// otherwise it goes to stderr and exits 2.
func badArgs(text string, args []string) int {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Println(text)
			return 0
		}
	}
	fmt.Fprintln(os.Stderr, text)
	return 2
}

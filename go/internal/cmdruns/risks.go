package cmdruns

import (
	"path/filepath"
	"slices"
	"strings"
)

// The risk marks of a proposed command (docs/design/agent-run-proposals.md, The card): hints for the founder reading it, from its text
// alone. They never approve or block anything, and a command with none is not safe.
const (
	RiskDelete      = "delete"
	RiskOverwrite   = "overwrite"
	RiskNetwork     = "network"
	RiskOutside     = "outside"      // a path not under the cwd
	RiskOutsideHome = "outside_home" // a path outside the founder's home: marked as well as outside
	RiskPrivilege   = "privilege"
)

// Risks is the marks of command run in cwd, for a founder whose home is home, in a fixed order. The command is read as the shell
// splits it: quotes, `$(...)`, `sh -c` and eval included. It does not see a command or path built at run time (`$x`, `$HOME`), a
// script the command runs, or a backslash-escaped space (`rm\ x` is one word).
func Risks(command, cwd, home string) []string {
	marks := map[string]bool{}
	for _, words := range simpleCommands(command, marks) {
		markCommand(words, cwd, home, marks)
	}
	var out []string
	for _, m := range []string{RiskDelete, RiskOverwrite, RiskNetwork, RiskOutside, RiskOutsideHome, RiskPrivilege} {
		if marks[m] {
			out = append(out, m)
		}
	}
	return out
}

// simpleCommands splits command into its simple commands, each its words unquoted. A redirect to a file marks overwrite in marks, and
// its target is kept as a word so its path is judged.
func simpleCommands(command string, marks map[string]bool) [][]string {
	var cmds [][]string
	var words []string
	var word strings.Builder
	inWord := false
	flushWord := func() {
		if inWord {
			words = append(words, word.String())
		}
		word.Reset()
		inWord = false
	}
	flushCmd := func() {
		flushWord()
		if len(words) > 0 {
			cmds = append(cmds, words)
		}
		words = nil
	}
	rs := []rune(command)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\'':
			inWord = true
			for i++; i < len(rs) && rs[i] != '\''; i++ {
				word.WriteRune(rs[i])
			}
		case c == '"':
			inWord = true
			for i++; i < len(rs) && rs[i] != '"'; i++ {
				if rs[i] == '\\' && i+1 < len(rs) {
					i++
				}
				if rs[i] == '$' && i+1 < len(rs) && rs[i+1] == '(' || rs[i] == '`' { // a substitution inside quotes still runs
					end, close := i+1, '`'
					if rs[i] == '$' {
						end, close = i+2, ')'
					}
					from := end
					for end < len(rs) && rs[end] != close && rs[end] != '"' {
						end++
					}
					cmds = append(cmds, simpleCommands(string(rs[from:end]), marks)...)
					i = end
					if end < len(rs) && rs[end] == '"' { // unclosed: the loop's step lands on the closing quote
						i--
					}
					continue
				}
				word.WriteRune(rs[i])
			}
		case c == '\\' && i+1 < len(rs):
			i++
			word.WriteRune(rs[i])
			inWord = true
		case c == '$' && i+1 < len(rs) && rs[i+1] == '(':
			flushCmd()
			i++
		case strings.ContainsRune(";&|()`\n{}", c):
			flushCmd()
		case c == '>':
			// fd digits before it (2>) are not the target; >& and >/dev/null write no file
			if inWord && strings.Trim(word.String(), "0123456789") == "" {
				word.Reset()
				inWord = false
			}
			flushWord()
			if i+1 < len(rs) && rs[i+1] == '>' {
				i++
			}
			if i+1 < len(rs) && rs[i+1] == '&' {
				i++
				continue
			}
			j := i + 1
			for j < len(rs) && rs[j] == ' ' {
				j++
			}
			if !strings.HasPrefix(string(rs[j:]), "/dev/null") {
				marks[RiskOverwrite] = true
			}
		case c == '<':
			flushWord()
		case c == ' ' || c == '\t':
			flushWord()
		default:
			word.WriteRune(c)
			inWord = true
		}
	}
	flushCmd()
	return cmds
}

// wrappers run the command that follows them.
var wrappers = []string{"env", "exec", "nohup", "time", "nice", "xargs", "command", "builtin", "caffeinate"}

var (
	deletes    = []string{"rm", "rmdir", "shred", "unlink", "srm"}
	overwrites = []string{"mv", "dd", "truncate"}
	networks   = []string{"curl", "wget", "ssh", "scp", "rsync", "sftp", "ftp", "nc", "telnet"}
	privileges = []string{"sudo", "su", "doas"}
	shells     = []string{"sh", "bash", "zsh", "dash", "ksh", "fish"}
)

func markCommand(words []string, cwd, home string, marks map[string]bool) {
	for _, w := range words {
		markPath(w, cwd, home, marks)
	}
	for len(words) > 0 && (strings.Contains(words[0], "=") && !strings.HasPrefix(words[0], "=") || slices.Contains(wrappers, filepath.Base(words[0]))) {
		words = words[1:]
	}
	if len(words) == 0 {
		return
	}
	name, args := filepath.Base(words[0]), words[1:]
	if slices.Contains(privileges, name) {
		marks[RiskPrivilege] = true
		var rest []string
		for i := 0; i < len(args); i++ {
			if a := args[i]; !strings.HasPrefix(a, "-") {
				rest = args[i:]
				break
			} else if a == "-u" || a == "-g" || a == "-C" || a == "-h" && name == "sudo" { // an option with a value
				i++
			}
		}
		markCommand(rest, cwd, home, marks)
		return
	}
	if slices.Contains(shells, name) || name == "eval" { // sh -c '<script>', eval '<script>': the script is commands too
		for i, a := range args {
			if name == "eval" || a == "-c" && i+1 < len(args) {
				script := a
				if name != "eval" {
					script = args[i+1]
				}
				for _, words := range simpleCommands(script, marks) {
					markCommand(words, cwd, home, marks)
				}
				if name != "eval" {
					break
				}
			}
		}
		return
	}
	has := func(flags ...string) bool {
		for _, a := range args {
			if slices.Contains(flags, a) {
				return true
			}
		}
		return false
	}
	sub := ""
	for i := 0; i < len(args); i++ {
		if a := args[i]; !strings.HasPrefix(a, "-") {
			sub = a
			break
		} else if name == "git" && (a == "-C" || a == "-c") { // git -C <dir> clean
			i++
		}
	}
	switch {
	case slices.Contains(deletes, name):
		marks[RiskDelete] = true
	case slices.Contains(overwrites, name):
		marks[RiskOverwrite] = true
	case slices.Contains(networks, name):
		marks[RiskNetwork] = true
	case name == "find" && has("-delete", "-exec", "-execdir", "-ok"):
		if has("-delete") {
			marks[RiskDelete] = true
		}
		for i, a := range args { // find -exec rm {} ;
			if (a == "-exec" || a == "-execdir" || a == "-ok") && i+1 < len(args) {
				markCommand(args[i+1:], cwd, home, marks)
			}
		}
	case name == "sed" && slices.ContainsFunc(args, func(a string) bool { return a == "-i" || strings.HasPrefix(a, "-i") && !strings.HasPrefix(a, "--") }):
		marks[RiskOverwrite] = true
	case (name == "chmod" || name == "chown" || name == "chgrp") && has("-R"):
		marks[RiskPrivilege] = true
	case name == "git":
		switch sub {
		case "clean":
			marks[RiskDelete] = true
		case "branch":
			if has("-D", "-d", "--delete") {
				marks[RiskDelete] = true
			}
		case "reset":
			if has("--hard") {
				marks[RiskOverwrite] = true
			}
		case "checkout", "restore":
			if has("--", ".", "-f", "--force") {
				marks[RiskOverwrite] = true
			}
		case "push":
			marks[RiskNetwork] = true
			if has("--force", "-f", "--force-with-lease", "--delete") {
				marks[RiskOverwrite] = true
			}
		case "pull", "fetch", "clone":
			marks[RiskNetwork] = true
		}
	case slices.Contains([]string{"npm", "pnpm", "yarn", "pip", "pip3", "brew", "gem", "cargo", "go"}, name):
		if slices.Contains([]string{"install", "i", "add", "publish", "get", "upgrade", "update"}, sub) {
			marks[RiskNetwork] = true
		}
	}
}

// markPath marks a word that is a path outside cwd: absolute, ~ or with a .. that climbs out of it. Flags and URLs are not paths.
func markPath(w, cwd, home string, marks map[string]bool) {
	if i := strings.Index(w, "="); i > 0 {
		w = w[i+1:] // --out=/tmp/x, of=/etc/x
	}
	if w == "" || strings.HasPrefix(w, "-") || strings.Contains(w, "://") {
		return
	}
	var p string
	switch {
	case w == "~" || strings.HasPrefix(w, "~/"):
		p = filepath.Join(home, strings.TrimPrefix(w, "~"))
	case strings.HasPrefix(w, "~"):
		p = "/" // another user's home
	case filepath.IsAbs(w):
		p = filepath.Clean(w)
	case w == ".." || strings.HasPrefix(w, "../") || strings.Contains(w, "/../") || strings.HasSuffix(w, "/.."):
		p = filepath.Join(cwd, w)
	default:
		return
	}
	if p == "/dev/null" || under(p, cwd) {
		return
	}
	marks[RiskOutside] = true
	if home == "" || !under(p, home) {
		marks[RiskOutsideHome] = true
	}
}

func under(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

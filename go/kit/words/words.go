// Package words writes small things the way an English sentence says them: a place in a row, a list of names.
package words

import (
	"fmt"
	"strings"
)

// Ordinal is n as a place: 1st, 2nd, 3rd, 4th, 11th, 21st.
func Ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// And joins the items as a list: "a", "a and b", "a, b and c".
func And(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// Command is a shell line as a person reads it: the program it runs and that program's first word ("xcodebuild test", "vite dev"),
// past the variables it sets, the folders it changes to and the wrappers it runs through. A line with no program reads as itself.
func Command(line string) string {
	cmds := splitCommands(line)
	for _, setup := range []bool{false, true} {
		for _, c := range cmds {
			if w := commandWords(c, setup); w != "" {
				return w
			}
		}
	}
	return strings.TrimSpace(line)
}

// setupPrograms set the stage for the work and are named only when a line runs nothing else.
var setupPrograms = map[string]bool{"cd": true, "pushd": true, "export": true, "set": true, "unset": true, "source": true, ".": true,
	"echo": true, "printf": true, "sleep": true, "true": true, "mkdir": true, "rm": true, "trap": true, "wait": true, "local": true}

// wrappers run the program after them, past their own flags.
var wrappers = map[string]bool{"env": true, "nohup": true, "time": true, "exec": true, "sudo": true, "nice": true, "command": true,
	"caffeinate": true, "timeout": true, "gtimeout": true, "npx": true, "bunx": true, "uvx": true, "xcrun": true}

// valueFlags are the wrappers' flags whose value is the next word: nice -n 10, sudo -u root, timeout -k 5.
var valueFlags = map[string]bool{"-n": true, "-u": true, "-g": true, "-k": true, "-s": true, "-w": true, "-t": true}

// shells run their script, or with -c the line after it.
var shells = map[string]bool{"sh": true, "bash": true, "zsh": true}

// commandWords is one simple command's program and first word, "" for none or, unless setup, for a setup program.
func commandWords(words []string, setup bool) string {
	i := 0
	for i < len(words) && isAssignment(words[i]) {
		i++
	}
	for i < len(words) && wrappers[base(words[i])] {
		w := base(words[i])
		i++
		for i < len(words) && (strings.HasPrefix(words[i], "-") || isAssignment(words[i]) || (strings.HasSuffix(w, "timeout") && isDuration(words[i]))) {
			if valueFlags[words[i]] {
				i++
			}
			i++
		}
	}
	if i >= len(words) || isRedirect(words[i]) {
		return ""
	}
	prog := base(words[i])
	if !setup && setupPrograms[prog] {
		return ""
	}
	if shells[prog] && i+1 < len(words) {
		if words[i+1] == "-c" && i+2 < len(words) {
			return Command(words[i+2])
		}
		if !strings.HasPrefix(words[i+1], "-") && !isRedirect(words[i+1]) {
			return commandWords(words[i+1:], setup)
		}
	}
	if i+1 < len(words) && isWord(words[i+1]) {
		return prog + " " + words[i+1]
	}
	return prog
}

func base(w string) string {
	if j := strings.LastIndexByte(w, '/'); j >= 0 && j < len(w)-1 {
		return w[j+1:]
	}
	return w
}

func isAssignment(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range w[:eq] {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func isDuration(w string) bool { return w != "" && w[0] >= '0' && w[0] <= '9' }

func isRedirect(w string) bool {
	w = strings.TrimLeft(w, "0123456789&")
	return strings.HasPrefix(w, ">") || strings.HasPrefix(w, "<")
}

// isWord is a subcommand as a person says it: a plain word, no flag, path or variable.
func isWord(w string) bool {
	if w == "" || !(w[0] >= 'a' && w[0] <= 'z' || w[0] >= 'A' && w[0] <= 'Z') {
		return false
	}
	return !strings.ContainsAny(w, "/$=<>`")
}

// splitCommands cuts a shell line into its simple commands at ; & | ( ) and line breaks outside quotes, each as its words with the
// quotes taken off; a $(...) stays inside its word.
func splitCommands(line string) [][]string {
	var cmds [][]string
	var words []string
	var cur strings.Builder
	inWord := false
	endWord := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	endCmd := func() {
		endWord()
		if len(words) > 0 {
			cmds = append(cmds, words)
			words = nil
		}
	}
	var quote rune
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote == '"' && r == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
		case quote != '\'' && r == '$' && i+1 < len(rs) && rs[i+1] == '(':
			depth, j := 0, i+1
			for ; j < len(rs); j++ {
				if rs[j] == '(' {
					depth++
				} else if rs[j] == ')' {
					if depth--; depth == 0 {
						break
					}
				}
			}
			cur.WriteString(string(rs[i:min(j+1, len(rs))]))
			i, inWord = j, true
		case quote != 0:
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == '\\' && i+1 < len(rs):
			i++
			cur.WriteRune(rs[i])
			inWord = true
		case r == ' ' || r == '\t':
			endWord()
		case strings.ContainsRune(";&|()\n", r):
			if r == '&' && (i > 0 && rs[i-1] == '>' || inWord && strings.HasSuffix(cur.String(), ">")) {
				cur.WriteRune(r) // 2>&1
				continue
			}
			endCmd()
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	endCmd()
	return cmds
}

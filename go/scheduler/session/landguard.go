package session

import (
	"fmt"
	"regexp"
	"strings"
)

// Land is the two kept rules about landing work: who alone may push to a repo's default branch, and the command a push there must pass.
// Empty is no rule.
type Land struct{ By, Check string }

// Git is what LandGuard asks of the repo a push is run in: the output of `git args...` in dir, "" when it fails.
type Git func(dir string, args ...string) string

// Check runs the land_check command in dir; the tail of its output and false when it fails.
type Check func(dir, command string) (string, bool)

var (
	segmentRE = regexp.MustCompile(`&&|\|\||[;|\n&]`)
	cdRE      = regexp.MustCompile(`^cd[` + space + `]+(\S+)$`)
)

// push is one `git push` of a command: where it runs and its arguments after "push".
type push struct {
	dir  string
	args []string
}

// pushesOf is the git pushes a Bash command line holds, a `cd DIR &&` before one or a `git -C DIR` moving where it runs.
func pushesOf(command, cwd string) []push {
	var out []push
	dir := cwd
	for _, seg := range segmentRE.Split(command, -1) {
		f := strings.Fields(seg)
		if m := cdRE.FindStringSubmatch(strings.TrimSpace(seg)); m != nil {
			dir = joinDir(dir, m[1])
			continue
		}
		for len(f) > 0 && (f[0] == "env" || f[0] == "command" || f[0] == "sudo" || strings.Contains(f[0], "=")) {
			f = f[1:] // FOO=1 git push, env git push
		}
		if len(f) == 0 || f[0] != "git" {
			continue
		}
		at, here := 1, dir
		for at < len(f) && strings.HasPrefix(f[at], "-") {
			if f[at] == "-C" && at+1 < len(f) {
				here = joinDir(here, f[at+1])
				at++
			} else if f[at] == "-c" {
				at++
			}
			at++
		}
		if at < len(f) && f[at] == "push" {
			out = append(out, push{here, f[at+1:]})
		}
	}
	return out
}

func joinDir(dir, to string) string {
	to = strings.Trim(to, `"'`)
	if strings.HasPrefix(to, "/") || dir == "" {
		return to
	}
	return strings.TrimRight(dir, "/") + "/" + to
}

// PushesToGit is whether a Bash command holds a git push at all: the cheap question asked before the daemon is.
func PushesToGit(tool string, toolInput any) bool {
	in, _ := toolInput.(map[string]any)
	command, _ := in["command"].(string)
	return tool == "Bash" && len(pushesOf(command, "")) > 0
}

// onDefault is the default branch if the push updates it, "" if not.
func (p push) onDefault(git Git) string {
	def := git(p.dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	def = strings.TrimPrefix(strings.TrimSpace(def), "origin/")
	if def == "" {
		for _, b := range []string{"main", "master"} {
			if git(p.dir, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+b) != "" {
				def = b
				break
			}
		}
	}
	if def == "" {
		return ""
	}
	var refs []string
	for _, a := range p.args {
		switch {
		case a == "-d" || a == "--delete" || a == "-n" || a == "--dry-run":
			return "" // taking a branch off, or trying a push, is not landing on it
		case a == "--all" || a == "--mirror":
			return def
		case !strings.HasPrefix(a, "-"):
			refs = append(refs, strings.Trim(a, `"'`))
		}
	}
	if len(refs) > 0 {
		refs = refs[1:] // the remote
	}
	current := ""
	if len(refs) == 0 { // `git push`, `git push origin`: the branch checked out
		refs = []string{"HEAD"}
	}
	for _, r := range refs {
		dst := strings.TrimPrefix(r, "+")
		if i := strings.LastIndex(dst, ":"); i >= 0 {
			if i == 0 {
				continue // :main deletes it
			}
			dst = dst[i+1:]
		}
		if dst == "HEAD" || dst == "" {
			if current == "" {
				current = strings.TrimSpace(git(p.dir, "rev-parse", "--abbrev-ref", "HEAD"))
			}
			dst = current
		}
		if strings.TrimPrefix(dst, "refs/heads/") == def {
			return def
		}
	}
	return ""
}

// LandGuard is the reason to refuse a Bash call that pushes to a repo's default branch under the workspace's land rules, "" to let it
// through. agent is who runs it; cwd is where.
func LandGuard(tool string, toolInput any, agent, cwd string, rules Land, git Git, check Check) string {
	in, _ := toolInput.(map[string]any)
	command, _ := in["command"].(string)
	if tool != "Bash" || (rules.By == "" && rules.Check == "") {
		return ""
	}
	checked := map[string]bool{} // a command with two pushes in one folder runs the check once
	for _, p := range pushesOf(command, cwd) {
		branch := p.onDefault(git)
		if branch == "" {
			continue
		}
		if rules.By != "" && agent != rules.By {
			return fmt.Sprintf("Refused: land_by says only %s pushes to %s in this workspace. Hand the branch to it with hl task handoff.", rules.By, branch)
		}
		if rules.Check != "" && !checked[p.dir] {
			checked[p.dir] = true
			if tail, ok := check(p.dir, rules.Check); !ok {
				return fmt.Sprintf("Refused: land_check `%s` failed, so nothing was pushed to %s. The end of its output:\n%s", rules.Check, branch, tail)
			}
		}
	}
	return ""
}

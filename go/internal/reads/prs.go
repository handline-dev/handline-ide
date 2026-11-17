package reads

import (
	"regexp"
	"strings"
)

var githubRemote = regexp.MustCompile(`^(?:https?://(?:[^@/]+@)?github\.com/|git@github\.com:|ssh://git@github\.com/)([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+?)(?:\.git)?/?$`)

// githubRepo is owner/repo of a GitHub remote url (https or ssh), "" for anything else.
func githubRepo(url string) string {
	m := githubRemote.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return ""
	}
	return m[1] + "/" + m[2]
}

// originRepo is the GitHub repo of the origin of the recorded worktree ("<repo>::<path>" or a path), "" when it has none or is not on GitHub.
func originRepo(recorded string) string {
	path := worktreePath(recorded)
	if path == "" {
		return ""
	}
	ok, out := git(path, "remote", "get-url", "origin")
	if !ok {
		return ""
	}
	return githubRepo(out)
}

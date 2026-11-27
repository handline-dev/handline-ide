package watch

import (
	"regexp"
	"slices"
	"strings"
)

var (
	repoName     = regexp.MustCompile(`^([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)$`)
	githubTarget = regexp.MustCompile(`^([^#]+)#([1-9][0-9]*)$`)
	pullLink     = regexp.MustCompile(`https://github\.com/([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)/pull/([1-9][0-9]*)\b`)
	githubRemote = regexp.MustCompile(`^(?:https?://(?:[^@/]+@)?github\.com/|git@github\.com:|ssh://git@github\.com/)([A-Za-z0-9._-]+/[A-Za-z0-9._-]+?)(?:\.git)?/?$`)
)

// RepoName is whether s names a repository as GitHub does: owner/name. Neither part is "." or "..": those are steps of a path and no
// name, and the answer is handed to gh as an argument and joined into a link.
func RepoName(s string) bool {
	m := repoName.FindStringSubmatch(s)
	return m != nil && m[1] != "." && m[1] != ".." && m[2] != "." && m[2] != ".."
}

// splitTarget is the repository and number of a target owner/name#N; ok is false for anything else.
func splitTarget(target string) (repo, number string, ok bool) {
	m := githubTarget.FindStringSubmatch(target)
	if m == nil || !RepoName(m[1]) {
		return "", "", false
	}
	return m[1], m[2], true
}

// RemoteRepo is owner/name of a remote's url when it is a repository on github.com (https or ssh), "" for anything else.
func RemoteRepo(url string) string {
	m := githubRemote.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil || !RepoName(m[1]) {
		return ""
	}
	return m[1]
}

// SameRepo is whether two owner/name are one repository: GitHub's names do not tell upper case from lower.
func SameRepo(a, b string) bool { return a != "" && strings.EqualFold(a, b) }

// PullLinks are the pull requests text links to on github.com, each once as owner/name#N, in the order it names them.
func PullLinks(text string) []string {
	var out []string
	for _, m := range pullLink.FindAllStringSubmatch(text, -1) {
		if !RepoName(m[1]) {
			continue
		}
		t := m[1] + "#" + m[2]
		if !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, t) }) {
			out = append(out, t)
		}
	}
	return out
}

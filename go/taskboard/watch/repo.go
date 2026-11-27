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

// SameRepo is whether two repositories, as PullRepo names them, are one: neither GitHub's nor GitLab's names tell upper case from lower.
func SameRepo(a, b string) bool { return a != "" && strings.EqualFold(a, b) }

// PullLinks are the pull requests text links to, each once, in the order it names them: owner/name#N on github.com, host/path!N for a
// merge request on a GitLab host.
func PullLinks(text string) []string {
	type link struct {
		at     int
		target string
	}
	var links []link
	for _, m := range pullLink.FindAllStringSubmatchIndex(text, -1) {
		if repo := text[m[2]:m[3]]; RepoName(repo) {
			links = append(links, link{m[0], repo + "#" + text[m[4]:m[5]]})
		}
	}
	for _, m := range labLink.FindAllStringSubmatchIndex(text, -1) {
		if repo := strings.ToLower(text[m[2]:m[3]]) + "/" + text[m[4]:m[5]]; HostRepo(repo) {
			links = append(links, link{m[0], repo + "!" + text[m[6]:m[7]]})
		}
	}
	slices.SortFunc(links, func(a, b link) int { return a.at - b.at })
	var out []string
	for _, l := range links {
		if !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, l.target) }) {
			out = append(out, l.target)
		}
	}
	return out
}

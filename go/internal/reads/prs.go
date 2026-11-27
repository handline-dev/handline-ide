package reads

import "github.com/handline/handline/go/taskboard/watch"

// originOf is the repository of the origin of the recorded worktree ("<repo>::<path>" or a path) as watch.OriginRepo names it: owner/name
// on GitHub, host/path elsewhere; "" when it has none.
func originOf(recorded string) string {
	path := worktreePath(recorded)
	if path == "" {
		return ""
	}
	ok, out := git(path, "remote", "get-url", "origin")
	if !ok {
		return ""
	}
	return watch.OriginRepo(out)
}

// originRepo is the GitHub repo (owner/name) of the origin of the recorded worktree, "" when it has none or is not on GitHub.
func originRepo(recorded string) string {
	if r := originOf(recorded); watch.RepoName(r) {
		return r
	}
	return ""
}

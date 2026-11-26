package reads

import "github.com/handline/handline/go/taskboard/watch"

// githubRepo is owner/repo of a GitHub remote url (https or ssh), "" for anything else.
func githubRepo(url string) string { return watch.RemoteRepo(url) }

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

package watch

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/handline/handline/go/data/db"
)

// GitHubHost is the one host whose pull requests gh reads. Its repositories are named owner/name; a repository on any other host is
// named host/path (gitlab.example.com/group/name), which is how GitLab's own -R takes one.
const GitHubHost = "github.com"

var (
	urlRemote = regexp.MustCompile(`^(?:https?|ssh|git)://(?:[^@/]+@)?([^/:@]+)(?::[0-9]+)?/(.+?)(?:\.git)?/?$`)
	scpRemote = regexp.MustCompile(`^(?:[^@/]+@)?([^/:@]+):([^/].*?)(?:\.git)?/?$`)
	pathPart  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	labTarget = regexp.MustCompile(`^([^!]+)!([1-9][0-9]*)$`)
	labLink   = regexp.MustCompile(`https://([A-Za-z0-9.-]+\.[A-Za-z0-9-]+)/((?:[A-Za-z0-9._-]+/)+[A-Za-z0-9._-]+)/-/merge_requests/([1-9][0-9]*)\b`)
)

// Remote is the host (lower case) and repository path of a remote url, https or ssh; both "" for a local path or anything else.
func Remote(url string) (host, path string) {
	url = strings.TrimSpace(url)
	m := urlRemote.FindStringSubmatch(url)
	if m == nil {
		m = scpRemote.FindStringSubmatch(url)
	}
	if m == nil || !strings.Contains(m[1], ".") || !repoPath(m[2], 2) {
		return "", ""
	}
	return strings.ToLower(m[1]), m[2]
}

// repoPath is at least min parts of a path, none "." or "..", which are steps of a path and no name.
func repoPath(s string, min int) bool {
	parts := strings.Split(s, "/")
	if len(parts) < min {
		return false
	}
	for _, p := range parts {
		if !pathPart.MatchString(p) || p == "." || p == ".." {
			return false
		}
	}
	return true
}

// OriginRepo is the repository of a remote url as its pull requests are named: owner/name on github.com, host/path on any other host;
// "" for a local path.
func OriginRepo(url string) string {
	host, path := Remote(url)
	switch {
	case host == "":
		return ""
	case host == GitHubHost:
		if RepoName(path) {
			return path
		}
		return ""
	}
	return host + "/" + path
}

// HostRepo is whether s names a repository on a host other than GitHub: host/path, the path of two parts or more.
func HostRepo(s string) bool {
	host, path, ok := strings.Cut(s, "/")
	return ok && strings.Contains(host, ".") && repoPath(host, 1) && !strings.EqualFold(host, GitHubHost) && repoPath(path, 2)
}

// PullRepo is whether s names a repository whose pull requests a task may name: owner/name on GitHub, host/path elsewhere.
func PullRepo(s string) bool { return RepoName(s) || HostRepo(s) }

// HostOf is the host of a repository as PullRepo names it.
func HostOf(repo string) string {
	if RepoName(repo) {
		return GitHubHost
	}
	host, _, _ := strings.Cut(repo, "/")
	return strings.ToLower(host)
}

// PullTarget is the watch on pull request n of repo: github owner/name#N, or gitlab host/path!N, as each host writes its own.
func PullTarget(repo string, n int64) (vendor, target string) {
	if RepoName(repo) {
		return "github", repo + "#" + strconv.FormatInt(n, 10)
	}
	return "gitlab", repo + "!" + strconv.FormatInt(n, 10)
}

// splitLab is the repository and number of a GitLab target host/path!N; ok is false for anything else.
func splitLab(target string) (repo string, number int64, ok bool) {
	m := labTarget.FindStringSubmatch(target)
	if m == nil || !HostRepo(m[1]) {
		return "", 0, false
	}
	n, err := strconv.ParseInt(m[2], 10, 64)
	return m[1], n, err == nil
}

// UnreadHost is what a card and a done say for a repository whose host no reader here reads.
func UnreadHost(host string) string {
	return fmt.Sprintf("Pull requests on %s are not read yet; this repository lands by push.", host)
}

// Reader reads one host's pull requests into Pull. Name is the host as people know it.
type Reader interface {
	Name() string
	Pull(ctx context.Context, repo string, number int64) (*Pull, error)
	BranchPull(ctx context.Context, repo, branch string) (*Pull, error)
	Unready() (reason, why string)
}

// ReaderOf is the reader of repo's pull requests: gh for github.com, glab for a host glab is logged in to here; nil for any other, a
// repository that lands by push (UnreadHost). A nil gh or gl reads nothing.
func ReaderOf(ctx context.Context, repo string, gh *GitHub, gl *GitLab) Reader {
	switch {
	case RepoName(repo):
		if gh != nil {
			return gh
		}
	case HostRepo(repo):
		if gl != nil && gl.Reads(ctx, HostOf(repo)) {
			return gl
		}
	}
	return nil
}

// Reads is whether this daemon's own readers (Vendors) read repo's pull requests.
func Reads(ctx context.Context, repo string) bool {
	gh, _ := Vendors["github"].(*GitHub)
	gl, _ := Vendors["gitlab"].(*GitLab)
	return ReaderOf(ctx, repo, gh, gl) != nil
}

// Landing is the watch on a pull request link's target (as PullLinks writes it) until it lands.
func Landing(target string) (db.Watch, error) {
	vendor := "github"
	if strings.Contains(target, "!") {
		vendor = "gitlab"
	}
	return Parse(vendor, target, "landed")
}

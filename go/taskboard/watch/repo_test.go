package watch

import (
	"context"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
)

// A repository is owner/name as GitHub names one. A step of a path is no name: "../.." has the letters of one and is refused wherever a
// repository is taken, in a target a gate watches and in a pull request a task names, before gh is ever run with it.
func TestARepositoryNameIsNeverAStepOfAPath(t *testing.T) {
	for name, want := range map[string]bool{
		"acme/app": true, "a-b/c_d.e": true, "acme/.github": true, "acme/...": true,
		"../..": false, "acme/..": false, "./app": false, "acme/.": false, "acme": false, "acme/app/x": false, "": false, "acme/a b": false,
	} {
		if RepoName(name) != want {
			t.Errorf("RepoName(%q) is %v", name, !want)
		}
	}
	ran := 0
	g := &GitHub{Run: func(context.Context, ...string) ([]byte, error) { ran++; return nil, nil }}
	for _, target := range []string{"../..#7", "acme/..#7", "./app#7"} {
		if _, _, err := g.Parse(target, "landed"); err == nil {
			t.Errorf("%s was taken as a target", target)
		}
		if _, _, err := g.Check(context.Background(), target, "landed", time.Time{}); err == nil {
			t.Errorf("%s was read", target)
		}
		if _, _, ok := PullOf(db.Watch{Vendor: "github", Target: target, Until: "landed"}); ok {
			t.Errorf("%s names a pull request", target)
		}
	}
	if ran != 0 {
		t.Fatalf("gh ran %d times", ran)
	}
	if repo, n, ok := PullOf(db.Watch{Vendor: "github", Target: "acme/app#418", Until: "landed"}); !ok || repo != "acme/app" || n != 418 {
		t.Fatalf("%q %d %v", repo, n, ok)
	}
}

// The repository of a remote is read from a github.com url in each form git writes one, and from nothing else; two names that differ
// only in case are one repository.
func TestARemotesRepositoryIsReadOnlyFromAGitHubURL(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/acme/app.git":         "acme/app",
		"https://github.com/acme/app":             "acme/app",
		"https://user@github.com/acme/app/":       "acme/app", // local-ref-ok: userinfo in a remote is the case
		"git@github.com:acme/app.git\n":           "acme/app",
		"ssh://git@github.com/acme/app":           "acme/app",
		"https://github.com/../..":                "",
		"https://github.com/acme/..":              "",
		"https://github.example.com/acme/app.git": "",
		"https://example.com/github.com/acme/app": "",
		"/srv/git/acme/app.git":                   "",
		"":                                        "",
	} {
		if got := RemoteRepo(url); got != want {
			t.Errorf("RemoteRepo(%q) is %q, want %q", url, got, want)
		}
	}
	if !SameRepo("Acme/App", "acme/app") || SameRepo("acme/app", "acme/apps") || SameRepo("", "") {
		t.Fatal("SameRepo")
	}
}

package team

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
)

func TestCreateMakesTheWorkspaceItsLeadAndItsRules(t *testing.T) {
	home := newHome(t)
	repo := checkout(t)
	key, err := NewRoster(home).Create("shop", repo)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := store.Shared(paths.DBFile(home))
	var folder, lead, reports string
	d.QueryRow("SELECT w.folder, a.name, a.reports_to FROM workspaces w JOIN agents a ON a.id = w.lead WHERE w.name = 'shop'").Scan(&folder, &lead, &reports)
	if folder != repo || lead != names.Lead || reports != "p1" {
		t.Fatalf("at %s, led by %s reporting to %s", folder, lead, reports)
	}
	if b, err := os.ReadFile(filepath.Join(paths.WorkspaceDir(home, key), "base.md")); err != nil || !strings.HasPrefix(string(b), "# base - shop") {
		t.Fatalf("base.md %q %v", b, err)
	}
}

func TestAFolderInACheckoutIsItsTop(t *testing.T) {
	home := newHome(t)
	repo := checkout(t)
	sub := filepath.Join(repo, "app", "src")
	os.MkdirAll(sub, 0o755)
	if _, err := NewRoster(home).Create("shop", sub); err != nil {
		t.Fatal(err)
	}
	d, _ := store.Shared(paths.DBFile(home))
	var folder string
	d.QueryRow("SELECT folder FROM workspaces WHERE name = 'shop'").Scan(&folder)
	if real(folder) != real(repo) {
		t.Fatalf("%s, want the checkout's top %s", folder, repo)
	}
}

func TestAFolderInNoCheckoutIsNotGitAndMakesNothing(t *testing.T) {
	home := newHome(t)
	_, err := NewRoster(home).Create("shop", t.TempDir())
	var ng *NotGit
	if !errors.As(err, &ng) || !IsRefusal(err) {
		t.Fatalf("%v", err)
	}
	d, _ := store.Shared(paths.DBFile(home))
	var n int
	d.QueryRow("SELECT COUNT(*) FROM workspaces").Scan(&n)
	if n != 0 {
		t.Fatalf("%d workspaces left behind", n)
	}
}

func TestAFolderSharedWithAnotherWorkspaceIsRefused(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	repo := checkout(t)
	if _, err := r.Create("shop", repo); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create("blog", repo); err == nil || !strings.Contains(err.Error(), "shares it") {
		t.Fatalf("the same checkout: %v", err)
	}
	inner := filepath.Join(repo, "vendor", "lib")
	os.MkdirAll(inner, 0o755)
	if out, err := runGit(inner, "init", "-q"); err != nil {
		t.Fatal(out)
	}
	if _, err := r.Create("blog", inner); err == nil || !strings.Contains(err.Error(), "shares it") {
		t.Fatalf("a checkout inside it: %v", err)
	}
	// moving a lead's folder onto another workspace's is refused the same way
	other := checkout(t)
	if _, err := r.Create("blog", other); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register("blog", RegisterOpts{Repo: repo}); err == nil || !strings.Contains(err.Error(), "shares it") {
		t.Fatalf("a lead moved onto shop's folder: %v", err)
	}
}

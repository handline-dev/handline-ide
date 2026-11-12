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
	"github.com/handline/handline/go/kit/yamlcompat"
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
	dir := paths.WorkspaceDir(home, key)
	if b, err := os.ReadFile(filepath.Join(dir, "base.md")); err != nil || string(b) != DefaultRules {
		t.Fatalf("base.md is the house rules: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".seeded")); err != nil {
		t.Fatal("seeded, so a team that empties its rules keeps them empty")
	}
	if _, err := NewRoster(home).Create("shop", repo); err == nil || !strings.Contains(err.Error(), "exists already") {
		t.Fatalf("a second Create of shop: %v", err)
	}
}

// A settings save sends the folder back as it is: that is no change, even where the folder is no checkout (a workspace made before the
// check), and an agent that is not the lead may send it.
func TestAnUnchangedFolderIsNoChange(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	register(t, r, "shop", "shop.ui")
	plain := t.TempDir()
	d, _ := store.Shared(paths.DBFile(home))
	d.Exec("UPDATE workspaces SET folder = ? WHERE name = 'shop'", plain)
	for _, who := range []string{"shop", "shop.ui"} {
		if err := r.SetKeys(who, []yamlcompat.Pair{{Key: "repo", Val: plain}, {Key: "model", Val: "opus"}}); err != nil {
			t.Fatalf("%s: %v", who, err)
		}
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

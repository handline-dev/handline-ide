package team

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/names"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/store"
	"github.com/handline/handline/go/kit/testfiles"
	"github.com/handline/handline/go/kit/yamlcompat"
)

func TestCreateMakesTheWorkspaceItsLeadAndItsRules(t *testing.T) {
	home := newHome(t)
	repo := testfiles.GitCheckout(t)
	key, err := NewRoster(home).Create("shop", repo)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := store.Shared(paths.DBFile(home))
	var folder, lead string
	d.QueryRow("SELECT w.folder, a.name FROM workspaces w JOIN agents a ON a.id = w.lead WHERE w.name = 'shop'").Scan(&folder, &lead)
	if folder != repo || lead != names.Lead {
		t.Fatalf("at %s, led by %s", folder, lead)
	}
	if in, err := db.CurrentInstruction(d, key, db.PartRules); err != nil || in == nil || in.Text != strings.TrimSpace(DefaultRules) || in.Actor != SeedActor {
		t.Fatalf("its rules are the house rules: %+v %v", in, err)
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
	register(t, r, "shop", "shop/ui")
	plain := t.TempDir()
	d, _ := store.Shared(paths.DBFile(home))
	d.Exec("UPDATE workspaces SET folder = ? WHERE name = 'shop'", plain)
	for _, who := range []string{"shop", "shop/ui"} {
		if err := r.SetKeys(who, []yamlcompat.Pair{{Key: "repo", Val: plain}, {Key: "model", Val: "opus"}}); err != nil {
			t.Fatalf("%s: %v", who, err)
		}
	}
}

func TestAFolderInACheckoutIsItsTop(t *testing.T) {
	home := newHome(t)
	repo := testfiles.GitCheckout(t)
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

func TestAPlainFolderIsAWorkspacesFolderAsItIs(t *testing.T) {
	home := newHome(t)
	plain := t.TempDir()
	if _, err := NewRoster(home).Create("notes", plain); err != nil {
		t.Fatal(err)
	}
	d, _ := store.Shared(paths.DBFile(home))
	var folder string
	d.QueryRow("SELECT folder FROM workspaces WHERE name = 'notes'").Scan(&folder)
	if folder != filepath.Clean(plain) || IsGit(folder) {
		t.Fatalf("%s, want the plain folder %s", folder, plain)
	}
}

func TestAFolderSharedWithAnotherWorkspaceIsRefused(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	repo := testfiles.GitCheckout(t)
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
	other := testfiles.GitCheckout(t)
	if _, err := r.Create("blog", other); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register("blog/lead", RegisterOpts{Repo: repo}); err == nil || !strings.Contains(err.Error(), "shares it") {
		t.Fatalf("a lead moved onto shop's folder: %v", err)
	}
}

// A workspace holds several folders, git or plain, its first the default: each agent works in one, and a folder an agent works in
// stays until it has another.
func TestAWorkspaceHoldsSeveralFoldersAndEachAgentWorksInOne(t *testing.T) {
	home := newHome(t)
	r := NewRoster(home)
	web, api, notes := testfiles.GitCheckout(t), testfiles.GitCheckout(t), t.TempDir()
	ws, err := r.Create("shop", web)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{api, notes, api} { // the same folder twice is one
		kept, _, err := Folder(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.AddFolder(ws, kept); err != nil {
			t.Fatal(err)
		}
	}
	_, id, _ := actor.Parse(ws)
	if fs, err := Folders(r.DB, id); err != nil || strings.Join(fs, ",") != strings.Join([]string{web, api, filepath.Clean(notes)}, ",") {
		t.Fatalf("the default first, then the added ones: %v %v", fs, err)
	}
	if _, err := r.Create("blog", testfiles.GitCheckout(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register("blog/lead", RegisterOpts{Repo: api}); err == nil || !strings.Contains(err.Error(), "shares it") {
		t.Fatalf("an added folder is the workspace's too: %v", err)
	}
	if _, err := r.Register("shop/server", RegisterOpts{Repo: api}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register("shop/ui", RegisterOpts{Repo: web}); err != nil {
		t.Fatal(err)
	}
	if r.RepoOf("shop/server") != api || r.RepoOf("shop/ui") != web || r.RepoOf("shop") != web {
		t.Fatalf("each in its folder: %q %q %q", r.RepoOf("shop/server"), r.RepoOf("shop/ui"), r.RepoOf("shop"))
	}
	if got := r.OtherFolders("shop/server"); strings.Join(got, ",") != web+","+filepath.Clean(notes) {
		t.Fatalf("it reads the others: %v", got)
	}
	var own sql.NullString
	r.DB.QueryRow("SELECT folder FROM agents WHERE name = 'ui'").Scan(&own)
	if own.Valid {
		t.Fatalf("an agent of the default keeps none of its own: %v", own)
	}
	if _, err := r.Register("shop/ui", RegisterOpts{Repo: testfiles.GitCheckout(t)}); err == nil || !strings.Contains(err.Error(), "not a folder of workspace shop") {
		t.Fatalf("a folder the workspace does not hold: %v", err)
	}
	if err := r.RemoveFolder(ws, api); err == nil || !strings.Contains(err.Error(), "1 of its agents work in") {
		t.Fatalf("a folder an agent works in stays: %v", err)
	}
	if err := r.RemoveFolder(ws, notes); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveFolder(ws, web); err == nil || !strings.Contains(err.Error(), "2 of its agents work in") {
		t.Fatalf("the default stays while its agents work there: %v", err)
	}
	if fs, _ := Folders(r.DB, id); strings.Join(fs, ",") != web+","+api {
		t.Fatalf("%v", fs)
	}
	if err := r.SetKeys("shop", []yamlcompat.Pair{{Key: "repo", Val: ""}}); err == nil || !strings.Contains(err.Error(), "has other folders") {
		t.Fatalf("a default is not cleared under the other folders: %v", err)
	}
	// the lead working in api sets a new default: it works there, and so does every agent of the default
	r.SetKeys("shop", []yamlcompat.Pair{{Key: "repo", Val: api}})
	other := testfiles.GitCheckout(t)
	if err := r.SetKeys("shop", []yamlcompat.Pair{{Key: "repo", Val: other}}); err != nil {
		t.Fatal(err)
	}
	if r.RepoOf("shop") != other || r.RepoOf("shop/ui") != other || r.RepoOf("shop/server") != api {
		t.Fatalf("%q %q %q", r.RepoOf("shop"), r.RepoOf("shop/ui"), r.RepoOf("shop/server"))
	}
}

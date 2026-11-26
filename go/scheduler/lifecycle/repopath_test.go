package lifecycle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/kit/paths"
)

func TestARepoIsExpandedToTheFolderItsLinksLeadTo(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir()) // a temp dir is itself behind a link on macOS
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if got := ExpandRepo(link); got != real {
		t.Errorf("a link: %q, want %q", got, real)
	}
	// What does not exist yet keeps its name under the folder its parent leads to.
	if got, want := ExpandRepo(filepath.Join(link, "not", "there")), filepath.Join(real, "not", "there"); got != want {
		t.Errorf("a path whose tail does not exist: %q, want %q", got, want)
	}
	if got := ExpandRepo(""); got != "" {
		t.Errorf("no folder: %q", got)
	}
}

func TestATildeIsTheUsersHome(t *testing.T) {
	home, err := filepath.EvalSymlinks(paths.UserHome()) // a test's own home (kit/paths), never the person's
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ExpandRepo("~/code/x"), filepath.Join(home, "code", "x"); got != want {
		t.Errorf("~/code/x: %q, want %q", got, want)
	}
	if got := ExpandRepo("~"); got != home {
		t.Errorf("~: %q, want %q", got, home)
	}
	if got, want := ExpandRepo("/~/x"), "/~/x"; got != want {
		t.Errorf("a tilde that is not first is a name: %q, want %q", got, want)
	}
}

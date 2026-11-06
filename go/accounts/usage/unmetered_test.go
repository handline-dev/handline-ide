package usage

import (
	"os"
	"path/filepath"
	"testing"

	_ "github.com/handline/handline/go/agentctl/agent/vendors" // opencode registers: a vendor that reports no usage
	"github.com/handline/handline/go/kit/paths"
)

// An unpinned agent never lands on an unmetered account: the pick and every room test leave it out with UnmeteredWhy, which only a pin
// gets past (lifecycle.Launcher.PinOf).
func TestAnUnmeteredAccountIsNeverAPlaceForAnUnpinnedAgent(t *testing.T) {
	home := t.TempDir()
	dir := paths.AccountDir(home, "oc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vendor"), []byte("opencode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Env{Home: home}
	if why := e.NotFor("oc", "opencode"); why != UnmeteredWhy {
		t.Fatalf("%q", why)
	}
	if st := e.Read("oc"); !st.Unmetered {
		t.Fatalf("%+v", st)
	}
	if ok, why := e.Read("oc").Assignable(0); ok || why != UnmeteredWhy {
		t.Fatalf("%v %q", ok, why)
	}
}

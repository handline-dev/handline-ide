package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/team/teamtest"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/taskboard/supervisor"
)

func TestADeliveryNamesWhoWroteItAndNeverTheSessionsOwnPerson(t *testing.T) {
	t.Setenv("HANDLINE_TEST_ROOT", t.TempDir())
	for _, k := range []string{"HANDLINE_AGENT"} {
		t.Setenv(k, "")
	}
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	teamtest.Seed(t, home, "free", "asks", "other")
	d, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	op := func(from string) string {
		rid, err := db.AddRequest(d, db.NewRequest{Requester: from, Target: "free", Body: "x", Kind: "ask"})
		if err != nil {
			t.Fatal(err)
		}
		return supervisor.RequestOp(rid)
	}
	from := peerFrom(d)
	for _, c := range []struct{ op, name string }{
		{op("asks"), "asks"},
		{op(db.Human), "founder"},
		{supervisor.DaemonOp, ""},
		{"notice:1", ""},
	} {
		if got := from("free", c.op); got.Name != c.name {
			t.Errorf("%s: %+v, want name %q", c.op, got, c.name)
		}
	}
}

package contracttest_test

import (
	"encoding/json"
	"testing"

	"github.com/handline/handline/go/agentctl/dialogs"
	"github.com/handline/handline/go/internal/contracttest"
)

// The dialogs table (agentctl) is the contract's x-dialogs: same dialogs, same class, same kind.
func TestTheTableIsTheContractsTable(t *testing.T) {
	raw := contracttest.Spec(t)
	var spec struct {
		X struct {
			Dialogs map[string]struct {
				Class string `json:"class"`
				Kind  string `json:"kind"`
			} `json:"dialogs"`
		} `json:"x-dialogs"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	if len(spec.X.Dialogs) != len(dialogs.Table) {
		t.Fatalf("the contract names %d dialogs, the table %d", len(spec.X.Dialogs), len(dialogs.Table))
	}
	for id, c := range spec.X.Dialogs {
		if got, ok := dialogs.Table[id]; !ok || got.Class != c.Class || got.Kind != c.Kind {
			t.Errorf("%s: contract %+v, table %+v", id, c, got)
		}
	}
}

package claudecli

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"github.com/handline/handline/go/accounts/livetest"
)

// TestLiveGetUsage asks a real CLI for an account's usage with the get_usage control request: no model turn, no session spoken in. It
// is the live check for the usage port. HANDLINE_LIVE=1 go test -run TestLiveGetUsage -v ./accounts/claudecli
func TestLiveGetUsage(t *testing.T) {
	cfg := livetest.Pick(t).ConfigDir
	path, err := exec.LookPath("claude")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), Launch{Path: path, Cwd: t.TempDir(), ConfigDir: cfg}, Handlers{}, 60*time.Second)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	r, err := s.GetUsage(c)
	if err != nil {
		t.Fatalf("get_usage: %v", err)
	}
	b, _ := json.MarshalIndent(r, "", " ")
	t.Logf("get_usage answered:\n%s", b)
	if avail, _ := r["rate_limits_available"].(bool); !avail {
		t.Error("rate_limits_available is false: the plan limits did not come back")
	}
	limits, _ := r["rate_limits"].(map[string]any)
	for _, w := range []string{"five_hour", "seven_day"} {
		if m, _ := limits[w].(map[string]any); m == nil || m["utilization"] == nil || m["resets_at"] == nil {
			t.Errorf("%s has no utilization and reset: %v", w, limits[w])
		}
	}
	// A second ask must not take a turn or fail: the poll asks once per account per cycle.
	if _, err := s.GetUsage(c); err != nil {
		t.Errorf("second get_usage: %v", err)
	}
}

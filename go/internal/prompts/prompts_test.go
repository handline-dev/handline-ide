package prompts

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/handline/handline/go/internal/reads"
	"github.com/handline/handline/go/kit/storetest"
)

// refreshInTests is a waiting call's refresh in these tests, so "two refreshes passed and it stayed open" takes a fraction of a second.
const refreshInTests = 100 * time.Millisecond

func testEnv(t *testing.T) *Env {
	t.Helper()
	home := t.TempDir()
	db, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := New(db, home, &reads.Env{DB: db, Home: home, Started: time.Now()})
	e.Refresh = refreshInTests
	return e
}

// TestMain sets the process environment once (storetest.Main), so the tests can run in parallel.
func TestMain(m *testing.M) {
	storetest.Main(m, []string{"HANDLINE_AGENT"}, nil)
}

func openPrompt(t *testing.T, e *Env, agent string) map[string]any {
	t.Helper()
	p, err := e.Reads.OpenPrompt(agent)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	var m map[string]any
	json.Unmarshal(b, &m)
	return m
}

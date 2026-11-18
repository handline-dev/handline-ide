package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/kit/gitrun"
	"github.com/handline/handline/go/kit/protected"
	"github.com/handline/handline/go/kit/storetest"
	"github.com/handline/handline/go/scheduler/worktreepool"
)

// deskRig is a repo with four free desks, given back an hour apart: desk-1 the oldest.
func deskRig(t *testing.T) (*deskPrune, string, []string, *[]string) {
	t.Helper()
	t.Setenv("HANDLINE_AGENT", "")
	home := t.TempDir()
	d, err := storetest.OpenAt(t, filepath.Join(home, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	repo := filepath.Join(t.TempDir(), "proj")
	os.MkdirAll(repo, 0o755)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "first"}} {
		if r := gitrun.Run(repo, args...); r.RC != 0 {
			t.Fatal(r.Stderr)
		}
	}
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local)
	pool := &worktreepool.Pool{DB: d, Base: func(string, string) (string, string, error) { return "main", "", nil },
		Branch: func(agent string) string { return agent + "-work" }, Now: func() time.Time { return at }}
	var desks []string
	for i := 1; i <= 4; i++ {
		tk, why, err := pool.Take(fmt.Sprintf("a%d", i), repo)
		if err != nil || why != "" {
			t.Fatal(err, why)
		}
		desks = append(desks, tk.Path)
	}
	for i := 1; i <= 4; i++ {
		at = at.Add(time.Hour)
		if _, err := pool.GiveBack(fmt.Sprintf("a%d", i), 0); err != nil {
			t.Fatal(err)
		}
	}
	var logged []string
	return &deskPrune{pool: pool, q: d, home: home, log: func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) }}, repo, desks, &logged
}

func setConfig(t *testing.T, home, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Once a night from 3:00, each project's free desks beyond what it keeps that went unused for three days go, oldest first; a Manual
// number is kept as set, and a second run the same night removes nothing.
func TestTheNightlyPruneRunsOnceANightFromThree(t *testing.T) {
	p, repo, desks, _ := deskRig(t)
	if err := worktreepool.PutKept(protected.New(p.home), repo, worktreepool.Kept{Mode: worktreepool.Manual, Value: 2}); err != nil {
		t.Fatal(err)
	}
	night := time.Date(2026, 11, 18, 2, 30, 0, 0, time.Local).Unix()
	if err := p.Run(night); err != nil {
		t.Fatal(err)
	}
	if left, _ := worktreepool.List(p.q, repo); len(left) != 4 {
		t.Fatalf("before 3:00 nothing goes: %d", len(left))
	}
	if err := p.Run(night + 3600); err != nil {
		t.Fatal(err)
	}
	left, _ := worktreepool.List(p.q, repo)
	if len(left) != 2 || left[0].Path != desks[2] || left[1].Path != desks[3] {
		t.Fatalf("the two oldest go: %+v", left)
	}
	if day, _ := db.Cursor(p.q, pruneCursor); day != 20261118 {
		t.Fatalf("cursor %d", day)
	}
	// Auto with no week of load recorded keeps them all, the next night too
	worktreepool.PutKept(protected.New(p.home), repo, worktreepool.Kept{Mode: worktreepool.Auto})
	p.Run(night + 86400 + 3600)
	if left, _ := worktreepool.List(p.q, repo); len(left) != 2 {
		t.Fatalf("auto without a week: %d", len(left))
	}
}

// Under the founder's disk target the free desks go oldest first, whatever their age or the kept number, until free disk is back over
// it; off, nothing does.
func TestTheDiskTargetRemovesFreeDesksUntilTheDiskIsBackOverIt(t *testing.T) {
	p, repo, desks, logged := deskRig(t)
	freeGB := int64(3)
	p.free = func(string) (int64, int64, error) {
		left, _ := worktreepool.List(p.q, repo)
		return (freeGB + int64(4-len(left))) << 30, 100 << 30, nil // each removed desk frees 1 GB
	}
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local).Unix()
	if err := p.Run(now); err != nil {
		t.Fatal(err)
	}
	if left, _ := worktreepool.List(p.q, repo); len(left) != 4 {
		t.Fatal("off: nothing goes")
	}
	setConfig(t, p.home, "disk_keep_free_gb: 5\n")
	if err := p.Run(now + diskEvery); err != nil {
		t.Fatal(err)
	}
	if left, _ := worktreepool.List(p.q, repo); len(left) != 3 || left[0].Path != desks[1] {
		t.Fatalf("a desk not yet measured counts as the whole shortfall: one goes per check: %+v", left)
	}
	for _, d := range desks {
		db.SetMeta(p.q, "worktree_size:"+d, fmt.Sprint(1<<30))
	}
	freeGB = 2 // the disk filled meanwhile: 2 GB short
	if err := p.Run(now + 2*diskEvery); err != nil {
		t.Fatal(err)
	}
	left, _ := worktreepool.List(p.q, repo)
	if len(left) != 1 || left[0].Path != desks[3] {
		t.Fatalf("measured desks go until the disk is back over it: %+v", left)
	}
	freeGB = 0 // the disk fills again: the rest go, then the shortfall is told once
	p.Run(now + 3*diskEvery)
	p.Run(now + 4*diskEvery)
	if left, _ := worktreepool.List(p.q, repo); len(left) != 0 {
		t.Fatalf("all free desks go: %+v", left)
	}
	short := 0
	for _, l := range *logged {
		if strings.Contains(l, "no free desk can be removed") {
			short++
		}
	}
	if len(*logged) != 5 || short != 1 {
		t.Fatalf("each removal logged with its reason, the shortfall once: %q", *logged)
	}
}

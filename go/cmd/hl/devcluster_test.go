package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The cluster's homes never sit in or around the live home, through a link or before the folder exists.
func TestClusterHomesNeverOverlapTheLiveHome(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(live, link); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		a    string
		want bool
	}{
		{live, true},
		{filepath.Join(live, "cluster"), true},
		{dir, true},
		{filepath.Join(link, "not-yet"), true},
		{filepath.Join(dir, "elsewhere"), false},
		{live + "-dev", false},
	} {
		if got := overlaps(c.a, live); got != c.want {
			t.Errorf("%s: overlaps %v, want %v", c.a, got, c.want)
		}
	}
}

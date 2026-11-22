package paths

import (
	"os"
	"path/filepath"
)

// UseCarriedTools puts the folder this program runs from last on PATH when it holds a tmux: the installed app carries one beside handlined
// and hl, so a Mac with no tmux has one. A tmux already on PATH comes first, so a running team's tmux server keeps its version, and every
// place that runs tmux by name (the terminal backend, the control link, the cluster's cleanup, the missing-tools check) finds the same one.
func UseCarriedTools() {
	if exe, err := os.Executable(); err == nil {
		os.Setenv("PATH", carriedPath(exe, os.Getenv("PATH")))
	}
}

func carriedPath(exe, path string) string {
	dir := filepath.Dir(exe)
	if fi, err := os.Stat(filepath.Join(dir, "tmux")); err != nil || fi.IsDir() {
		return path
	}
	for _, d := range filepath.SplitList(path) {
		if d == dir {
			return path
		}
	}
	if path == "" {
		return dir
	}
	return path + string(os.PathListSeparator) + dir
}

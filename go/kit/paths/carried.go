package paths

import (
	"os"
	"path/filepath"
)

// UseCarriedTools puts the folder this program runs from last on PATH when it holds a tmux (the installed app carries one), so a tmux already
// on PATH comes first and a Mac with none still has one.
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

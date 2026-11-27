package reads

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
)

// runIndex is each home's run proposals by the task they were asked on, parsed again only when ProposalSignature changes, so a
// task.get reads no proposal file. Keyed by home, as each caller builds its own Env.
var runIndex sync.Map // home -> *taskRuns

type taskRuns struct {
	mu     sync.Mutex
	sig    string
	byTask map[int64][]proposal
}

// ProposalSignature is whether the store changed: each proposal file by name, inode, size and modification time. A write renames a
// new file in, so its inode moves even within one clock tick.
func ProposalSignature(home string) string {
	files, _ := filepath.Glob(filepath.Join(home, "proposals", "*.yaml"))
	slices.Sort(files)
	var b strings.Builder
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		var ino uint64
		if s, ok := st.Sys().(*syscall.Stat_t); ok {
			ino = s.Ino
		}
		fmt.Fprintf(&b, "%s=%d,%d,%d;", filepath.Base(f), ino, st.Size(), st.ModTime().UnixNano())
	}
	return b.String()
}

// taskRuns is the run proposals asked while their agent was doing task tid, as last parsed.
func (e *Env) taskRuns(tid int64) []proposal {
	v, _ := runIndex.LoadOrStore(e.Home, &taskRuns{})
	c := v.(*taskRuns)
	c.mu.Lock()
	defer c.mu.Unlock()
	if sig := ProposalSignature(e.Home); c.byTask == nil || sig != c.sig {
		c.byTask, c.sig = map[int64][]proposal{}, sig
		for _, p := range e.proposals() {
			if t, _ := p.get("task").(int64); t > 0 && p.text("kind") == "run" {
				c.byTask[t] = append(c.byTask[t], p)
			}
		}
	}
	return c.byTask[tid]
}

package reads

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// The memory budget (docs/memory-design.md): the index MEMORY.md holds at most memoryBudget bytes, and a topic file not read or
// written for memoryStaleDays is unused.
const (
	memoryBudget    = 2048
	memoryStaleDays = 28
)

// memoryStatus is memory.status: each agent's memory index against its budget, and the topic files it has not used lately; one agent
// when the call names it.
func (e *Env) memoryStatus(c *rpc.Conn, params map[string]any) (any, error) {
	agents := e.listAgents()
	if _, has := params["agent"]; has {
		name, err := e.named(params)
		if err != nil {
			return nil, err
		}
		agents = []string{name}
	}
	out := []any{}
	for _, a := range agents {
		out = append(out, memoryOf(e.Home, a, time.Now()))
	}
	return map[string]any{"agents": out}, nil
}

// memoryOf is one agent's row: an agent with no memory folder has nothing in it.
func memoryOf(home, agent string, now time.Time) map[string]any {
	dir := filepath.Join(paths.AgentDir(home, agent), "memory")
	index, _ := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	entries := 0
	for _, line := range strings.Split(string(index), "\n") {
		if strings.HasPrefix(line, "- [") {
			entries++
		}
	}
	used := map[string]any{} // the hook's memory-use.json: {path relative to the folder: unix seconds}
	if b, err := os.ReadFile(filepath.Join(paths.AgentDir(home, agent), "memory-use.json")); err == nil {
		_ = json.Unmarshal(b, &used)
	}
	unused := []string{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "MEMORY.md" {
			return nil
		}
		last, ok := used[filepath.ToSlash(rel)].(float64)
		if !ok || last == 0 { // a file the hook never saw: its mtime stands in
			info, err := d.Info()
			if err != nil {
				return nil
			}
			last = float64(info.ModTime().Unix())
		}
		if float64(now.Unix())-last > memoryStaleDays*86400 {
			unused = append(unused, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(unused)
	return map[string]any{"agent": agent, "bytes": len(index), "budget": memoryBudget, "entries": entries, "unused": unused}
}

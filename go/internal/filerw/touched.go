package filerw

import (
	"path/filepath"
	"sync"
	"time"
)

// touchKeep is how long an agent's edit of a file names it as the one who changed it (DocUpdate.by).
const touchKeep = 10 * time.Second

// editTools are the tools whose PostToolUse names a file the agent changed.
var editTools = map[string]bool{"Write": true, "Edit": true, "MultiEdit": true, "NotebookEdit": true}

type touch struct {
	agent string
	at    time.Time
}

// touches are the files agents' edit tools changed lately, by real path.
type touches struct {
	mu  sync.Mutex
	all map[string]touch
}

// realPath is p with its links resolved, so a path an agent's tool gave and a worktree path compare; p as it is when it cannot be.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// Touched notes the file an agent's Write, Edit, MultiEdit or NotebookEdit changed, from its PostToolUse hook's own object.
func (e *Env) Touched(agent string, raw map[string]any) {
	tool, _ := raw["tool_name"].(string)
	input, _ := raw["tool_input"].(map[string]any)
	path, _ := input["file_path"].(string)
	if path == "" {
		path, _ = input["notebook_path"].(string)
	}
	if !editTools[tool] { // a relative path is kept as it is, and matches no doc's real path
		return
	}
	real, now := realPath(path), e.now()
	e.touches.mu.Lock()
	defer e.touches.mu.Unlock()
	if e.touches.all == nil {
		e.touches.all = map[string]touch{}
	}
	for p, t := range e.touches.all {
		if now.Sub(t.at) > touchKeep {
			delete(e.touches.all, p)
		}
	}
	e.touches.all[real] = touch{agent, now}
}

// touchedBy is the agent whose edit tool changed the file at real path abs within touchKeep, or "". use spends it: the edit names one
// disk update, not a later change by someone else.
func (e *Env) touchedBy(abs string, use bool) string {
	e.touches.mu.Lock()
	defer e.touches.mu.Unlock()
	t, ok := e.touches.all[abs]
	if !ok || e.now().Sub(t.at) > touchKeep {
		return ""
	}
	if use {
		delete(e.touches.all, abs)
	}
	return t.agent
}

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

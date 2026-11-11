package reads

import (
	"os"
	"path/filepath"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/protocol/rpc"
)

// fileInfo is a team.get file entry: its path and when it was last written, null when it cannot be read.
func fileInfo(path string) map[string]any {
	var updated any
	if st, err := os.Stat(path); err == nil {
		updated = ts(st.ModTime().Unix())
	}
	return map[string]any{"path": path, "updated": updated}
}

// teamGet is team.get: every agent with its parent and files. Rows that cannot be read are no parents.
func (e *Env) teamGet(c *rpc.Conn, params map[string]any) (any, error) {
	table, err := team.LoadTeam(e.Home)
	if err != nil {
		table = nil
	}
	agents := []any{}
	for _, name := range e.listAgents() {
		var parent any
		if table != nil {
			if p := table.SupervisorOf(name); p != "" {
				parent = p
			}
		}
		dir := paths.AgentDir(e.Home, name)
		agents = append(agents, map[string]any{"name": name, "role": e.roleLine(name), "parent": parent,
			"role_file": fileInfo(filepath.Join(dir, "ROLE.md")), "base_file": fileInfo(filepath.Join(dir, "base.md"))})
	}
	// Routing follows workspaces now, with no edges: the field stays empty until the contract drops it (base-v2 1e).
	return map[string]any{"agents": agents, "edges": []any{}}, nil
}

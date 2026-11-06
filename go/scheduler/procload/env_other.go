//go:build !darwin && !linux

package procload

// agentOf cannot read another process's environment here: every process falls back to the pane's tree.
func agentOf(pid int) (agent string, ok bool) { return "", false }

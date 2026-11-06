package procload

import (
	"fmt"
	"os"
)

// agentOf is the HANDLINE_AGENT in a process's environment, read from /proc/<pid>/environ.
func agentOf(pid int) (agent string, ok bool) {
	buf, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
	if err != nil {
		return "", false
	}
	return envFrom(buf, agentEnv), true
}

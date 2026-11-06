package procload

import "golang.org/x/sys/unix"

// agentOf is the HANDLINE_AGENT in a process's environment, read with sysctl KERN_PROCARGS2 (same user, no root); ok false when the
// process cannot be read (another user's, or gone), "" with ok true when it has no agent.
func agentOf(pid int) (agent string, ok bool) {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", false
	}
	return envValue(buf, agentEnv), true
}

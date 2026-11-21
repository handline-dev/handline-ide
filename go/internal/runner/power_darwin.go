package runner

import (
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
)

// bootID is when this machine last started, which another boot changes.
func bootID() string {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return ""
	}
	return fmt.Sprint(tv.Sec)
}

// machineOnBattery is whether this Mac draws from its battery now (pmset -g ps).
func machineOnBattery() bool {
	out, err := exec.Command("pmset", "-g", "ps").Output()
	return err == nil && strings.Contains(string(out), "'Battery Power'")
}

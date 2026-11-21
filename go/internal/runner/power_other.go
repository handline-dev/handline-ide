//go:build !darwin

package runner

import (
	"os"
	"strings"
)

// bootID is this machine's boot id, which another boot changes.
func bootID() string {
	b, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(b))
}

// machineOnBattery is false: only a Mac's power source is read.
func machineOnBattery() bool { return false }

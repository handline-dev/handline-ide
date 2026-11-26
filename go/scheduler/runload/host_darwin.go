package runload

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// readHost is the kernel's memory pressure level (1 normal, 2 warn, 4 critical) and how many pages it has compressed since boot.
// Swap-outs are not read: with the compressor busy and the level at warn they stood at none, so they miss the state that matters.
func readHost() (pressure int, compressions int64, ok bool) {
	level, err := unix.SysctlUint32("kern.memorystatus_vm_pressure_level")
	if err != nil {
		return 0, 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "vm_stat").Output()
	if err != nil {
		return int(level), 0, false
	}
	return int(level), compressed(string(out)), true
}

// compressed is the Compressions line of vm_stat, 0 when it has none.
func compressed(vmStat string) int64 {
	for _, line := range strings.Split(vmStat, "\n") {
		if name, val, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(name) == "Compressions" {
			n, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(val), "."), 10, 64)
			return n
		}
	}
	return 0
}

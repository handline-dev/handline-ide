//go:build darwin || linux

package runload

import (
	"errors"
	"syscall"
)

// LowNice is the niceness a heavy run's processes are given while the machine is slow: they still share what is free between
// them, and give way to everything else. Only the owner of a process may raise its niceness, and nobody but root lowers it again,
// so it lasts as long as the process.
const LowNice = 10

// lower gives the process pid that began at start LowNice, unless it is already that nice or nicer. A pid that now names another
// process (one that began at another time) is left alone, and a process that has ended is no error.
func lower(pid int, start int64) error {
	if began, ok := startOf(pid); ok && (began < start-1 || began > start+1) {
		return nil
	}
	if now, err := syscall.Getpriority(syscall.PRIO_PROCESS, pid); err == nil && nicer(now) {
		return nil
	}
	if err := syscall.Setpriority(syscall.PRIO_PROCESS, pid, LowNice); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

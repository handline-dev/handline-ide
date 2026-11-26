// Package lock is the exclusive holds, each one flock: one writer per agent terminal, and one daemon per installation
// (handline/infra/lock.py). The kernel releases an flock when its holder dies, so neither hold survives the process that took
// it. Neither nests (each acquire opens a fresh file description, so a second acquire inside the first fails) and neither
// waits: a caller that cannot have the hold leaves the work to the one that has it.
package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// The two kinds of hold. The suffix keeps them apart on disk, so an agent named daemon cannot take the daemon's lock by being typed at.
const (
	Terminal  = "terminal"
	Singleton = "lock"
	pidWidth  = 16
)

// Hold is a lock held until Release; the zero value holds nothing.
type Hold struct{ f *os.File }

// Release lets the hold go.
func (h *Hold) Release() {
	if h != nil && h.f != nil {
		h.f.Close()
		h.f = nil
	}
}

// take opens the lock file and tries the exclusive lock without waiting. nil when someone else holds it, or when it cannot even
// be attempted: without a lock there is no promise of one of us, and the alternative to promising it is two.
func take(home, name, kind string, recordPID bool) *Hold {
	dir := filepath.Join(home, "locks")
	if os.MkdirAll(dir, 0o755) != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, name+"."+kind), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil
	}
	if recordPID {
		// Over the old bytes, at a fixed width and without truncating, so a reader never sees an empty file where a name was.
		f.WriteAt([]byte(fmt.Sprintf("%-*d\n", pidWidth, os.Getpid())), 0)
	}
	return &Hold{f}
}

// TakeTerminal is the right to type at agent; nil immediately when someone else holds it.
func TakeTerminal(home, agent string) *Hold { return take(home, agent, Terminal, false) }

// TakeSingleton is the right to be the only name running against this home; the holder's pid is written into the file.
func TakeSingleton(home, name string) *Hold { return take(home, name, Singleton, true) }

// Alive reports whether process pid exists: kill(pid, 0) succeeds, or fails with EPERM (it exists and belongs to another user).
// A pid of 0 or below names no one process, so it is not alive.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

//go:build darwin || linux

package runload

import (
	"os/exec"
	"syscall"
	"testing"
)

// A process of ours is given LowNice, once: one already that nice is left as it is.
func TestLowerGivesAProcessLowNice(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("no sleep to start:", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	pid := cmd.Process.Pid
	before, err := syscall.Getpriority(syscall.PRIO_PROCESS, pid)
	if err != nil {
		t.Fatal(err)
	}
	if nicer(before) {
		t.Skipf("the test itself runs at a low priority (%d)", before)
	}
	began, known := startOf(pid)
	if known {
		// A pid that named another process when it was sampled is left alone.
		if err := lower(pid, began-3600); err != nil {
			t.Fatal(err)
		}
		if got, _ := syscall.Getpriority(syscall.PRIO_PROCESS, pid); nicer(got) {
			t.Fatalf("a process that began at another time was lowered: %d", got)
		}
	}
	if err := lower(pid, began); err != nil {
		t.Fatal(err)
	}
	after, err := syscall.Getpriority(syscall.PRIO_PROCESS, pid)
	if err != nil || !nicer(after) {
		t.Fatalf("after lowering: %d %v", after, err)
	}
	if err := lower(pid, began); err != nil {
		t.Fatalf("lowering it again: %v", err)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if err := lower(pid, began); err != nil {
		t.Fatalf("a process that has ended is no error: %v", err)
	}
}

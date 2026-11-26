package runload

import "golang.org/x/sys/unix"

// nicer is whether a getpriority answer is LowNice or more: the niceness itself.
func nicer(got int) bool { return got >= LowNice }

// startOf is when the process pid began, epoch seconds; ok false when it cannot be read (it has ended).
func startOf(pid int) (int64, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(kp.Proc.P_pid) != pid {
		return 0, false
	}
	return kp.Proc.P_starttime.Sec, true
}

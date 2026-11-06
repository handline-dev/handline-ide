package daemon

import (
	"fmt"
	"io"
	"runtime"
	"time"
)

// StopLimit is how long a stop may take to finish Close before it is cut off. A step in Close that legitimately runs longer is cut off (a
// heal step's length is not measured), and a signal stop then exits 1.
const StopLimit = 20 * time.Second

// Stop ends the daemon with Close. When Close has not finished after limit, every goroutine's stack goes to w (daemon.log for an
// app-owned daemon) and the stop stops waiting: after a signal (signaled) it calls exit(1); a dev-mode restart goes on to its exec, which
// gives up the lock and the database with the process.
func (d *Daemon) Stop(limit time.Duration, w io.Writer, signaled bool, exit func(int)) {
	done := make(chan struct{})
	go func() { d.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(limit):
		buf := make([]byte, 1<<20)
		fmt.Fprintf(w, "handlined: stop has not finished after %s; goroutines:\n%s\n", limit, buf[:runtime.Stack(buf, true)])
		if signaled {
			exit(1)
		}
	}
}

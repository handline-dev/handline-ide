package daemon

import (
	"net"
	"os"
	"testing"
	"time"
)

func TestAnInheritedConnectionWaitsForItsFirstByteBeforeTheLimitApplies(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c := &inherited{Conn: a}
	c.SetReadDeadline(time.Now().Add(-time.Second))
	go b.Write([]byte("x"))
	if n, err := c.Read(make([]byte, 1)); n != 1 || err != nil {
		t.Fatalf("before its first byte a read waits past the deadline: got %d, %v", n, err)
	}
	c.SetReadDeadline(time.Now().Add(-time.Second))
	if _, err := c.Read(make([]byte, 1)); !os.IsTimeout(err) {
		t.Fatalf("after its first byte the deadline holds: got %v", err)
	}
}

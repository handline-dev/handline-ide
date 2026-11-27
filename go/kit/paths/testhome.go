package paths

import (
	"os"
	"testing"
)

// A test binary started from an agent's shell inherits HANDLINE_HOME at the live home, and a test that sets none would open the live
// database (and migrate it under the running daemon), or hand that home to a process it starts. It is dropped before any test runs:
// a test gets a home only from its own Setenv or fixture, else the stand-in under UserHome.
func init() {
	if testing.Testing() {
		dropLiveHome()
	}
}

// dropLiveHome unsets HANDLINE_HOME when it names the live home.
func dropLiveHome() {
	if h, ok := Getenv("HOME"); ok && h != "" && IsLive(h) {
		os.Unsetenv("HANDLINE_HOME")
	}
}

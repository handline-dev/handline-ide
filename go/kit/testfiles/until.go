package testfiles

import (
	"testing"
	"time"
)

// Until asks ok at each interval every until it holds, failing the test when within has passed: a bounded wait on the state itself.
func Until(t testing.TB, what string, within, every time.Duration, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(within); !ok(); time.Sleep(every) {
		if time.Now().After(end) {
			t.Fatalf("no %s within %s", what, within)
		}
	}
}

package agent

import (
	"os/user"
	"testing"
)

// Without USER, the name is the OS user's.
func TestUserNameFallsBackToTheOSUser(t *testing.T) {
	t.Setenv("USER", "")
	u, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	if got := UserName(); got == "" || got != u.Username {
		t.Fatalf("UserName() = %q, want %q", got, u.Username)
	}
	t.Setenv("USER", "someone")
	if got := UserName(); got != "someone" {
		t.Fatalf("UserName() = %q with USER set", got)
	}
}

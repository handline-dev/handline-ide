package daemon

import (
	"os/exec"
	"os/user"
	"strings"
)

// ownerName is the display name the owner gets at first start: the Mac account's full name, else its login name.
func ownerName() string {
	if out, err := exec.Command("id", "-F").Output(); err == nil {
		if n := strings.TrimSpace(string(out)); n != "" {
			return n
		}
	}
	if u, err := user.Current(); err == nil {
		if n := strings.TrimSpace(u.Name); n != "" {
			return n
		}
		return u.Username
	}
	return "Owner"
}

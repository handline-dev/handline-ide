package sysres

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var machineName = sync.OnceValue(func() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("scutil", "--get", "ComputerName").Output(); err == nil {
			if n := strings.TrimSpace(string(out)); n != "" {
				return n
			}
		}
	}
	h, _ := os.Hostname()
	return strings.SplitN(h, ".", 2)[0]
})

// MachineName is this machine's name as its owner set it (the computer name on macOS), else its host name; read once.
func MachineName() string { return machineName() }

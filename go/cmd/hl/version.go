package main

import (
	"cmp"
	"errors"
	"fmt"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/version"
)

// versionCmd prints the commit hl was built from and, when the daemon answers, the daemon's: after an update the two match.
func versionCmd(c *hlclient.Client, args []string) (int, bool) {
	if len(args) > 0 {
		return 0, false
	}
	fmt.Printf("hl %s\n", version.Sha)
	got, err := c.Call("hello", nil)
	var down *hlclient.DaemonDown
	switch {
	case errors.As(err, &down):
		fmt.Println("handlined not running")
	case err != nil:
		fmt.Printf("handlined unknown: %v\n", err)
		return 1, true
	default:
		daemon, _ := got["daemon"].(string)
		fmt.Printf("handlined %s\n", cmp.Or(daemon, "unknown"))
	}
	return 0, true
}

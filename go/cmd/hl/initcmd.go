package main

import (
	"fmt"
	"os"

	"github.com/handline/handline/go/data/homeinit"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/paths"
)

// initCmd is `hl init` (setup.cmd_init): the home set up by hand, which the daemon also does at start. A home already set up is left alone.
func initCmd(c *hlclient.Client, args []string) (int, bool) {
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "usage: hl init")
		return 2, true
	}
	say := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	if err := homeinit.Setup(c.Home, paths.HlExe(), say); err != nil {
		fmt.Fprintln(os.Stderr, "hl init:", err)
		return 1, true
	}
	fmt.Printf("initialized %s\n", c.Home)
	fmt.Println("next: add an account in the app's Accounts page - it registers the account and logs it in")
	return 0, true
}

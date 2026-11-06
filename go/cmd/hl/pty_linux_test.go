package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openPty is a pseudo-terminal: the master to hold open and the slave to give a child as its stdin.
func openPty() (master, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := int(master.Fd())
	var n int
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err == nil {
		n, err = unix.IoctlGetInt(fd, unix.TIOCGPTN)
	}
	if err == nil {
		slave, err = os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	}
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}

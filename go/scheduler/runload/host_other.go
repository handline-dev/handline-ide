//go:build !darwin

package runload

// readHost reads nothing where the kernel does not say it this way.
func readHost() (pressure int, compressions int64, ok bool) { return 0, 0, false }

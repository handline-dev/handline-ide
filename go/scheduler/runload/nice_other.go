//go:build !darwin && !linux

package runload

// LowNice is the niceness a heavy run's processes are given while the machine is slow.
const LowNice = 10

func lower(int, int64) error { return nil }

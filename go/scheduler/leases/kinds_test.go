package leases

// The shared runs the tests ask for, all This Mac's, named so they are asked in the order build, sim, web.
const (
	Build = "mac:build"
	Sim   = "mac:sim"
	Web   = "mac:web"
)

// fixed is Shared with build, sim and web each n at once.
func fixed(n int) func() []Shared {
	return func() []Shared { return []Shared{{Build, n}, {Sim, n}, {Web, n}} }
}

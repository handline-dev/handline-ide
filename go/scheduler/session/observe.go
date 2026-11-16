package session

// Observed is one agent as the outside world reports it. A nil pointer means we could not look, which is not the same as no: acting
// on "I could not ask the terminal host" as though it were "the terminal is gone" would tear down a healthy team the first time it
// was slow to start.
type Observed struct {
	TerminalAlive *bool
	SessionPID    *int
	SessionSeen   bool // whether the process table could be read
}

// Process is one row of the process table.
type Process struct {
	PID     int
	Command string
}

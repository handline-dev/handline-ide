package agent

// Chat is an agent CLI run headless: lines in on stdin, frames out on stdout, under a holder. Nil from ChatOf is a CLI with no Chat mode.
type Chat interface {
	// ChatArgv turns a terminal launch line into the headless one.
	ChatArgv(argv []string) []string
	// Init is the line that opens the session; Secret names its answer, which carries account details and is never kept.
	Init() (line []byte, secret string)
	// UserLine is a message from the person or a peer, under uuid, so the CLI's receipts name it.
	UserLine(uuid, text string) []byte
	// InterruptLine stops the running turn; StopTaskLine one piece of background work.
	InterruptLine(id string) []byte
	StopTaskLine(id, task string) []byte
	// Env is what the CLI's environment adds for a Chat session.
	Env() []string
	// Read takes from one frame only the fields an Event allows.
	Read(frame []byte) Event
	// Refuse answers an Ask with no, giving why.
	Refuse(ask Event, why string) []byte
	// Unserved answers any other request as not served, so the CLI never waits on it.
	Unserved(req Event) []byte
}

// ChatOf is a's Chat mode, nil when it has none.
func ChatOf(a Agent) Chat {
	c, _ := a.(Chat)
	return c
}

// EventKind is what a frame says.
type EventKind int

const (
	EventOther EventKind = iota
	// EventInit is the session's start: SessionID.
	EventInit
	// EventCommand is a message's progress: UUID and State (queued, started, completed, cancelled, discarded, refused).
	EventCommand
	// EventEcho is a message the CLI has taken into the conversation: UUID.
	EventEcho
	// EventResult is a turn's end: IsError and Subtype.
	EventResult
	// EventAsk is the CLI waiting on the host to allow a tool: Request (to answer it) and Tool.
	EventAsk
	// EventRequest is any other request the CLI makes of the host: Request.
	EventRequest
	// EventState is the session running a turn or idle: State.
	EventState
	// EventBackground is the whole list of background work still running: Tasks.
	EventBackground
	// EventLimit is the account's usage limit as the CLI last heard it: Limit.
	EventLimit
)

// Task is one piece of background work.
type Task struct{ ID, Description string }

// Limit is a usage window's standing: Blocked once the account is at it, until ResetsAt (unix seconds, 0 unknown).
type Limit struct {
	Blocked  bool
	Window   string
	ResetsAt int64
}

// Event is a frame by allowlist: nothing else is taken from it.
type Event struct {
	Kind      EventKind
	SessionID string
	UUID      string
	State     string
	Subtype   string
	IsError   bool
	Request   string
	Tool      string
	Tasks     []Task
	Limit     Limit
}

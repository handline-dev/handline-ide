package runner

import (
	"time"

	"github.com/handline/handline/go/kit/joined"
)

// BeatEvery is how long a runner waits after the master answered its heartbeat before it sends the next: one is in flight at a time.
const BeatEvery = 15 * time.Second

// Beat is what a runner's heartbeat says of it.
type Beat struct {
	Version  string `json:"version"`
	Platform string `json:"platform"` // GOOS/GOARCH: a master hands its build only to a runner of its own
	// BuildRefused is why this runner refused the master's build (a sha256 that did not match), "" when it did not.
	BuildRefused string        `json:"build_refused,omitempty"`
	Boot         string        `json:"boot"` // the machine's boot: another one means it restarted, and what ran there is gone
	Load         Load          `json:"load"`
	Sessions     []BeatSession `json:"sessions"`
	Accounts     []BeatAccount `json:"accounts"`
	Has          Has           `json:"has"`
	Battery      bool          `json:"battery"` // the machine runs on its battery now
	// Door is where this machine would serve as master (its -listen and certificate), nil when it has no network door.
	Door *Door `json:"door,omitempty"`
	// SetAside is the board this machine set aside when a newer master outranked it as master, nil when it set none aside so.
	SetAside *joined.SetAside `json:"set_aside,omitempty"`
	// App is the Mac app around this runner (a cluster engine's), nil for none.
	App *App `json:"app,omitempty"`
}

// Door is an address a master serves (host:port) and its certificate's fingerprint, "" for a plain door.
type Door struct {
	Addr string `json:"addr"`
	FP   string `json:"fp"`
}

// NodeDoor is one of the owner's machines and its door, as a heartbeat's answer lists them: where a runner looks for the master.
type NodeDoor struct {
	Node string `json:"node"`
	Door
}

// Load is the machine as a heartbeat measured it; a figure it could not read is 0.
type Load struct {
	Cores    int     `json:"cores"`
	Load1    float64 `json:"load1"`
	RAMTotal int64   `json:"ram_total"`
	RAMFree  int64   `json:"ram_free"`
	// AgentsRSS is the memory its sessions' process trees use.
	AgentsRSS int64 `json:"agents_rss"`
}

// BeatSession is a session the runner holds: its agent, terminal, and the branch of its worktree with the commit last pushed.
type BeatSession struct {
	Agent  string `json:"agent"`
	Handle string `json:"handle"`
	Branch string `json:"branch,omitempty"`
	Pushed string `json:"pushed,omitempty"`
}

// BeatAccount is a Claude account folder on the runner, with the email it is logged in as, its config dir (relative to the home), its
// newest usage reading, and whether its login was refused even after a real turn.
type BeatAccount struct {
	Name        string       `json:"name"`
	Email       string       `json:"email"`
	Dir         string       `json:"dir"`
	Reading     *BeatReading `json:"reading,omitempty"`
	LoginFailed bool         `json:"login_failed,omitempty"`
}

// BeatReading is a usage reading (usage.Reading): how long ago it was read, by this machine's clock (the master's may differ), and each
// window's percent used and reset, and its extra usage; Extra nil is unknown.
type BeatReading struct {
	AgeS    int64        `json:"age_s"`
	Windows []BeatWindow `json:"windows"`
	Extra   *BeatExtra   `json:"extra,omitempty"`
}

// BeatExtra is a reading's extra usage (usage.Extra); Cap or Spent nil is not named, Currency "" none named.
type BeatExtra struct {
	On       bool     `json:"on"`
	Cap      *float64 `json:"cap"`
	Spent    *float64 `json:"spent"`
	Currency string   `json:"currency,omitempty"`
}

// BeatWindow is one window of a BeatReading; Pct nil is not read, Reset 0 unknown.
type BeatWindow struct {
	Minutes int   `json:"minutes"`
	Pct     *int  `json:"pct"`
	Reset   int64 `json:"reset,omitempty"`
}

// Has is what the runner has for an agent's work.
type Has struct {
	CLIs       []string `json:"clis"`       // the agent CLIs on its PATH
	MCP        []string `json:"mcp"`        // the MCP servers its accounts set up whose program is there
	Programs   []string `json:"programs"`   // of the programs the master asked about (its answer's check), those on its PATH
	Xcode      bool     `json:"xcode"`      // Xcode is installed
	Simulators []string `json:"simulators"` // the iOS simulators it has
}

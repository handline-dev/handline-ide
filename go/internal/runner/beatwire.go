package runner

import "time"

// BeatEvery is how long a runner waits after the master answered its heartbeat before it sends the next: one is in flight at a time.
const BeatEvery = 15 * time.Second

// Beat is what a runner's heartbeat says of it.
type Beat struct {
	Version  string        `json:"version"`
	Boot     string        `json:"boot"` // the machine's boot: another one means it restarted, and what ran there is gone
	Load     Load          `json:"load"`
	Sessions []BeatSession `json:"sessions"`
	Accounts []BeatAccount `json:"accounts"`
	Has      Has           `json:"has"`
	Battery  bool          `json:"battery"` // the machine runs on its battery now
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

// BeatAccount is a Claude account folder on the runner, with the email it is logged in as and its config dir, relative to the home.
type BeatAccount struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Dir   string `json:"dir"`
}

// Has is what the runner has for an agent's work.
type Has struct {
	CLIs       []string `json:"clis"`       // the agent CLIs on its PATH
	MCP        []string `json:"mcp"`        // the MCP servers its accounts set up whose program is there
	Programs   []string `json:"programs"`   // of the programs the master asked about (its answer's check), those on its PATH
	Xcode      bool     `json:"xcode"`      // Xcode is installed
	Simulators []string `json:"simulators"` // the iOS simulators it has
}

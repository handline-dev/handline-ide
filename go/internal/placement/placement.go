// Package placement picks the machine an agent's session starts on (docs/several-machines.md sections 2 and 7): on Auto a light agent stays
// on the master's machine and a heavy one goes to the lent device with the most room that has all it needs; on Manual an agent pinned to
// a device starts there.
package placement

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/scheduler/agentload"
)

// Key is the setting that says how agents are placed: Auto (the default) places them by load, Manual starts a pinned agent (agents.device)
// on its device and places the others as Auto does.
const (
	Key    = "agent_placement"
	Auto   = "auto"
	Manual = "manual"
)

// What makes an agent heavy, from its last 7 days (agentload): this much heavy command time, or this peak memory (a simulator's).
const (
	HeavyTime   = 2 * time.Hour
	HeavyMemory = int64(6) << 30
)

// Heavy is whether l makes its agent heavy.
func Heavy(l agentload.Load) bool {
	return l.HeavyS >= int64(HeavyTime/time.Second) || l.MemoryPeak >= HeavyMemory
}

// Room is a machine's room for one more session: the sessions on it, its agents' memory and its free memory, in bytes.
type Room struct {
	Sessions           int
	AgentsRSS, RAMFree int64
}

// Limits are the caps a machine is full at; 0 turns one off.
type Limits struct {
	Sessions  int   // most sessions on one machine (host_active_cap)
	AgentsRSS int64 // most memory its agents' sessions use (agents_memory_max_gb), in bytes
}

// Holds is whether a runner holds a workspace's repo (projects.Syncer.Holds).
type Holds func(node, workspace string) bool

// Placer places agents. Here is the master's own room; Limits the caps.
type Placer struct {
	DB     *sql.DB
	Home   string
	Reg    *nodes.Registry
	Holds  Holds
	Here   func() (Room, error)
	Limits func() Limits
	// Manual is whether placement is Manual (Key); nil is Auto.
	Manual func() bool
	Log    func(format string, a ...any)

	mu       sync.Mutex
	reserved map[string]int     // by machine, the sessions picked for it whose terminal is not open yet
	picked   map[string]picking // by agent, its pick until its terminal opens or fails
}

// picking is a pick waiting for its terminal: the machine it opens on, which becomes the agent's once it opens, a fallback here too.
type picking struct {
	node string
}

// need is what an agent's session needs of a machine, and what placement goes by.
type need struct {
	workspace string
	cli       string // its vendor's CLI
	pin       string // the device it is pinned to (agents.device), "" for none
	heavy     bool
	load      agentload.Load
}

// Where is the runner agent's session opens on, "" for this machine (term.Machines.Where). The pick holds a slot there until Placed.
func (p *Placer) Where(agent string) string {
	key, n, err := p.agentNeed(agent) // read before the lock: a load read is not quick
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reserved == nil {
		p.reserved, p.picked = map[string]int{}, map[string]picking{}
	}
	p.release(agent) // a pick before it whose terminal never said
	node, why := p.Reg.Self, ""
	if err == nil {
		node, why, err = p.pick(key, n)
	}
	if err != nil {
		p.log("placement: %s: %v - it starts here", agent, err)
		node = p.Reg.Self
	} else if why != "" {
		p.log("placement: %s: %s (%s)", agent, why, p.name(node))
	}
	p.reserved[node]++
	p.picked[agent] = picking{node}
	if node == p.Reg.Self {
		return ""
	}
	return node
}

// Next is the runner agent's next session would open on now, "" for this machine; it holds nothing (nodes.Registry.DeviceOf).
func (p *Placer) Next(agent string) string {
	pl, err := p.Status(agent)
	if err != nil {
		p.log("placement: %s: %v - its account is chosen as this machine's", agent, err)
		return ""
	}
	if pl.Device == p.Reg.Self {
		return ""
	}
	return pl.Device
}

// Placement is one agent's row of node.status' placement: where its next session starts (a node id), why ("" for a light agent that
// stays on the master's machine), its load, and its pin.
type Placement struct {
	Agent      string `json:"agent"`
	Device     string `json:"device"`
	Why        string `json:"why"`
	Heavy      bool   `json:"heavy"`
	HeavyS     int64  `json:"heavy_s"`
	MemoryPeak int64  `json:"memory_peak"`
	Pin        string `json:"pin,omitempty"`
}

// Status is agent's Placement now, what Devices shows; it holds nothing.
func (p *Placer) Status(agent string) (Placement, error) {
	pl := Placement{Agent: agent, Device: p.Reg.Self}
	key, n, err := p.agentNeed(agent)
	if err != nil || key == "" {
		return pl, err
	}
	pl.Heavy, pl.HeavyS, pl.MemoryPeak, pl.Pin = n.heavy, n.load.HeavyS, n.load.MemoryPeak, n.pin
	p.mu.Lock()
	defer p.mu.Unlock()
	if got, had := p.picked[agent]; had { // as Where picks: without the agent's own pick before
		p.reserved[got.node]--
		defer func() { p.reserved[got.node]++ }()
	}
	pl.Device, pl.Why, err = p.pick(key, n)
	return pl, err
}

// agentNeed is agent's key and need; key "" for one not on the roster, which starts here as before runners.
func (p *Placer) agentNeed(agent string) (string, need, error) {
	key, err := paths.AgentKeyIn(p.DB, agent)
	if err != nil || key == "" {
		return "", need{}, err
	}
	n, err := p.needOf(key)
	return key, n, err
}

// Placed is told whether agent's terminal opened where Where picked (term.Machines.Placed): its slot is given back, and once it opened
// that machine becomes the agent's, wherever it is: agent_state.node says where its session runs, so a heavy agent whose lent device
// went away and which started here is on this machine, not shown on the one it left.
func (p *Placer) Placed(agent string, ok bool) {
	p.mu.Lock()
	got, had := p.picked[agent]
	p.release(agent)
	p.mu.Unlock()
	if !had || !ok {
		return
	}
	key, err := paths.AgentKeyIn(p.DB, agent)
	if err == nil && key != "" {
		_, err = p.DB.Exec(`INSERT INTO agent_state(agent, state, updated, node) VALUES(?, 'IDLE', strftime('%s', 'now'), ?)
			ON CONFLICT(agent) DO UPDATE SET node = excluded.node`, key, got.node)
	}
	if err != nil {
		p.log("placement: %s: its machine %s was not kept: %v", agent, got.node, err)
	}
}

func (p *Placer) release(agent string) {
	if got, had := p.picked[agent]; had {
		p.reserved[got.node]--
		delete(p.picked, agent)
	}
}

// pick is the machine for agent's next session and why it is placed so ("" when it stays here as a light agent does).
func (p *Placer) pick(key string, n need) (node string, why string, err error) {
	if key == "" {
		return p.Reg.Self, "", nil // not on the roster: as before runners, here
	}
	var current sql.NullString
	if err := p.DB.QueryRow("SELECT node FROM agent_state WHERE agent = ?", key).Scan(&current); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	l := p.Limits()
	if n.pin != "" && p.Manual != nil && p.Manual() {
		if n.pin == p.Reg.Self {
			return p.Reg.Self, "", nil
		}
		lacks := p.lacks(n.pin, n, l)
		if lacks == "" {
			return n.pin, "pinned to " + p.name(n.pin), nil
		}
		return p.Reg.Self, fmt.Sprintf("pinned to %s, which cannot take it now (%s), so it starts here", p.name(n.pin), lacks), nil
	}
	if !n.heavy {
		if lacks := p.lacks(p.Reg.Self, n, l); lacks == "" {
			return p.Reg.Self, "", nil
		} else {
			why = "this machine is " + lacks
		}
	} else {
		why = "heavy: " + heavyWhy(n.load)
		if current.String != "" && current.String != p.Reg.Self {
			lacks := p.lacks(current.String, n, l)
			if lacks == "" {
				return current.String, why, nil // a heavy agent stays on its lent device while it can
			}
			why += "; leaves " + p.name(current.String) + ": " + lacks
		}
	}
	best, kept := p.best(n, l, n.heavy)
	if best == "" { // nowhere else fits: this machine, which waits or starts as today
		if kept == "" {
			kept = "no device is lent"
		}
		return p.Reg.Self, why + "; " + kept + ", so it stays here", nil
	}
	return best, why, nil
}

// best is the device with the most room that can take a session needing n, a lent one only when lent; one that has the programs the
// agent's heavy commands ran comes first. kept is why a lent device could not, one that takes work first ("" when none is lent).
func (p *Placer) best(n need, l Limits, lent bool) (best, kept string) {
	want := usedPrograms(n.load)
	room, used := int64(-1), -1
	keptTakes := false
	for _, d := range p.machines() {
		if d == p.Reg.Self && lent {
			continue
		}
		if lacks := p.lacks(d, n, l); lacks != "" {
			if takes := d != p.Reg.Self && p.Reg.Takes(d) == ""; d != p.Reg.Self && (kept == "" || takes && !keptTakes) {
				kept, keptTakes = p.name(d)+": "+lacks, takes
			}
			continue
		}
		r, err := p.room(d)
		if err != nil {
			continue
		}
		b, _, _ := p.Reg.Heard(d)
		has := 0
		for _, prog := range want {
			if contains(b.Has.Programs, prog) {
				has++
			}
		}
		if has > used || has == used && r.RAMFree > room {
			best, used, room = d, has, r.RAMFree
		}
	}
	return best, kept
}

// heavyWhy is a heavy agent's load in a few words: "checks 18 h this week", "peaks at 7 GB".
func heavyWhy(l agentload.Load) string {
	if l.HeavyS >= int64(HeavyTime/time.Second) {
		what := "heavy commands"
		if len(l.Shapes) > 0 {
			what = l.Shapes[0].Name
		}
		return fmt.Sprintf("%s %d h this week", what, (l.HeavyS+1800)/3600)
	}
	return fmt.Sprintf("peaks at %d GB", (l.MemoryPeak+(1<<29))>>30)
}

// usedPrograms is the programs an agent's heavy commands ran (xcodebuild, gradle): a device that has them is preferred, though an agent
// installs what it lacks.
func usedPrograms(l agentload.Load) []string {
	var out []string
	for _, s := range l.Shapes {
		prog, _, _ := strings.Cut(s.Name, " ")
		if !strings.HasPrefix(prog, "@") && !strings.Contains(prog, ".") { // a script (check.sh, run.py) is the repo's, not the device's
			out = append(out, prog)
		}
	}
	return uniq(out)
}

// Lacks is why agent's session cannot start on node now, "" when it can ("codex not installed"): what node.status shows for each agent.
func (p *Placer) Lacks(agent, node string) string {
	key, err := paths.AgentKeyIn(p.DB, agent)
	if err != nil || key == "" {
		return "not an agent here"
	}
	n, err := p.needOf(key)
	if err != nil {
		return err.Error()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lacks(node, n, p.Limits())
}

// lacks is the first reason node cannot take a session that needs n under l, "" when it can.
func (p *Placer) lacks(node string, n need, l Limits) string {
	if node != p.Reg.Self {
		if why := p.Reg.Takes(node); why != "" {
			return why
		}
		if p.Holds == nil || !p.Holds(node, n.workspace) {
			return "its project is not there"
		}
		b, _, _ := p.Reg.Heard(node)
		if !contains(b.Has.CLIs, n.cli) {
			return n.cli + " not installed"
		}
	}
	if node != p.Reg.Self {
		var gb sql.NullInt64
		if err := p.DB.QueryRow("SELECT memory_max_gb FROM nodes WHERE id = ?", node).Scan(&gb); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "its memory cap could not be read: " + err.Error()
		} else if gb.Valid {
			l.AgentsRSS = gb.Int64 << 30 // its own cap (device.set) in place of agents_memory_max_gb
		}
	}
	r, err := p.room(node)
	switch {
	case err != nil:
		return "its sessions could not be counted: " + err.Error()
	case l.Sessions > 0 && r.Sessions >= l.Sessions:
		return fmt.Sprintf("full: %d of %d sessions", r.Sessions, l.Sessions)
	case l.AgentsRSS > 0 && r.AgentsRSS >= l.AgentsRSS:
		return fmt.Sprintf("full: its agents use %d of %d GB", r.AgentsRSS>>30, l.AgentsRSS>>30)
	}
	return ""
}

// machines is this machine and the others of this master, in join order.
func (p *Placer) machines() []string {
	out := []string{p.Reg.Self}
	rs, err := p.DB.Query("SELECT id FROM nodes WHERE id != ? ORDER BY joined_at, id", p.Reg.Self)
	if err != nil {
		p.log("placement: the machines could not be read: %v", err)
		return out
	}
	defer rs.Close()
	for rs.Next() {
		var id string
		if rs.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

// room is node's room now, with the sessions picked for it and not open yet: a runner's from its heartbeat and the sessions the master
// opened there, each one since that heartbeat counted as using what the others use.
func (p *Placer) room(node string) (Room, error) {
	var r Room
	if node == p.Reg.Self {
		if p.Here != nil {
			var err error
			if r, err = p.Here(); err != nil {
				return r, err
			}
		}
		r.Sessions += p.reserved[node]
		return r, nil
	}
	b, _, _ := p.Reg.Heard(node)
	r = Room{AgentsRSS: b.Load.AgentsRSS, RAMFree: b.Load.RAMFree}
	if err := p.DB.QueryRow("SELECT COUNT(*) FROM node_sessions WHERE node = ?", node).Scan(&r.Sessions); err != nil {
		return r, err
	}
	r.Sessions += p.reserved[node]
	if extra := r.Sessions - len(b.Sessions); extra > 0 && len(b.Sessions) > 0 {
		per := b.Load.AgentsRSS / int64(len(b.Sessions))
		r.AgentsRSS += per * int64(extra)
		r.RAMFree -= per * int64(extra)
	}
	return r, nil
}

func (p *Placer) name(node string) string {
	var name string
	if p.DB.QueryRow("SELECT name FROM nodes WHERE id = ?", node).Scan(&name) != nil {
		return node
	}
	return name
}

// needOf is what the agent keyed key needs: its workspace, its vendor's CLI, its pin and its load.
func (p *Placer) needOf(key string) (need, error) {
	_, id, ok := actor.Parse(key)
	if !ok {
		return need{}, fmt.Errorf("%s is not an agent's key", key)
	}
	var ws int64
	var vendor, pin sql.NullString
	if err := p.DB.QueryRow("SELECT workspace, vendor, device FROM agents WHERE id = ?", id).Scan(&ws, &vendor, &pin); err != nil {
		return need{}, err
	}
	n := need{workspace: actor.Key(actor.Workspace, ws), cli: team.DefaultVendor, pin: pin.String}
	if vendor.String != "" {
		n.cli = vendor.String
	}
	var err error
	if n.load, err = agentload.Of(p.DB, key, p.now()); err != nil {
		return need{}, err
	}
	n.heavy = Heavy(n.load)
	return n, nil
}

func (p *Placer) now() int64 {
	if p.Reg != nil {
		return p.Reg.Now().Unix()
	}
	return time.Now().Unix()
}

// Check is every program an agent's heavy commands ran: what a runner is asked to say it has (nodes.Registry.Check).
func (p *Placer) Check() []string {
	var out []string
	loads, err := agentload.All(p.DB, p.now())
	if err != nil {
		p.log("placement: the agents' loads could not be read: %v", err)
	}
	for _, l := range loads {
		out = append(out, usedPrograms(l)...)
	}
	return uniq(out)
}

func uniq(s []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (p *Placer) log(format string, a ...any) {
	if p.Log != nil {
		p.Log(format, a...)
	}
}

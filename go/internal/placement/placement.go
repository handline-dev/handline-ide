// Package placement picks the machine an agent's session starts on (docs/story4-design.md section 2, slice 10): the agent's own while
// it can take the session, else the one with the most free memory that can, which becomes the agent's.
package placement

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/handline/handline/go/data/team"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/kit/actor"
	"github.com/handline/handline/go/kit/paths"
)

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
	Log    func(format string, a ...any)

	mu       sync.Mutex
	reserved map[string]int     // by machine, the sessions picked for it whose terminal is not open yet
	picked   map[string]picking // by agent, its pick until its terminal opens or fails
}

// picking is a pick waiting for its terminal: the machine, and whether it becomes the agent's (a fallback here does not).
type picking struct {
	node string
	keep bool
}

// need is what an agent's session needs of a machine.
type need struct {
	workspace string
	cli       string   // its vendor's CLI
	programs  []string // the programs of its MCP servers
}

// Where is the runner agent's session opens on, "" for this machine (term.Machines.Where). The pick holds a slot there until Placed.
func (p *Placer) Where(agent string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reserved == nil {
		p.reserved, p.picked = map[string]int{}, map[string]picking{}
	}
	p.release(agent) // a pick before it whose terminal never said
	node, keep, why, err := p.pick(agent)
	if err != nil {
		p.log("placement: %s: %v - it starts here", agent, err)
		node, keep = p.Reg.Self, false
	} else if why != "" {
		p.log("placement: %s: %s", agent, why)
	}
	p.reserved[node]++
	p.picked[agent] = picking{node, keep}
	if node == p.Reg.Self {
		return ""
	}
	return node
}

// Next is the runner agent's next session would open on now, "" for this machine; it holds nothing (nodes.Registry.DeviceOf).
func (p *Placer) Next(agent string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if got, had := p.picked[agent]; had { // as Where picks: without the agent's own pick before
		p.reserved[got.node]--
		defer func() { p.reserved[got.node]++ }()
	}
	node, _, _, err := p.pick(agent)
	if err != nil {
		p.log("placement: %s: %v - its account is chosen as this machine's", agent, err)
	}
	if err != nil || node == p.Reg.Self {
		return ""
	}
	return node
}

// Placed is told whether agent's terminal opened where Where picked (term.Machines.Placed): its slot is given back, and once it opened
// that machine becomes the agent's.
func (p *Placer) Placed(agent string, ok bool) {
	p.mu.Lock()
	got, had := p.picked[agent]
	p.release(agent)
	p.mu.Unlock()
	if !had || !ok || !got.keep {
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

// pick is the machine for agent's next session, whether it becomes the agent's, and why it moved ("" when it stays).
func (p *Placer) pick(agent string) (node string, keep bool, why string, err error) {
	key, err := paths.AgentKeyIn(p.DB, agent)
	if err != nil || key == "" {
		return p.Reg.Self, false, "", err // not on the roster: as before runners, here
	}
	n, err := p.needOf(key)
	if err != nil {
		return "", false, "", err
	}
	var current, worktree sql.NullString
	err = p.DB.QueryRow("SELECT node, worktree FROM agent_state WHERE agent = ?", key).Scan(&current, &worktree)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, "", err
	}
	if !current.Valid && worktree.String != "" { // it ran before runners: on this machine
		current = sql.NullString{String: p.Reg.Self, Valid: true}
	}
	l := p.Limits()
	if current.String != "" {
		lacks := p.lacks(current.String, n, l)
		if lacks == "" {
			return current.String, current.Valid && current.String != "", "", nil
		}
		why = fmt.Sprintf("leaves %s: %s", p.name(current.String), lacks)
	}
	best, room := "", int64(-1)
	for _, d := range p.machines() {
		if d == current.String || p.lacks(d, n, l) != "" {
			continue
		}
		if r, err := p.room(d); err == nil && r.RAMFree > room {
			best, room = d, r.RAMFree
		}
	}
	if best == "" { // nowhere fits: this machine, which waits or starts as today; the agent's own stays its own
		return p.Reg.Self, false, strings.TrimPrefix(why+"; no machine can take it, so it starts here", "; "), nil
	}
	if why != "" {
		why += " - goes to " + p.name(best)
	}
	return best, true, why, nil
}

// Lacks is why agent's session cannot start on node now, "" when it can ("needs pencil"): what node.status shows for each agent.
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
			return "needs " + n.cli
		}
		for _, prog := range n.programs {
			if !contains(b.Has.Programs, prog) {
				return "needs " + prog
			}
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

// needOf is what the agent keyed key needs: its workspace, its vendor's CLI, and the programs of its own MCP servers and the shared ones.
func (p *Placer) needOf(key string) (need, error) {
	_, id, ok := actor.Parse(key)
	if !ok {
		return need{}, fmt.Errorf("%s is not an agent's key", key)
	}
	var ws int64
	var vendor, mcp sql.NullString
	if err := p.DB.QueryRow("SELECT workspace, vendor, mcp_config FROM agents WHERE id = ?", id).Scan(&ws, &vendor, &mcp); err != nil {
		return need{}, err
	}
	n := need{workspace: actor.Key(actor.Workspace, ws), cli: team.DefaultVendor}
	if vendor.String != "" {
		n.cli = vendor.String
	}
	n.programs = Programs(mcp.String)
	if b, err := os.ReadFile(paths.MCPConfig(p.Home)); err == nil {
		n.programs = append(n.programs, Programs(string(b))...)
	}
	n.programs = uniq(n.programs)
	return n, nil
}

// Programs is the programs a {"mcpServers": {...}} config runs, by their file name: a server reached at a URL runs none.
func Programs(config string) []string {
	var cfg struct {
		Servers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal([]byte(config), &cfg) != nil {
		return nil
	}
	var out []string
	for _, s := range cfg.Servers {
		if s.Command != "" {
			out = append(out, filepath.Base(s.Command))
		}
	}
	return uniq(out)
}

// Check is every program an agent's MCP servers run: what a runner is asked to say it has (nodes.Registry.Check).
func (p *Placer) Check() []string {
	var out []string
	rs, err := p.DB.Query("SELECT mcp_config FROM agents WHERE mcp_config IS NOT NULL")
	if err == nil {
		for rs.Next() {
			var c string
			if rs.Scan(&c) == nil {
				out = append(out, Programs(c)...)
			}
		}
		err = errors.Join(rs.Err(), rs.Close())
	}
	if err != nil {
		p.log("placement: the agents' MCP servers could not be read: %v", err)
	}
	if b, err := os.ReadFile(paths.MCPConfig(p.Home)); err == nil {
		out = append(out, Programs(string(b))...)
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

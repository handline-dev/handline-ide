package nodes

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/protocol/rpc"
)

// A runner's heartbeat (runner.Beat, docs/story4-design.md section 1 item 4, slice 9): a runner the master has not heard from for
// OfflineAfter is offline; after the reclaim time (ReclaimKey, ReclaimS
// by default) offline, its sessions are taken back.
const (
	OfflineAfter = 60 * time.Second
	ReclaimKey   = "node_reclaim_s"
	ReclaimS     = 30 * 60
	// BatteryKey is the battery switch: a Mac whose heartbeat says it went on battery is set to Off, and back to Run when it is
	// plugged in again. Off by default.
	BatteryKey = "node_battery_switch"
)

// heard is a runner's newest heartbeat and when it came.
type heard struct {
	beat runner.Beat
	at   time.Time
}

// Heard is node's newest heartbeat and when it came; ok false when none came since this master started.
func (r *Registry) Heard(node string) (b runner.Beat, at time.Time, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.beats[node]
	return h.beat, h.at, ok
}

// Hear keeps b as node's newest heartbeat, heard at at.
func (r *Registry) Hear(node string, b runner.Beat, at time.Time) {
	r.mu.Lock()
	r.beats[node] = heard{beat: b, at: at}
	r.mu.Unlock()
}

// Online is whether node is the master or has sent a heartbeat within OfflineAfter.
func (r *Registry) Online(node string) bool {
	if node == r.Self {
		return true
	}
	_, at, ok := r.Heard(node)
	return ok && r.now().Sub(at) <= OfflineAfter
}

// Takes is "" when a new session may open on node now, else why not: it is offline, on another Handline version than the master, or
// its person set it to Pause or Off.
func (r *Registry) Takes(node string) string {
	if node == r.Self {
		return ""
	}
	b, _, _ := r.Heard(node)
	var lending string
	if err := r.DB.QueryRow("SELECT lending FROM nodes WHERE id = ?", node).Scan(&lending); err != nil {
		return "it is not a machine of this master"
	}
	switch {
	case !r.Online(node):
		return "it is offline"
	case b.Version == "":
		return "it has not sent its heartbeat yet"
	case r.Version != "" && b.Version != r.Version:
		return fmt.Sprintf("it runs Handline %s, not the master's %s: update to take work", short(b.Version), short(r.Version))
	case lending != "run":
		return "it is set to " + lending
	}
	return ""
}

func short(sha string) string {
	if len(sha) > 9 {
		return sha[:9]
	}
	if sha == "" {
		return "?"
	}
	return sha
}

// heartbeat is node.heartbeat: the newest beat is kept, in memory and as the node row's last_seen. The answer names the sessions the
// runner holds that the master no longer places there (it reclaimed them, or opened the agent elsewhere): the runner puts what they
// left on a rescue branch and stops them.
func (r *Registry) heartbeat(c *rpc.Conn, p map[string]any) (any, error) {
	node := r.NodeOf(c.Root())
	if node == "" {
		return nil, rpc.Refused("node.heartbeat is a runner's own call, on its link", "node", "")
	}
	raw, err := json.Marshal(p)
	var b runner.Beat
	if err != nil || json.Unmarshal(raw, &b) != nil {
		return nil, rpc.BadRequestError("the heartbeat is a runner's beat", "beat", rpc.InvalidParams)
	}
	now := r.now()
	r.mu.Lock()
	before, had := r.beats[node]
	r.beats[node] = heard{beat: b, at: now}
	opening := map[string]bool{}
	for a := range r.opening[node] {
		opening[a] = true
	}
	r.mu.Unlock()
	if _, err := r.DB.Exec("UPDATE nodes SET last_seen = ? WHERE id = ?", now.Unix(), node); err != nil {
		return nil, err
	}
	if !had || before.beat.Battery != b.Battery {
		if err := r.battery(node, b.Battery); err != nil {
			return nil, err
		}
	}
	placed := map[string]string{}
	rs, err := r.DB.Query("SELECT agent, handle FROM node_sessions WHERE node = ?", node)
	if err != nil {
		return nil, err
	}
	for rs.Next() {
		var a, h string
		if rs.Scan(&a, &h) == nil {
			placed[a] = h
		}
	}
	if err := errors.Join(rs.Err(), rs.Close()); err != nil {
		return nil, err
	}
	stop := []string{}
	for _, s := range b.Sessions {
		if placed[s.Agent] != s.Handle && !opening[s.Agent] {
			stop = append(stop, s.Agent)
		}
	}
	check := []string{}
	if r.Check != nil {
		check = r.Check()
	}
	return map[string]any{"stop": stop, "check": check}, nil
}

// Reclaimed is a session the sweep took back from a runner: the task it was on and the memo that task was given.
type Reclaimed struct {
	Agent, Node string
	Task        int64
	Memo        string
}

// Sweep takes back the sessions of each runner offline past reclaimS (Reclaim).
func (r *Registry) Sweep(reclaimS int64) ([]Reclaimed, error) {
	now := r.now()
	type gone struct {
		node string
		why  string
	}
	var todo []gone
	rs, err := r.DB.Query("SELECT DISTINCT node FROM node_sessions")
	if err != nil {
		return nil, err
	}
	var held []string
	for rs.Next() {
		var n string
		if rs.Scan(&n) == nil {
			held = append(held, n)
		}
	}
	if err := errors.Join(rs.Err(), rs.Close()); err != nil {
		return nil, err
	}
	for _, n := range held {
		_, at, ok := r.Heard(n)
		if !ok {
			at = r.started // heard of since this master started, or offline from then
		}
		if off := now.Sub(at) - OfflineAfter; off >= time.Duration(reclaimS)*time.Second {
			todo = append(todo, gone{n, fmt.Sprintf("offline since %s", at.Add(OfflineAfter).Format("15:04"))})
		}
	}
	var out []Reclaimed
	for _, g := range todo {
		got, err := r.Reclaim(g.node, g.why)
		out = append(out, got...)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

// Reclaim takes back node's sessions, why it was: each leaves node_sessions, so its terminal reads gone and the lifecycle starts its
// agent again by the usual placement; it is not resumed (its conversation is on that machine), and the task it was on is given a memo
// naming the commit last pushed on its branch.
func (r *Registry) Reclaim(node, why string) ([]Reclaimed, error) {
	b, _, _ := r.Heard(node)
	last := map[string]runner.BeatSession{}
	for _, s := range b.Sessions {
		last[s.Agent] = s
	}
	var out []Reclaimed
	for _, a := range r.agentsOn(node) {
		key := a
		if r.local != nil {
			key = r.local().AgentCaller(a).Key
		}
		rec := Reclaimed{Agent: key, Node: node, Memo: reclaimMemo(r.nodeName(node), why, last[a])}
		if err := r.takeBack(a, node, &rec); err != nil {
			return out, fmt.Errorf("reclaim %s from %s: %w", a, node, err)
		}
		r.forget(a)
		r.log("nodes: %s: %s taken back (%s)", node, a, why)
		out = append(out, rec)
	}
	return out, nil
}

// takeBack drops agent's session on node and gives its task rec's memo, in one transaction.
func (r *Registry) takeBack(agent, node string, rec *Reclaimed) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM node_sessions WHERE agent = ? AND node = ?", agent, node); err != nil {
		return err
	}
	if err := db.ForgetResume(tx, rec.Agent); err != nil {
		return err
	}
	if rec.Task, err = db.DoingTask(tx, rec.Agent); err != nil {
		return err
	}
	if rec.Task != 0 {
		if _, err := db.AddMemo(tx, rec.Task, "daemon", rec.Memo); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Registry) nodeName(node string) string {
	var name string
	if r.DB.QueryRow("SELECT name FROM nodes WHERE id = ?", node).Scan(&name) != nil {
		return node
	}
	return name
}

// reclaimMemo is the memo a reclaimed session's task is given.
func reclaimMemo(machine, why string, s runner.BeatSession) string {
	var at string
	switch {
	case s.Branch == "":
		at = "No branch of it was pushed from there."
	case s.Pushed == "":
		at = fmt.Sprintf("Nothing on %s was pushed from there yet.", s.Branch)
	default:
		at = fmt.Sprintf("The last commit pushed from there is %s on %s; the work goes on from it.", short(s.Pushed), s.Branch)
	}
	return strings.Join([]string{fmt.Sprintf("Taken back from %s (%s), to start on another machine.", machine, why), at}, " ")
}

// accountHere is the master's name for account, an account folder on node: the master's account logged in as the email node's newest
// heartbeat names for it, else account as node calls it.
func (r *Registry) accountHere(node, account string) string {
	if r.AccountOf == nil {
		return account
	}
	b, _, _ := r.Heard(node)
	for _, a := range b.Accounts {
		if a.Name == account && a.Email != "" {
			if here := r.AccountOf(a.Email); here != "" {
				return here
			}
		}
	}
	return account
}

// battery is the battery switch, when it is on: node gone on battery is set to Off from Run, and back to Run once plugged in, unless its
// person set it otherwise meanwhile. That the switch set it is kept on the board (the battery_off:<node> cursor), so a restart keeps it.
func (r *Registry) battery(node string, on bool) error {
	if r.BatterySwitch == nil || !r.BatterySwitch() {
		return nil
	}
	mark := "battery_off:" + node
	from, to := "run", "off"
	if !on {
		switched, err := db.Cursor(r.DB, mark)
		if err != nil || switched == 0 {
			return err
		}
		from, to = "off", "run"
	}
	res, err := r.DB.Exec("UPDATE nodes SET lending = ? WHERE id = ? AND lending = ? AND device IS NOT NULL", to, node, from)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	switch {
	case on && n > 0:
		err = db.SetCursor(r.DB, mark, r.now().Unix())
	case !on:
		err = db.ClearCursor(r.DB, mark)
	}
	if n > 0 {
		r.log("nodes: %s: on %s, so set to %s", node, map[bool]string{true: "battery", false: "power"}[on], to)
	}
	return err
}

// OffMachine is the name of the machine agent's session runs on when its person set it to Off, "" when it runs on none such.
func (r *Registry) OffMachine(agent string) string {
	rs, err := r.DB.Query("SELECT s.agent, n.name FROM node_sessions s JOIN nodes n ON n.id = s.node WHERE n.lending = 'off'")
	if err != nil {
		return ""
	}
	defer rs.Close()
	for rs.Next() {
		var a, name string
		if rs.Scan(&a, &name) == nil && (a == agent || r.local != nil && r.local().AgentCaller(a).Key == agent) {
			return name
		}
	}
	return ""
}

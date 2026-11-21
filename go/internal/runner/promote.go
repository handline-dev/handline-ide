package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/handline/handline/go/internal/boardcopy"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/kit/atomicfile"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/protocol/rpc"
)

// The master moved by hand (docs/story4-design.md section 4, slice 18): node.promote makes this runner the master, and a runner whose
// master is gone looks for the one that is master now among the owner's machines its heartbeats were told of.

// HandoverWait is how long a master handing over waits for this machine to hold its last change; node.promote waits twice that for
// its answer, then takes over without it.
const HandoverWait = 30 * time.Second

// DoorsFile is where a runner keeps the owner's machines and their doors, as its last heartbeat's answer listed them.
func DoorsFile(home string) string { return filepath.Join(home, "master-doors.json") }

// master is the master this runner obeys now.
func (r *Runner) master() joined.Master {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg.Master
}

// obey makes m the master this runner obeys, kept for its next start.
func (r *Runner) obey(m joined.Master) {
	r.mu.Lock()
	same := r.cfg.Master == m
	r.cfg.Master = m
	r.mu.Unlock()
	if same {
		return
	}
	if err := joined.Keep(r.cfg.Home, m); err != nil {
		r.cfg.Log("runner: the master %s was not kept for the next start: %v", m.Addr, err)
	}
}

// heard is what a master's hello says of it, set on the master this runner obeys: an older master than the one it obeys is refused.
func (r *Runner) heard(m joined.Master, says hlclient.MasterSays) (joined.Master, error) {
	if m.Node != "" && says.Node != m.Node && !says.Above(m.Number, m.Node) || says.Number < m.Number {
		return m, fmt.Errorf("%s is master number %d, below the %d this runner obeys", m.Addr, says.Number, m.Number)
	}
	if says.Node != "" {
		m.Node = says.Node
	}
	m.Number = max(m.Number, says.Number)
	return m, nil
}

// keepDoors keeps the doors a heartbeat's answer listed.
func (r *Runner) keepDoors(raw any) {
	b, err := json.Marshal(raw)
	var doors []NodeDoor
	if err != nil || json.Unmarshal(b, &doors) != nil {
		return
	}
	if reflect.DeepEqual(doors, r.doors()) {
		return
	}
	if err := atomicfile.Write(DoorsFile(r.cfg.Home), string(b)); err != nil {
		r.cfg.Log("runner: the owner's machines were not kept: %v", err)
	}
}

// doors is the owner's machines with a door, as last kept.
func (r *Runner) doors() []NodeDoor {
	var out []NodeDoor
	if b, err := os.ReadFile(DoorsFile(r.cfg.Home)); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

// lookForMaster asks each of the owner's other machines at once at its door, pinned to its fingerprint, who is master there, and
// obeys the highest one not below the master obeyed now. Only a master listens, so only a master answers.
func (r *Runner) lookForMaster() {
	m := r.master()
	var asks []hlclient.Door
	for _, d := range r.doors() {
		if d.Node != r.self() {
			asks = append(asks, hlclient.Door{Node: d.Node, Addr: d.Addr, FP: d.FP})
		}
	}
	best, says, ok := hlclient.Highest(asks, m.Token, Client+"-ask")
	if !ok || best.Addr == m.Addr && best.FP == m.Fingerprint || says.Node != m.Node && !says.Above(m.Number, m.Node) {
		return
	}
	r.cfg.Log("runner: %s is master number %d now, at %s", best.Node, says.Number, best.Addr)
	m.Addr, m.Fingerprint, m.Node, m.Number = best.Addr, best.FP, best.Node, says.Number
	r.obey(m)
}

// self is this machine's node id.
func (r *Runner) self() string { return joined.NodeIDOf(r.cfg.Home) }

// Promoted is closed once node.promote made this machine the master: main stops the runner and starts it again, as master.
func (r *Runner) Promoted() <-chan struct{} { return r.promoted }

// promote is node.promote: the master hands over (node.handover), or, when it does not answer, this machine takes over from its copy;
// the copy becomes this home's board and, once the answer is sent, the daemon starts again as master.
func (r *Runner) promote(c *rpc.Conn, _ map[string]any) (any, error) {
	if !r.promoting.CompareAndSwap(false, true) {
		return nil, rpc.Refused("this machine is already being made the master", "state", "")
	}
	done := false
	defer func() {
		if !done { // made the master, it stays so until the start again
			r.promoting.Store(false)
		}
	}()
	if r.cfg.DBFile == "" {
		return nil, rpc.Refused("this runner has no board file to take over to", "board", "")
	}
	if r.door() == nil {
		return nil, rpc.Refused("this machine has no network door to serve as master (handlined -listen)", "door", "")
	}
	dir := boardcopy.Dir(r.cfg.Home)
	if files, err := boardcopy.List(dir); err != nil {
		return nil, err
	} else if len(files) == 0 {
		return nil, rpc.Refused("this machine keeps no copy of the board: only the owner's machines do, once they have pulled one", "copy", "")
	}
	m := r.master()
	p := joined.Promotion{Number: max(m.Number, 1) + 1, From: m.Node, Tell: true}
	if l := r.linked(); l != nil {
		got, err := l.Call("node.handover", map[string]any{}, 2*HandoverWait)
		var refused *rpc.Error
		switch {
		case errors.As(err, &refused): // it answered no: nothing moves
			return nil, fmt.Errorf("the master did not hand over: %w", err)
		case err != nil: // no answer: it may have stepped down meanwhile, or never will; told every 15 s (node.outranked)
			r.cfg.Log("runner: the master did not answer the handover (%v), so this machine takes over from its copy", err)
		default:
			if n, ok := rpc.AsInt(got["number"]); ok && n > m.Number {
				p.Number = n
			}
			p.Tell = false // it joins this machine as a runner by itself
		}
	}
	r.pulling.Lock()
	defer r.pulling.Unlock()
	r.leading = true // no pull writes into the copy from here
	if err := r.lead(dir, m, p); err != nil {
		r.leading = false
		joined.DropPromotion(r.cfg.Home)
		return nil, err
	}
	done = true
	r.cfg.Log("runner: made the master, number %d", p.Number)
	c.AfterReply(func() { r.promoteOnce.Do(func() { close(r.promoted) }) })
	return map[string]any{"number": p.Number, "handed_over": !p.Tell}, nil
}

// lead makes this home the master: the copy restored as its board, p kept for its first start, and its join token its own device.
func (r *Runner) lead(dir string, m joined.Master, p joined.Promotion) error {
	if err := restore(dir, r.cfg.DBFile, time.Now()); err != nil {
		return fmt.Errorf("the copy did not restore: %w", err)
	}
	if err := joined.KeepPromotion(r.cfg.Home, p); err != nil {
		return err
	}
	return joined.Lead(r.cfg.Home, m)
}

// door is where this machine would serve as master now.
func (r *Runner) door() *Door {
	if r.cfg.Door == nil {
		return nil
	}
	return r.cfg.Door()
}

// restore makes the board file at dest from the copy in dir: a board already there is set aside first, unchanged.
func restore(dir, dest string, now time.Time) error {
	tmp := dest + ".restoring"
	os.Remove(tmp)
	if err := boardcopy.Restore(dir, tmp); err != nil {
		return err
	}
	aside := filepath.Join(filepath.Dir(dest), "db.set-aside-"+strconv.FormatInt(now.Unix(), 10)+".sqlite")
	for _, s := range []string{"", "-wal", "-shm"} {
		if err := os.Rename(dest+s, aside+s); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return os.Rename(tmp, dest)
}

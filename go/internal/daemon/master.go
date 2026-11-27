package daemon

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/handline/handline/go/agentctl/term"
	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/internal/nodes"
	"github.com/handline/handline/go/internal/runner"
	"github.com/handline/handline/go/kit/joined"
	"github.com/handline/handline/go/protocol/devices"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/sysres"
)

// The master moved by hand (docs/several-machines.md section 4).

// askClient is the client a master names when it asks another machine's door: not a runner's link.
const askClient = "handlined-master"

// mastership is this daemon's standing as master: its number, whether it holds its writes, and whether it must start again.
type mastership struct {
	number  atomic.Int64 // its own node row's, read at start: it changes only by a move, which starts the daemon again
	moving  atomic.Bool  // a handover runs
	held    atomic.Pointer[string]
	again   chan struct{}
	once    sync.Once
	checked chan struct{} // closed once the start check has run
}

func newMastership() *mastership {
	return &mastership{again: make(chan struct{}), checked: make(chan struct{})}
}

// hold refuses every write from now with why; "" takes them again.
func (m *mastership) hold(why string) {
	if why == "" {
		m.held.Store(nil)
	} else {
		m.held.Store(&why)
	}
}

// refusal is rpc.Server.Hold: the refusal while writes are held.
func (m *mastership) refusal() error {
	if why := m.held.Load(); why != nil {
		return rpc.Refused(*why, "master", "")
	}
	return nil
}

func (m *mastership) startAgain() { m.once.Do(func() { close(m.again) }) }

// StartAgain is closed once this daemon must stop and start again: as a runner once a newer master outranked it, or as master after a
// move that did not finish. Main stops it and runs its executable again.
func (d *Daemon) StartAgain() <-chan struct{} { return d.master.again }

// Checked is closed once the start check has asked the owner's other machines; main starts the tick only after it.
func (d *Daemon) Checked() <-chan struct{} { return d.master.checked }

// moves is whether this daemon takes part in moving the master: it keeps the board's copy and has a network door.
func (d *Daemon) moves() bool { return d.cfg.BoardCopy && d.cfg.Listen != "" }

// number is this master's master number.
func (d *Daemon) number() int64 { return d.master.number.Load() }

// readNumber reads the master number from the master's own node row.
func (d *Daemon) readNumber() error {
	var n int64
	if err := d.db.QueryRow("SELECT master_number FROM nodes WHERE id = ?", d.nodes.Self).Scan(&n); err != nil {
		return fmt.Errorf("the master number: %w", err)
	}
	d.master.number.Store(n)
	return nil
}

// machineKind is what a join names this machine: a mac, or a server for anything else.
func machineKind() string {
	if runtime.GOOS == "darwin" {
		return "mac"
	}
	return "server"
}

// ensureOwn keeps the master's own device in the board and its token in the protected store.
func (d *Daemon) ensureOwn() error {
	own, _ := joined.OwnOf(d.cfg.Home)
	token, device, err := devices.EnsureOwn(d.db, d.nodes.Self, sysres.MachineName(), machineKind(), own.Token, time.Now())
	if err != nil {
		return err
	}
	if token == own.Token && device == own.Device {
		return nil
	}
	return joined.KeepOwn(d.cfg.Home, joined.Own{Token: token, Device: device})
}

// keepOwnDoor writes the door a pairing names into the master's own node row: where the owner's runners find it after a move.
func (d *Daemon) keepOwnDoor() error {
	door, ok := d.net.pairDoor()
	if !ok {
		return nil
	}
	_, err := d.db.Exec("UPDATE nodes SET door = ?, door_fp = ? WHERE id = ?", door.Addr, door.Fingerprint, d.nodes.Self)
	return err
}

// OwnDoor is the door this machine would serve as master with cfg, without opening it: the one a pairing names by default, its
// certificate made when it has none yet. nil when cfg has no network listener.
func OwnDoor(cfg Config) (*runner.Door, error) {
	if cfg.Listen == "" {
		return nil, nil
	}
	if cfg.plain() {
		addr, err := plainAddr(cfg.Listen, cfg.ifaces)
		if err != nil {
			return nil, err
		}
		cfg.Listen = addr
	}
	if err := checkBind(cfg.Listen); err != nil {
		return nil, err
	}
	_, port, _ := net.SplitHostPort(cfg.Listen)
	if port == "0" { // bound only when listening: no address to give
		return nil, nil
	}
	n := &netDoors{cfg: cfg, port: port}
	if !cfg.plain() {
		n.src = &certSource{certFile: cfg.CertFile, keyFile: cfg.KeyFile}
	}
	hosts, specs, _, _, onlyPicked, chosen := n.wantAll()
	if onlyPicked {
		hosts = hosts[:len(hosts)-1]
	}
	if len(hosts) == 0 {
		return nil, nil
	}
	host := hosts[0]
	if chosen {
		for _, a := range hosts {
			if a.Kind == kindLAN {
				host = a
				break
			}
		}
	}
	addr := net.JoinHostPort(host.Host, port)
	if !chosen && cfg.Advertise != "" {
		addr = cfg.Advertise
	}
	door := &runner.Door{Addr: addr}
	if specs[host.IPs[0].String()].tls {
		src, err := n.tlsSource()
		if err != nil {
			return nil, err
		}
		c, err := src.get()
		if err != nil || len(c.Certificate) == 0 {
			return nil, fmt.Errorf("network listener certificate: %v", err)
		}
		sum := sha256.Sum256(c.Certificate[0])
		door.FP = hex.EncodeToString(sum[:])
	}
	return door, nil
}

// applyPromotion writes a promotion this home was made master by into the board, once: the master number it took, and the sessions
// the board has as the old master's own, which are that machine's now (taken back like any offline runner's, slice 9), while this
// machine's sessions, which it ran as a runner, are its own.
func applyPromotion(db *sql.DB, home, self string, now time.Time) error {
	p, ok := joined.PromotionOf(home)
	if !ok || p.Applied {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE nodes SET master_number = MAX(master_number, ?), lending = 'run' WHERE id = ?", p.Number, self); err != nil {
		return err
	}
	type held struct{ agent, handle string }
	var theirs, mine []held
	rs, err := tx.Query("SELECT agent, terminal FROM agent_state WHERE terminal IS NOT NULL AND terminal != ''")
	if err != nil {
		return err
	}
	for rs.Next() {
		var h held
		if err := rs.Scan(&h.agent, &h.handle); err != nil {
			rs.Close()
			return err
		}
		if node, local, ok := term.OnRunner(h.handle); !ok {
			theirs = append(theirs, h)
		} else if node == self {
			mine = append(mine, held{h.agent, local})
		}
	}
	if err := errors.Join(rs.Err(), rs.Close()); err != nil {
		return err
	}
	if p.From != "" && p.From != self {
		for _, h := range theirs {
			if _, err := tx.Exec("UPDATE agent_state SET terminal = ? WHERE agent = ?", term.RunnerHandle(p.From, h.handle), h.agent); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO node_sessions(agent, node, handle, opened_at) VALUES(?, ?, ?, ?)
				ON CONFLICT(agent) DO UPDATE SET node = excluded.node, handle = excluded.handle, opened_at = excluded.opened_at`,
				h.agent, p.From, h.handle, now.Unix()); err != nil {
				return err
			}
		}
	}
	for _, h := range mine {
		if _, err := tx.Exec("UPDATE agent_state SET terminal = ? WHERE agent = ?", h.handle, h.agent); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("DELETE FROM node_sessions WHERE node = ?", self); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.Applied = true
	if !p.Tell {
		return joined.DropPromotion(home)
	}
	return joined.KeepPromotion(home, p)
}

// startCheck is the start check: before this master fires a timer or starts work, it asks each of the owner's other machines with a
// door whether it is master with a higher number. One that is outranks this one, which joins it as a runner.
func (d *Daemon) startCheck() {
	defer close(d.master.checked)
	doors, err := nodes.Doors(d.db)
	if err != nil {
		d.cfg.Log("master: the owner's machines could not be read, so none was asked: %v", err)
		return
	}
	var others []runner.NodeDoor
	for _, door := range doors {
		if door.Node != d.nodes.Self {
			others = append(others, door)
		}
	}
	d.outrankedBy(others)
}

// outrankedBy asks each door at once whether a master above this one is there (MasterSays.Above), and joins the highest that is.
func (d *Daemon) outrankedBy(doors []runner.NodeDoor) {
	own, ok := joined.OwnOf(d.cfg.Home)
	if !ok || len(doors) == 0 {
		return
	}
	asks := make([]hlclient.Door, len(doors))
	for i, door := range doors {
		asks[i] = hlclient.Door{Node: door.Node, Addr: door.Addr, FP: door.FP}
	}
	best, says, found := hlclient.Highest(asks, own.Token, askClient)
	mine := d.number()
	if !found || !says.Above(mine, d.nodes.Self) {
		return
	}
	d.cfg.Log("master: %s is master number %d, above this one's %d: this machine joins it as a runner", best.Node, says.Number, mine)
	d.becomeRunner(joined.Master{Addr: best.Addr, Fingerprint: best.FP, Token: own.Token, Device: own.Device,
		At: time.Now().Unix(), Node: best.Node, Number: says.Number})
}

// becomeRunner holds the writes, keeps m as the master this home joined with its own device's token, and has main stop the tick and
// start it again: as a runner, which sets this board aside unchanged.
func (d *Daemon) becomeRunner(m joined.Master) {
	d.master.hold("this machine is no longer the master: it joins the master at " + m.Addr)
	if err := joined.KeepSetAside(d.cfg.Home, joined.SetAside{Since: time.Now().Unix(), Master: m.Number}); err != nil {
		d.cfg.Log("master: the set-aside board was not noted: %v", err)
	}
	if err := joined.Become(d.cfg.Home, m); err != nil { // started again as master, its start check tries again
		d.cfg.Log("master: the master to join was not kept: %v", err)
	}
	d.master.startAgain()
}

// stopWork stops the tick (Config.StopWork) and waits for it.
func (d *Daemon) stopWork() {
	if d.cfg.StopWork != nil {
		d.cfg.StopWork()
	}
}

// canHandOver is why this master cannot hand over to node now, nil when it can; it answers its own device and node's door.
func (d *Daemon) canHandOver(node string) (own joined.Own, addr, fp string, err error) {
	if d.copier == nil || d.nodes.Copy == nil || !d.moves() {
		return own, "", "", rpc.Refused("this master keeps no copy of its board now", "copy", "")
	}
	if d.nodes.CopyOf(node) == nil {
		return own, "", "", rpc.Refused("that machine does not hold the board's copy yet", "copy", "")
	}
	own, ok := joined.OwnOf(d.cfg.Home)
	if !ok {
		return own, "", "", rpc.Refused("this master has no device of its own to join the next one with", "device", "")
	}
	var a, f sql.NullString
	if err := d.db.QueryRow("SELECT door, door_fp FROM nodes WHERE id = ?", node).Scan(&a, &f); err != nil {
		return own, "", "", fmt.Errorf("the door of node %s: %w", node, err)
	}
	if a.String == "" {
		return own, "", "", rpc.Refused("that machine has no network door to serve as master", "door", "")
	}
	return own, a.String, f.String, nil
}

// handover is node.handover: this master holds its writes, stops its work, syncs its last change into the copy and waits until the
// calling runner holds it; it answers the number the runner takes, and once the answer is sent joins it as a runner.
func (d *Daemon) handover(c *rpc.Conn, _ map[string]any) (any, error) {
	node := d.nodes.NodeOf(c.Root())
	if node == "" {
		return nil, rpc.Refused("node.handover is a runner's own call, on its link", "node", "")
	}
	own, addr, fp, err := d.canHandOver(node)
	if err != nil {
		return nil, err
	}
	if !d.master.moving.CompareAndSwap(false, true) {
		return nil, rpc.Refused("the master is already moving to another machine", "master", "")
	}
	number := d.number() + 1
	d.master.hold("the master is moving to another machine")
	d.stopWork()
	ctx, cancel := context.WithTimeout(context.Background(), runner.HandoverWait)
	defer cancel()
	go func() { // the runner hanging up ends the wait
		select {
		case <-c.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	txid, err := d.copier.Synced(ctx)
	if err == nil && !d.nodes.Copy.WaitHolds(node, txid, ctx.Done()) {
		err = errors.New("that machine did not take the board's last changes in time")
	}
	if err != nil {
		d.cfg.Log("master: the move to %s did not finish, so this master starts again: %v", node, err)
		d.master.startAgain() // the work stopped does not resume in place
		return nil, rpc.Refused("the master did not hand over: "+err.Error(), "copy", "")
	}
	m := joined.Master{Addr: addr, Fingerprint: fp, Token: own.Token, Device: own.Device, At: time.Now().Unix(), Node: node,
		Number: number}
	c.AfterReply(func() { d.becomeRunner(m) })
	d.cfg.Log("master: handing over to %s as master number %d", node, number)
	return map[string]any{"number": number}, nil
}

// outranked is node.outranked, the call of a machine of the owner that says it is master now: this master asks it, at the door its
// own board has for it, and joins it as a runner once its number is the higher. Its word alone moves nothing.
func (d *Daemon) outranked(c *rpc.Conn, _ map[string]any) (any, error) {
	device, _ := c.Caller.Info["device"].(string)
	var door runner.NodeDoor
	var fp sql.NullString
	err := d.db.QueryRow(`SELECT n.id, COALESCE(n.door, ''), n.door_fp FROM nodes n JOIN people p ON p.id = n.person
		WHERE n.device = ? AND p.role = 'owner'`, device).Scan(&door.Node, &door.Addr, &fp)
	if errors.Is(err, sql.ErrNoRows) || door.Node == d.nodes.Self {
		return nil, rpc.Refused("only another of the owner's machines says it is master now", "node", "")
	} else if err != nil {
		return nil, err
	}
	door.FP = fp.String
	if door.Addr == "" {
		return nil, rpc.Refused("this master has no door for that machine", "door", "")
	}
	c.AfterReply(func() { go d.outrankedBy([]runner.NodeDoor{door}) }) // its word alone moves nothing
	return map[string]any{}, nil
}

// TellEvery is how often a master made so while the old one was gone dials that one's door until it answers: a runner's heartbeat.
const TellEvery = runner.BeatEvery

// tellOldMaster runs while a promotion says the old master was gone: each TellEvery it dials that master's door (its own node row's,
// pinned), and once it answers with a lower number tells it it is outranked (node.outranked). An old master that never restarts (a
// laptop woken from sleep, a cut link healed) is reached so.
func (d *Daemon) tellOldMaster() {
	defer d.wg.Done()
	var said string // the last reason the old master was not told, logged when it changes
	for {
		p, ok := joined.PromotionOf(d.cfg.Home)
		if !ok || !p.Tell || p.From == "" {
			return
		}
		var err error
		if !slices.Contains(d.nodes.Linked(), p.From) {
			err = d.tellOnce(p.From)
		}
		if err == nil { // told, or linked: it joined as a runner by itself
			if err := joined.DropPromotion(d.cfg.Home); err != nil {
				d.cfg.Log("master: %v", err)
			}
			return
		}
		if err.Error() != said {
			said = err.Error()
			d.cfg.Log("master: the old master %s was not told it is outranked: %v; again every %s", p.From, err, TellEvery)
		}
		select {
		case <-d.stop:
			return
		case <-time.After(TellEvery):
		}
	}
}

// tellOnce dials old once and tells it it is outranked; nil once it was told.
func (d *Daemon) tellOnce(old string) error {
	own, ok := joined.OwnOf(d.cfg.Home)
	if !ok {
		return errors.New("this master has no device of its own to dial it with")
	}
	var addr, fp sql.NullString
	if err := d.db.QueryRow("SELECT door, door_fp FROM nodes WHERE id = ?", old).Scan(&addr, &fp); err != nil {
		return fmt.Errorf("its door: %w", err)
	} else if addr.String == "" {
		return errors.New("the board has no door for it")
	}
	mine := d.number()
	_, err := hlclient.Ask(addr.String, fp.String, own.Token, askClient, func(l *rpc.Link, says hlclient.MasterSays) error {
		if says.Node != old {
			return fmt.Errorf("%s answers as %s", addr.String, says.Node)
		}
		if !(hlclient.MasterSays{Number: mine, Node: d.nodes.Self}).Above(says.Number, says.Node) {
			return fmt.Errorf("it is master number %d, not below this one's %d", says.Number, mine)
		}
		_, err := l.Call("node.outranked", map[string]any{}, hlclient.AskWait)
		return err
	})
	if err == nil {
		d.cfg.Log("master: the old master %s was told it is outranked", old)
	}
	return err
}

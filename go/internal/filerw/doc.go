package filerw

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
)

// outboxSize is how many notices may wait for one connection; past it the connection is closed, as a slow client is.
var outboxSize = 1000

// update is one entry of a doc's log, as doc_changed and doc.pull carry it.
type update struct {
	Client  string `json:"client"`
	Changes any    `json:"changes"`
	By      string `json:"by,omitempty"`
}

// doc is a .md file held as one shared text (contract doc.*): the editors' updates are applied in the order they are taken, every editor
// is told, and the text is written back to the file, after what was changed there meanwhile is taken in.
type doc struct {
	mu             sync.Mutex
	id, agent, rel string
	crlf           bool // the file has \r\n line breaks; the text has \n
	text           []uint16
	version        int
	log            []update // the last keep updates; log[0] took the doc to version-len(log)+1
	keep           int
	pending        []changeSet
	conns          map[*rpc.Conn]*editor
	written        string // the file's text as last loaded, taken in or written: what pending applies to
	timer          *time.Timer
	closed         bool     // let go: a write or a notice no longer belongs to it
	abs            string   // the file's real path, as an agent's edit names it
	stat           fileStat // the file as last loaded or written
	watch          *time.Timer
	unseen         bool // a change was seen that no edit has named yet: it is taken in at the next look
}

// fileStat is what a look compares, one stat and no read: a file put in its place (an atomic save) is another file even with the same
// time and size.
type fileStat struct {
	mod  time.Time
	size int64
	fi   os.FileInfo
}

func (a fileStat) same(b fileStat) bool {
	return a.mod.Equal(b.mod) && a.size == b.size && a.fi != nil && b.fi != nil && os.SameFile(a.fi, b.fi)
}

func statOf(root *os.Root, rel string) (fileStat, error) {
	fi, err := root.Lstat(rel)
	if err != nil {
		return fileStat{}, err
	}
	return fileStat{fi.ModTime(), fi.Size(), fi}, nil
}

// seen records st as the file the doc holds: what a look compares with.
func (d *doc) seen(st fileStat) { d.stat, d.unseen = st, false }

func or[T int | time.Duration](v, d T) T {
	if v > 0 {
		return v
	}
	return d
}

func (e *Env) every() time.Duration      { return or(e.WatchEvery, time.Second) }
func (e *Env) writeDelay() time.Duration { return or(e.WriteDelay, 300*time.Millisecond) }
func (e *Env) retryDelay() time.Duration { return or(e.RetryDelay, 5*time.Second) }

// editor is a connection that has docs open: its notices go out in order through its own queue, so a slow one holds up nobody.
type editor struct {
	c    *rpc.Conn
	out  chan []byte
	docs map[string]bool
}

func (ed *editor) send(b []byte) {
	select {
	case ed.out <- b:
	default:
		ed.c.HangUp()
	}
}

// docs are the docs loaded, by agent and path, and the connections that have them open.
type docs struct {
	mu      sync.Mutex
	all     map[string]*doc
	editors map[*rpc.Conn]*editor
}

func docKey(agent, rel string) string { return agent + "\x00" + rel }

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func encode(s string) []uint16 { return utf16.Encode([]rune(s)) }

func decode(t []uint16) string { return string(utf16.Decode(t)) }

// notify sends method with params to every connection that has d open; the caller holds d.mu, so they go out in version order.
func (d *doc) notify(method string, params map[string]any) {
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return
	}
	for _, ed := range d.conns {
		ed.send(b)
	}
}

// docPath is the agent and .md path a doc.* call names.
func docPath(p map[string]any) (agent, rel string, err error) {
	agent, _ = p["agent"].(string)
	rel, _ = p["path"].(string)
	if agent == "" {
		return "", "", bad("agent is an agent's name", "agent")
	}
	if err := checkRel(rel); err != nil {
		return "", "", err
	}
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return "", "", bad(rel+" is not a .md file: only those open as a doc", "path")
	}
	return agent, rel, nil
}

// load reads rel as a doc's text, \r\n made \n (crlf says it was so); an error when it is not text a doc holds.
func load(root *os.Root, rel string) (s string, crlf bool, err error) {
	fi, err := root.Lstat(rel)
	if err != nil {
		return "", false, rpc.NotFoundError(rel + " is not there")
	}
	if !fi.Mode().IsRegular() {
		return "", false, bad(rel+" is not a file", "path")
	}
	if fi.Size() > MaxBytes {
		return "", false, rpc.Refused(rel+" is over 1 MiB", "too_big", "")
	}
	f, err := root.Open(rel)
	if err != nil {
		return "", false, bad("cannot read "+rel, "path")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil || len(b) > MaxBytes {
		return "", false, bad("cannot read "+rel, "path")
	}
	s, ok := text(b, false)
	crlf = strings.Contains(s, "\r\n")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if !ok || strings.Contains(s, "\r") {
		return "", false, bad(rel+" is not text with \\n or \\r\\n line breaks", "path")
	}
	return s, crlf, nil
}

// openDoc is doc.open: the loaded doc, or the file loaded as a new one.
func (e *Env) openDoc(c *rpc.Conn, p map[string]any) (any, error) {
	if _, _, err := docPath(p); err != nil {
		return nil, err
	}
	agent, rel, root, err := e.target(p)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := checkWritable(rel); err != nil {
		return nil, err
	}
	e.docs.mu.Lock()
	defer e.docs.mu.Unlock()
	if e.docs.all == nil {
		e.docs.all, e.docs.editors = map[string]*doc{}, map[*rpc.Conn]*editor{}
	}
	key := docKey(agent, rel)
	d := e.docs.all[key]
	if d == nil {
		st, _ := statOf(root, rel) // before the read: a change made in between is seen by the first look
		s, crlf, err := load(root, rel)
		if err != nil {
			return nil, err
		}
		wt, _ := e.worktree(agent)
		d = &doc{id: newID(), agent: agent, rel: rel, crlf: crlf, text: encode(s), conns: map[*rpc.Conn]*editor{}, written: s,
			abs: realPath(filepath.Join(wt, rel)), stat: st, keep: or(e.KeepUpdates, 1000)}
		d.mu.Lock()
		d.watch = time.AfterFunc(e.every(), func() { e.look(d) })
		d.mu.Unlock()
		e.docs.all[key] = d
	}
	ed := e.docs.editors[c]
	if ed == nil {
		ed = &editor{c: c, out: make(chan []byte, outboxSize), docs: map[string]bool{}}
		e.docs.editors[c] = ed
		go func() {
			for b := range ed.out {
				if !c.SendRaw(b) {
					c.HangUp()
				}
			}
		}()
		c.OnClose(func() { e.dropEditor(c) })
	}
	ed.docs[key] = true
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conns[c] = ed
	return map[string]any{"doc": d.id, "version": d.version, "text": decode(d.text)}, nil
}

// held is the loaded doc p names, locked, when c has it open and p's doc id is its; the caller unlocks it.
func (e *Env) held(c *rpc.Conn, p map[string]any) (*doc, error) {
	agent, rel, err := docPath(p)
	if err != nil {
		return nil, err
	}
	id, _ := p["doc"].(string)
	if id == "" {
		return nil, bad("doc is the id doc.open gave", "doc")
	}
	e.docs.mu.Lock()
	d := e.docs.all[docKey(agent, rel)]
	e.docs.mu.Unlock()
	if d == nil {
		return nil, rpc.NotFoundError(rel + " is not open here: doc.open it")
	}
	d.mu.Lock()
	switch {
	case d.closed || d.conns[c] == nil:
		d.mu.Unlock()
		return nil, rpc.NotFoundError(rel + " is not open here: doc.open it")
	case id != d.id:
		d.mu.Unlock()
		return nil, rpc.Refused(rel+" was loaded again: open it again", "reopen", "")
	}
	return d, nil
}

// pushDoc is doc.push: the updates, taken whole or not at all, on the current version only.
func (e *Env) pushDoc(c *rpc.Conn, p map[string]any) (any, error) {
	version, ok := count(p["version"])
	if !ok {
		return nil, bad("version is the doc's version the updates were made on", "version")
	}
	raw, _ := p["updates"].([]any)
	if len(raw) == 0 || len(raw) > 100 {
		return nil, bad("updates is 1 to 100 updates", "updates")
	}
	d, err := e.held(c, p)
	if err != nil {
		return nil, err
	}
	defer d.mu.Unlock()
	if version != d.version {
		r := rpc.Refused("the doc is at version "+strconv.Itoa(d.version)+": pull, rebase and push again", "behind", "")
		r.Data["version"] = d.version
		return nil, r
	}
	text, taken, sets := d.text, make([]update, 0, len(raw)), make([]changeSet, 0, len(raw))
	for _, u := range raw {
		m, _ := u.(map[string]any)
		client, _ := m["client"].(string)
		cs, err := parseChangeSet(m["changes"])
		if client == "" || len(client) > 100 || err != nil {
			return nil, bad("each update is {client, changes} with changes a ChangeSet's JSON", "updates")
		}
		if text, err = cs.apply(text); err != nil {
			return nil, bad("a ChangeSet does not fit the text it applies to", "updates")
		}
		taken, sets = append(taken, update{Client: client, Changes: m["changes"], By: db.Human}), append(sets, cs)
	}
	if !whole(text) {
		return nil, bad("the updates cut a character in two", "updates")
	}
	if d.size(text) > MaxBytes {
		return nil, rpc.Refused("the doc would be over 1 MiB", "too_big", "")
	}
	d.take(text, taken)
	d.pending = append(d.pending, sets...)
	e.writeIn(d, e.writeDelay())
	return map[string]any{"version": d.version}, nil
}

// size is how many bytes text takes in the file, with the line breaks the file has.
func (d *doc) size(text []uint16) int {
	n := len(decode(text))
	if d.crlf {
		n += strings.Count(decode(text), "\n")
	}
	return n
}

// writeIn has d written after delay, unless a push moves it on; the caller holds d.mu.
func (e *Env) writeIn(d *doc, delay time.Duration) {
	if d.timer == nil {
		d.timer = time.AfterFunc(delay, func() { e.flushLater(d) })
	} else {
		d.timer.Reset(delay)
	}
}

// take puts updates in d's log as the new text, and tells every editor; the caller holds d.mu.
func (d *doc) take(text []uint16, updates []update) {
	d.text = text
	d.version += len(updates)
	d.log = append(d.log, updates...)
	if over := len(d.log) - d.keep; over > 0 {
		d.log = append([]update(nil), d.log[over:]...)
	}
	d.notify("doc_changed", map[string]any{"agent": d.agent, "path": d.rel, "doc": d.id, "version": d.version, "updates": updates})
}

// pullDoc is doc.pull: the updates after version.
func (e *Env) pullDoc(c *rpc.Conn, p map[string]any) (any, error) {
	version, ok := count(p["version"])
	if !ok {
		return nil, bad("version is the version the caller has", "version")
	}
	d, err := e.held(c, p)
	if err != nil {
		return nil, err
	}
	defer d.mu.Unlock()
	if version > d.version {
		return nil, bad("version is above the doc's "+strconv.Itoa(d.version), "version")
	}
	first := d.version - len(d.log)
	if version < first {
		return nil, rpc.Refused("updates before version "+strconv.Itoa(first)+" are not kept: open the doc again", "reopen", "")
	}
	return map[string]any{"version": d.version, "updates": append([]update{}, d.log[version-first:]...)}, nil // [] when none, never null
}

// closeDocRPC is doc.close.
func (e *Env) closeDocRPC(c *rpc.Conn, p map[string]any) (any, error) {
	agent, rel, err := docPath(p)
	if err != nil {
		return nil, err
	}
	e.docs.mu.Lock()
	defer e.docs.mu.Unlock()
	e.closeLocked(c, docKey(agent, rel))
	return map[string]any{}, nil
}

// dropEditor closes every doc c has open: its connection ended.
func (e *Env) dropEditor(c *rpc.Conn) {
	e.docs.mu.Lock()
	defer e.docs.mu.Unlock()
	ed := e.docs.editors[c]
	if ed == nil {
		return
	}
	for key := range ed.docs {
		e.closeLocked(c, key)
	}
	delete(e.docs.editors, c)
	close(ed.out) // no doc holds ed any more, so nothing sends on it
}

// closeLocked takes c off the doc at key; the last one off writes what is not written and lets the doc go. The caller holds docs.mu,
// so a doc opened again after the last close loads what was written.
func (e *Env) closeLocked(c *rpc.Conn, key string) {
	if ed := e.docs.editors[c]; ed != nil {
		delete(ed.docs, key)
	}
	d := e.docs.all[key]
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.conns, c)
	if len(d.conns) > 0 {
		return
	}
	switch reason, done := e.flushLocked(d); {
	case reason != "":
		e.letGo(d, reason)
	case done:
		if d.timer != nil {
			d.timer.Stop()
		}
		e.letGo(d, "")
	default: // not written: the doc stays loaded, and is opened again as it is, until the write is made
		e.writeIn(d, e.retryDelay())
	}
}

// letGo takes d out of the loaded docs, telling its editors why when there is a reason; the caller holds docs.mu and d.mu.
func (e *Env) letGo(d *doc, reason string) {
	if reason != "" {
		d.notify("doc_closed", map[string]any{"agent": d.agent, "path": d.rel, "doc": d.id, "reason": reason})
	}
	key := docKey(d.agent, d.rel)
	for c := range d.conns {
		if ed := e.docs.editors[c]; ed != nil {
			delete(ed.docs, key)
		}
	}
	d.conns, d.closed = map[*rpc.Conn]*editor{}, true
	d.watch.Stop()
	if e.docs.all[key] == d {
		delete(e.docs.all, key)
	}
}

// flushLater is the write once the pushes have paused, or again after one failed. A file found gone or unreadable lets the doc go, and
// so does a write made for a doc no editor has open any more.
func (e *Env) flushLater(d *doc) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	reason, done := e.flushLocked(d)
	if !done && reason == "" {
		e.writeIn(d, e.retryDelay())
	}
	d.mu.Unlock()
	if reason == "" && !done {
		return
	}
	e.docs.mu.Lock()
	defer e.docs.mu.Unlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed && (reason != "" || len(d.conns) == 0 && decode(d.text) == d.written) {
		e.letGo(d, reason)
	}
}

// flushLocked writes d's text to its file. It reads the file first: a change there it did not write is taken in (takeIn), so the write
// keeps it. done: the file holds the doc's text. reason: gone or unreadable, when the file is so; nothing is written then. The caller
// holds d.mu.
func (e *Env) flushLocked(d *doc) (reason string, done bool) {
	_, rel, root, err := e.target(map[string]any{"agent": d.agent, "path": d.rel})
	if err != nil {
		e.logf("  doc: cannot write %s of %s: %v", d.rel, d.agent, err)
		return "", false
	}
	defer root.Close()
	e.lock.Lock()
	defer e.lock.Unlock()
	disk, _, err := load(root, rel)
	if rerr, ok := err.(*rpc.Error); ok && rerr.Code == "not_found" {
		return "gone", false
	} else if err != nil {
		return "unreadable", false
	}
	s := decode(d.text)
	switch {
	case disk == s: // the file already holds it, whoever wrote it there
		d.written, d.pending = disk, nil
		return "", true
	case disk != d.written:
		if !d.takeIn(disk, e.touchedBy(d.abs, true)) {
			e.logf("  doc: %s of %s changed outside the doc and could not be taken in: not written", d.rel, d.agent)
			return "", false
		}
		s = decode(d.text)
	}
	if s == d.written {
		return "", true
	}
	fi, err := root.Lstat(rel)
	if err != nil {
		return "gone", false
	}
	out := s
	if d.crlf {
		out = strings.ReplaceAll(s, "\n", "\r\n")
	}
	if err := replace(root, rel, out, fi.Mode().Perm()); err != nil {
		e.logf("  doc: cannot write %s of %s: %v", d.rel, d.agent, err)
		return "", false
	}
	d.written, d.pending = s, nil
	if st, err := statOf(root, rel); err == nil {
		d.seen(st)
	}
	return "", true
}

// look is the watch on a loaded doc's file: a change made there is taken in without waiting for a push, and a file gone or unreadable
// lets the doc go.
func (e *Env) look(d *doc) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	reason := e.lookLocked(d)
	d.watch.Reset(e.every())
	d.mu.Unlock()
	if reason == "" {
		return
	}
	e.docs.mu.Lock()
	defer e.docs.mu.Unlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		e.letGo(d, reason)
	}
}

// lookLocked takes in a change made to d's file since it was last loaded or written. A change no agent's edit has named yet waits one
// look, as the edit's hook may come just after the file changed. The caller holds d.mu.
func (e *Env) lookLocked(d *doc) (reason string) {
	_, rel, root, err := e.target(map[string]any{"agent": d.agent, "path": d.rel})
	if rerr, ok := err.(*rpc.Error); ok && rerr.Code == "not_found" { // the worktree is gone
		return "gone"
	} else if err != nil {
		return ""
	}
	defer root.Close()
	st, err := statOf(root, rel)
	if err != nil {
		return "gone"
	}
	if st.same(d.stat) {
		return ""
	}
	by := e.touchedBy(d.abs, false)
	if by == "" && !d.unseen {
		d.unseen = true
		return ""
	}
	disk, _, err := load(root, rel)
	if rerr, ok := err.(*rpc.Error); ok && rerr.Code == "not_found" {
		return "gone"
	} else if err != nil {
		return "unreadable"
	}
	d.seen(st)
	switch {
	case disk == decode(d.text):
		d.written, d.pending = disk, nil
	case disk == d.written:
	case d.takeIn(disk, by):
		e.touchedBy(d.abs, true) // the edit is named once
	default:
		e.logf("  doc: %s of %s changed outside the doc and could not be taken in", d.rel, d.agent)
	}
	return ""
}

// takeIn puts what the file now holds into the doc as one update of client disk: the change from what was last written to disk,
// carried over the pushes made since; false when it does not fit, and nothing is taken. The caller holds d.mu.
func (d *doc) takeIn(disk, by string) bool {
	js, cs := changeSetJSON(rebase(diffLines(encode(d.written), encode(disk)), d.pending), len(d.text))
	text, err := cs.apply(d.text)
	if err != nil {
		return false
	}
	d.take(text, []update{{Client: "disk", Changes: js, By: by}})
	_, since := changeSetJSON(diffLines(encode(disk), text), len(encode(disk))) // the pushes, now on disk's text
	d.written, d.pending = disk, []changeSet{since}
	return true
}

func (e *Env) logf(format string, args ...any) {
	if e.Log != nil {
		e.Log(format, args...)
	}
}

// whole is whether t has no surrogate half without its other half: a ChangeSet that cut a character in two is refused.
func whole(t []uint16) bool {
	for i := 0; i < len(t); i++ {
		switch {
		case t[i] >= 0xD800 && t[i] < 0xDC00:
			if i+1 == len(t) || t[i+1] < 0xDC00 || t[i+1] >= 0xE000 {
				return false
			}
			i++
		case t[i] >= 0xDC00 && t[i] < 0xE000:
			return false
		}
	}
	return true
}

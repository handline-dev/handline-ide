// Package holder keeps an agent CLI that speaks lines on stdin and stdout alive apart from the program that reads it: it owns the CLI's
// pipes, keeps each output line until a reader acknowledges it, and serves them on a unix socket, so the reader can die, come back and
// go on from its cursor. It knows no CLI's protocol: what it must not keep is named by the caller as a substring.
package holder

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Config is one held session.
type Config struct {
	Argv []string
	Env  []string
	Dir  string
	// Socket is the unix socket readers and writers connect to.
	Socket string
	// Init are written to the CLI first, in order. The first output line holding Secret is never kept: a Ready record takes its place.
	Init   [][]byte
	Secret string
	// MaxBytes bounds the unread lines kept; the oldest go first and a Gap record says how many.
	MaxBytes int
	// Linger is how long the holder waits, once the CLI has exited, for a reader to take what is unread.
	Linger time.Duration
	// Status is where the holder writes its one line of status (the terminal it runs in).
	Status io.Writer
}

// Record is one line the holder serves: a CLI line (Line) or one of its own (Holder: "ready", "gap", "exit").
type Record struct {
	Seq    int64           `json:"seq"`
	Line   json.RawMessage `json:"line,omitempty"`
	Holder string          `json:"holder,omitempty"`
	Lost   int64           `json:"lost,omitempty"`
	Code   int             `json:"code,omitempty"`
}

// Request is the first line a client sends: Read (from the record after this seq, then acks on the same connection), Write (one
// line to the CLI's stdin), or Close (end the CLI's stdin).
type Request struct {
	Read  *int64  `json:"read,omitempty"`
	Ack   int64   `json:"ack,omitempty"`
	Write *string `json:"write,omitempty"`
	Close bool    `json:"close,omitempty"`
}

// Reply answers a Write or a Close.
type Reply struct {
	Err string `json:"err,omitempty"`
}

type holder struct {
	cfg Config

	mu      sync.Mutex
	stdin   io.WriteCloser
	recs    []Record
	size    int
	seq     int64
	acked   int64
	lost    int64
	exited  bool
	wake    chan struct{}
	reader  net.Conn
	secret  bool // the secret line was dropped already
	writeMu sync.Mutex
}

// Run starts the CLI and holds it until it has exited and a reader has taken every record (or Linger has run). It returns the CLI's
// exit code.
func Run(ctx context.Context, cfg Config) (int, error) {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 32 << 20
	}
	if cfg.Status == nil {
		cfg.Status = io.Discard
	}
	if len(cfg.Argv) == 0 {
		return -1, errors.New("no command to hold")
	}
	if c, err := net.DialTimeout("unix", cfg.Socket, time.Second); err == nil {
		c.Close()
		return -1, errors.New("another holder serves " + cfg.Socket)
	}
	_ = os.Remove(cfg.Socket)
	ln, err := net.Listen("unix", cfg.Socket)
	if err != nil {
		return -1, err
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	mine, _ := os.Stat(cfg.Socket)
	_ = os.WriteFile(PidFile(cfg.Socket), []byte(strconv.Itoa(os.Getpid())), 0o600)
	defer func() { // what is at the path now may be a newer holder's: only our own goes
		if now, err := os.Stat(cfg.Socket); err == nil && mine != nil && os.SameFile(mine, now) {
			os.Remove(cfg.Socket)
			os.Remove(PidFile(cfg.Socket))
		}
	}()
	defer ln.Close()

	cmd := exec.CommandContext(ctx, cfg.Argv[0], cfg.Argv[1:]...)
	cmd.Env, cmd.Dir, cmd.Stderr = cfg.Env, cfg.Dir, cfg.Status
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return -1, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err
	}
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	h := &holder{cfg: cfg, stdin: stdin, wake: make(chan struct{})}
	go h.serve(ln)
	for _, l := range cfg.Init {
		_ = h.write(l)
	}
	h.pump(stdout)
	code := 0
	if err := cmd.Wait(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return -1, err
		}
		code = ee.ExitCode()
	}
	h.mu.Lock()
	h.exited = true
	h.add(Record{Holder: "exit", Code: code})
	h.mu.Unlock()
	h.linger(ctx)
	return code, nil
}

// maxLine is the longest output line kept; a longer one is dropped whole and a gap says so, and reading goes on.
const maxLine = 64 << 20

func (h *holder) pump(r io.Reader) {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, long, err := readLine(br)
		if long {
			h.mu.Lock()
			h.add(Record{Holder: "gap", Lost: 1})
			h.mu.Unlock()
		} else if len(line) > 0 || err == nil {
			h.keep(line)
		}
		if err != nil {
			return
		}
	}
}

// readLine is the next line without its newline; long when it ran past maxLine, which is then read to its end and dropped.
func readLine(br *bufio.Reader) (line []byte, long bool, err error) {
	for {
		chunk, err := br.ReadSlice('\n')
		if !long {
			line = append(line, chunk...)
			if len(line) > maxLine {
				line, long = nil, true
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return bytes.TrimSuffix(line, []byte("\n")), long, err
	}
}

// keep adds one output line; the first holding Secret becomes Ready instead.
func (h *holder) keep(line []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case !h.secret && h.cfg.Secret != "" && bytes.Contains(line, []byte(h.cfg.Secret)):
		h.secret = true
		h.add(Record{Holder: "ready"})
	case json.Valid(line):
		h.add(Record{Line: line})
	default:
		q, _ := json.Marshal(string(line))
		h.add(Record{Line: q})
	}
}

// PidFile names the holder's process beside its socket, so a socket a killed holder left is known as dead.
func PidFile(socket string) string { return socket + ".pid" }

// Alive is whether a holder serves at socket: its socket is there and the process its pid file names runs.
func Alive(socket string) bool {
	fi, err := os.Stat(socket)
	if err != nil || fi.Mode()&os.ModeSocket == 0 {
		return false
	}
	b, err := os.ReadFile(PidFile(socket))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil || pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// add keeps r under the next seq and wakes the reader; the caller holds mu.
func (h *holder) add(r Record) {
	h.seq++
	r.Seq = h.seq
	h.recs = append(h.recs, r)
	h.size += len(r.Line) + 64
	for h.size > h.cfg.MaxBytes && len(h.recs) > 1 {
		h.size -= len(h.recs[0].Line) + 64
		h.recs = h.recs[1:]
		h.lost++
	}
	close(h.wake)
	h.wake = make(chan struct{})
}

func (h *holder) linger(ctx context.Context) {
	t := time.NewTimer(h.cfg.Linger)
	defer t.Stop()
	for {
		h.mu.Lock()
		done := h.acked >= h.seq
		w := h.wake
		h.mu.Unlock()
		if done {
			return
		}
		select {
		case <-w:
		case <-t.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (h *holder) serve(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go h.client(c)
	}
}

func (h *holder) client(c net.Conn) {
	br := bufio.NewReader(c)
	var req Request
	line, err := br.ReadBytes('\n')
	if err != nil || json.Unmarshal(line, &req) != nil {
		c.Close()
		return
	}
	switch {
	case req.Read != nil:
		h.read(c, br, *req.Read)
	case req.Write != nil:
		h.answer(c, h.write([]byte(*req.Write)))
	case req.Close:
		h.writeMu.Lock()
		err := h.stdin.Close()
		h.writeMu.Unlock()
		h.answer(c, err)
	default:
		c.Close()
	}
}

func (h *holder) answer(c net.Conn, err error) {
	defer c.Close()
	var r Reply
	if err != nil {
		r.Err = err.Error()
	}
	b, _ := json.Marshal(r)
	_, _ = c.Write(append(b, '\n'))
}

func (h *holder) write(line []byte) error {
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	if _, err := h.stdin.Write(append(append([]byte(nil), line...), '\n')); err != nil {
		return err
	}
	return nil
}

// read serves records after from to c, then each new one, until c fails. A new reader takes the place of the one before. Acks read
// from c drop what they name.
func (h *holder) read(c net.Conn, br *bufio.Reader, from int64) {
	h.mu.Lock()
	if h.reader != nil {
		h.reader.Close()
	}
	h.reader = c
	h.mu.Unlock()
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		for {
			line, err := br.ReadBytes('\n')
			if err != nil {
				c.Close()
				return
			}
			var r Request
			if json.Unmarshal(line, &r) == nil && r.Ack > 0 {
				h.ack(r.Ack)
			}
		}
	}()
	defer c.Close()
	sent := from
	gapTold := false
	for {
		h.mu.Lock()
		var out []Record
		if sent < h.acked {
			sent = h.acked
		}
		if !gapTold && h.lost > 0 && len(h.recs) > 0 && h.recs[0].Seq > sent+1 {
			out = append(out, Record{Seq: sent, Holder: "gap", Lost: h.recs[0].Seq - sent - 1})
			gapTold = true
		}
		for _, r := range h.recs {
			if r.Seq > sent {
				out = append(out, r)
			}
		}
		w := h.wake
		mine := h.reader == c
		h.mu.Unlock()
		if !mine {
			return
		}
		for _, r := range out {
			b, _ := json.Marshal(r)
			if _, err := c.Write(append(b, '\n')); err != nil {
				return
			}
			if r.Seq > sent {
				sent = r.Seq
			}
		}
		select {
		case <-w:
		case <-gone:
			return
		}
	}
}

func (h *holder) ack(n int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n > h.seq {
		n = h.seq
	}
	if n <= h.acked {
		return
	}
	h.acked = n
	i := 0
	for i < len(h.recs) && h.recs[i].Seq <= n {
		h.size -= len(h.recs[i].Line) + 64
		i++
	}
	h.recs = h.recs[i:]
	close(h.wake)
	h.wake = make(chan struct{})
}

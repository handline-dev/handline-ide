package procload

import (
	"cmp"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ListenEvery is how often the machine's listening sockets are read (a multiple of Every).
const ListenEvery = 15

// Listener is one TCP socket a process listens on.
type Listener struct {
	PID  int
	Addr string // 127.0.0.1, ::1, 0.0.0.0 (any IPv4), :: (any IPv6) or an interface's address
	Port int
}

// Server is a Listener of an agent's process, with the command and start of the process.
type Server struct {
	PID     int
	Port    int
	Addr    string
	Command string
	Since   int64 // epoch seconds the process began
}

// sessionRoots are the executables of an agent's own session, whose own sockets and those of its direct children that are not a shell (the
// MCP and LSP servers it spawns) are not servers the agent started: a server it starts goes through its Bash tool's shell.
var sessionRoots = map[string]bool{"claude": true, "codex": true}

var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true, "ksh": true, "tcsh": true, "csh": true}

// lsofListeners reads every listening TCP socket in one lsof call.
func lsofListeners() ([]Listener, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-F", "pPtn").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 && len(out) == 0 { // lsof exits 1 when it lists nothing
			return []Listener{}, nil
		}
		return nil, err
	}
	return parseLsof(string(out)), nil
}

// parseLsof reads `lsof -F pPtn`: p starts a process, then each socket gives its type (IPv4 or IPv6) and name (host:port, "*" for any).
func parseLsof(out string) []Listener {
	var res []Listener
	pid, v6 := 0, false
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 't':
			v6 = line[1:] == "IPv6"
		case 'n':
			i := strings.LastIndexByte(line, ':')
			if i < 2 || pid == 0 {
				continue
			}
			port, err := strconv.Atoi(line[i+1:])
			if err != nil || port < 1 || port > 65535 {
				continue
			}
			addr := strings.Trim(line[1:i], "[]")
			if addr == "*" {
				addr = "0.0.0.0"
				if v6 {
					addr = "::"
				}
			}
			res = append(res, Listener{PID: pid, Addr: addr, Port: port})
		}
	}
	return res
}

// addrRank orders the addresses one process listens on for a port: the one a client opens first.
func addrRank(a string) int {
	switch a {
	case "127.0.0.1":
		return 0
	case "0.0.0.0":
		return 1
	case "::1":
		return 2
	case "::":
		return 3
	}
	return 4
}

// serversOf splits the listeners by the agent that owns their process, leaving out those of the session itself, of the daemon and of the
// panes, and those whose process is not in the table.
func serversOf(listeners []Listener, procs []Proc, panes map[string]int, self int) map[string][]Server {
	byPID := make(map[int]Proc, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	paneRoot := map[int]bool{}
	for _, pid := range panes {
		paneRoot[pid] = true
	}
	sessionOf := func(p Proc) bool { // the agent's session process, or a direct child of it that is not a shell
		if base := exeName(p.Cmd); sessionRoots[base] {
			return true
		}
		up, ok := byPID[p.PPID]
		return ok && up.Agent == p.Agent && sessionRoots[exeName(up.Cmd)] && !shells[exeName(p.Cmd)]
	}
	best := map[[2]int]Listener{}
	for _, l := range listeners {
		p, ok := byPID[l.PID]
		if !ok || p.Agent == "" || p.Gone || l.PID == self || paneRoot[l.PID] || sessionOf(p) {
			continue
		}
		k := [2]int{l.PID, l.Port}
		if cur, has := best[k]; !has || addrRank(l.Addr) < addrRank(cur.Addr) || addrRank(l.Addr) == addrRank(cur.Addr) && l.Addr < cur.Addr {
			best[k] = l
		}
	}
	out := map[string][]Server{}
	for _, l := range best {
		p := byPID[l.PID]
		out[p.Agent] = append(out[p.Agent], Server{PID: l.PID, Port: l.Port, Addr: l.Addr, Command: p.Cmd, Since: p.Start})
	}
	for _, s := range out {
		slices.SortFunc(s, func(a, b Server) int { return cmp.Or(cmp.Compare(a.Port, b.Port), cmp.Compare(a.PID, b.PID)) })
	}
	return out
}

// exeName is the executable's file name in a command line (the first word; a path with spaces is not told apart).
func exeName(cmd string) string {
	first, _, _ := strings.Cut(cmd, " ")
	return filepath.Base(first)
}

// ServersOf is the servers the agent's processes listen on as of the last read of the sockets, lowest port first.
func (t *Tracker) ServersOf(agent string) []Server {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Server(nil), t.servers[agent]...)
}

// listenDue is whether ListenEvery has passed since the last read of the sockets.
func (t *Tracker) listenDue(now int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.listenAt == 0 || now-t.listenAt >= ListenEvery
}

// readListeners reads the sockets; nil when it failed, which keeps the last read.
func (t *Tracker) readListeners() []Listener {
	read := t.Listen
	if read == nil {
		read = lsofListeners
	}
	ls, err := read()
	if err != nil {
		if t.Log != nil {
			t.Log("procload: listeners: " + err.Error())
		}
		return nil
	}
	if ls == nil {
		ls = []Listener{}
	}
	return ls
}

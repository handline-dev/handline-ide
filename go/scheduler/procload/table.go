package procload

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Raw is one row of the process table.
type Raw struct {
	PID, PPID int
	RSS       int64   // resident bytes
	CPUSecs   float64 // CPU time used since the process began
	Start     int64   // epoch seconds the process began
	Cmd       string
}

// cmdMax is how many characters of a command a process keeps.
const cmdMax = 120

// psTable reads every process of this machine in one ps call.
func psTable() ([]Raw, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,rss=,time=,lstart=,command=").Output()
	if err != nil {
		return nil, err
	}
	return parseTable(string(out), time.Local), nil
}

// parseTable reads `ps -axo pid=,ppid=,rss=,time=,lstart=,command=`; a line that does not parse is skipped.
func parseTable(out string, loc *time.Location) []Raw {
	var rows []Raw
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 { // pid ppid rss time, five words of lstart, a command
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		ppid, e2 := strconv.Atoi(f[1])
		rss, e3 := strconv.ParseInt(f[2], 10, 64)
		cpu, ok := parseCPUTime(f[3])
		start, e5 := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(f[4:9], " "), loc)
		if e1 != nil || e2 != nil || e3 != nil || !ok || e5 != nil {
			continue
		}
		cmd := strings.Join(f[9:], " ")
		if len(cmd) > cmdMax {
			cmd = cmd[:cmdMax]
		}
		rows = append(rows, Raw{PID: pid, PPID: ppid, RSS: rss * 1024, CPUSecs: cpu, Start: start.Unix(), Cmd: cmd})
	}
	return rows
}

// parseCPUTime reads ps's TIME: [[dd-]hh:]mm:ss[.ss].
func parseCPUTime(s string) (float64, bool) {
	days := 0.0
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, false
		}
		days, s = float64(n), rest
	}
	total := 0.0
	for _, part := range strings.Split(s, ":") {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0, false
		}
		total = total*60 + v
	}
	return total + days*86400, true
}

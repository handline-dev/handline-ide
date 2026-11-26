package runload

import (
	"strings"

	"github.com/handline/handline/go/scheduler/procload"
)

// procKey names one process: a pid is used again, a pid and its start is not.
type procKey struct {
	pid   int
	start int64
}

// toolShell is whether a process is the shell Claude Code starts for a Bash call: it sources the session's shell snapshot first.
func toolShell(cmd string) bool {
	return strings.Contains(cmd, " -c ") && strings.Contains(cmd, "shell-snapshots/snapshot-")
}

// matchWithin is how long after its hook a call's shell may start and still be its own, in seconds: a busy machine starts it late.
// Two calls of one agent closer together than this are not told apart, and neither is paired.
const matchWithin = 20

// Found is which process each run's shell is, by the run's ID, kept from look to look.
type Found map[string]procKey

// Match gives each run the processes it runs as, from one reading of the process table, and says which runs have ended without a
// hook telling: those whose shell was found on an earlier look and is gone now. A run's shell is a tool shell of its agent that
// started within matchWithin after the run did, and only when the pairing cannot be another: the one free shell in the run's
// window, in no other unfound run's window. A copy a tool shell forked of itself is not a candidate. A run whose shell is not found
// is left as it is, and counts as running. heavy are the processes of the heavy runs, for the caller to lower.
func Match(runs []Run, procs []procload.Proc, found Found) (ended map[string]bool, heavy []procKey) {
	alive := map[procKey]procload.Proc{}
	byPID := map[int]procload.Proc{}
	kids := map[int][]procload.Proc{}
	for _, p := range procs {
		if p.Gone {
			continue
		}
		alive[procKey{p.PID, p.Start}] = p
		byPID[p.PID] = p
		kids[p.PPID] = append(kids[p.PPID], p)
	}
	live := map[string]bool{}
	for _, r := range runs {
		live[r.ID] = true
	}
	taken := map[procKey]bool{}
	for id, k := range found {
		if !live[id] {
			delete(found, id)
			continue
		}
		taken[k] = true
	}
	ended = map[string]bool{}
	for _, r := range runs {
		if k, ok := found[r.ID]; ok {
			if _, there := alive[k]; !there {
				ended[r.ID] = true
				delete(found, r.ID)
			}
		}
	}
	// The free tool shells of each agent, and for each the unfound runs whose window it starts in.
	inWindow := func(r Run, p procload.Proc) bool { return p.Start >= r.Started-2 && p.Start <= r.Started+matchWithin }
	wanted := map[procKey][]string{}
	mine := map[string][]procKey{}
	for key, p := range alive {
		if taken[key] || !toolShell(p.Cmd) {
			continue
		}
		if parent, ok := byPID[p.PPID]; ok && toolShell(parent.Cmd) {
			continue
		}
		for _, r := range runs {
			if _, has := found[r.ID]; has || ended[r.ID] || p.Agent != r.Agent || !inWindow(r, p) {
				continue
			}
			wanted[key] = append(wanted[key], r.ID)
			mine[r.ID] = append(mine[r.ID], key)
		}
	}
	for id, keys := range mine {
		if len(keys) == 1 && len(wanted[keys[0]]) == 1 {
			found[id] = keys[0]
		}
	}
	for i := range runs {
		r := &runs[i]
		k, ok := found[r.ID]
		if !ok {
			continue
		}
		r.Found = true
		for stack, seen := []procload.Proc{alive[k]}, map[int]bool{}; len(stack) > 0; {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[p.PID] {
				continue
			}
			seen[p.PID] = true
			r.CPU, r.RSS, r.Procs = r.CPU+p.CPU, r.RSS+p.RSS, r.Procs+1
			if r.Heavy {
				heavy = append(heavy, procKey{p.PID, p.Start})
			}
			stack = append(stack, kids[p.PID]...)
		}
	}
	return ended, heavy
}

package sessionrpc

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/protocol/rpc"
	"github.com/handline/handline/go/scheduler/procload"
)

// RunningJob is one job the agent's last Stop listed: its id, type ("shell", ...) and name.
type RunningJob struct{ ID, Type, Name string }

// OutputLines is how many of a shell job's last output lines agent.jobs gives, each cut to OutputWidth.
const OutputLines, OutputWidth = 5, 200

// stopAllKeys is Ctrl+X Ctrl+K, Claude Code's stop for its background work; pressed twice, the second confirms it.
const stopAllKeys = "\x18\x0b"

func registerJobs(s *rpc.Server, e *Env) {
	s.Register("agent.jobs", rpc.Method{Handler: e.agentJobs, Read: true})
	s.Register("agent.job.stop", rpc.Method{Handler: e.stopJob})
	s.Register("agent.jobs.stop", rpc.Method{Handler: e.stopJobs})
}

// shellJob is a listed job with what agent.jobs and agent.job.stop read of it: its start, and a shell's output file.
type shellJob struct {
	RunningJob
	started int64
	output  string // "" for no shell, or one whose file is not recorded
}

// jobs is what the agent's last Stop listed, less the shells no process writes to any more (one stopped, or ended since).
func (e *Env) jobs(agent string) ([]shellJob, error) {
	from, _, err := db.LastEventOfTS(e.DB, agent, []string{"background_work_ended", "SessionStart"})
	if err != nil {
		return nil, err
	}
	var all []shellJob
	var files []string
	for _, j := range e.running(agent) {
		s := shellJob{RunningJob: j, started: db.BgJobStarted(e.DB, agent, from, j.ID)}
		if j.Type == "shell" {
			if s.output = db.BgJobOutput(e.DB, agent, from, j.ID); s.output != "" {
				files = append(files, s.output)
			}
		}
		all = append(all, s)
	}
	open, known := holders(files)
	out := all[:0]
	for _, s := range all {
		if s.output == "" || !known || len(open[s.output]) > 0 {
			out = append(out, s)
		}
	}
	return out, nil
}

func (e *Env) agentJobs(_ *rpc.Conn, params map[string]any) (any, error) {
	agent, err := e.agent(params)
	if err != nil {
		return nil, err
	}
	listed, err := e.jobs(agent)
	if err != nil {
		return nil, err
	}
	rows := []any{}
	for _, j := range listed {
		row := map[string]any{"id": j.ID, "type": j.Type, "name": j.Name, "started": nil, "output": nil}
		if j.started > 0 {
			row["started"] = j.started
		}
		if j.Type == "shell" {
			row["output"] = tailLines(j.output, OutputLines)
		}
		rows = append(rows, row)
	}
	return map[string]any{"jobs": rows}, nil
}

// tailLines is the last n lines of the file at path that are not blank, each cut to OutputWidth; none for a file it cannot read.
func tailLines(path string, n int) []string {
	out := []string{}
	if path == "" {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return out
	}
	start := max(0, end-8192)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return out
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return out
	}
	lines := strings.Split(strings.ToValidUTF8(string(b), "�"), "\n")
	if start > 0 { // the window began inside a line
		lines = lines[1:]
	}
	for _, l := range lines {
		if l = strings.TrimRight(l, "\r"); strings.TrimSpace(l) != "" {
			out = append(out, truncate(l, OutputWidth))
		}
	}
	return out[max(0, len(out)-n):]
}

// stopJob ends one background shell of the agent's: the process group of its own, under the agent's CLI, that writes the job's
// output file. Its gates are the typing calls': the terminal lock and someone else's input hold.
func (e *Env) stopJob(c *rpc.Conn, params map[string]any) (any, error) {
	agent, err := e.agent(params)
	if err != nil {
		return nil, err
	}
	id, _ := params["job"].(string)
	if id == "" {
		return nil, bad("job is the id agent.jobs gave", "job")
	}
	listed, err := e.jobs(agent)
	if err != nil {
		return nil, err
	}
	var job *shellJob
	for _, j := range listed {
		if j.ID == id {
			job = &j
			break
		}
	}
	switch {
	case job == nil:
		return nil, rpc.NotFoundError(agent + " runs no background job " + id)
	case job.Type != "shell":
		return nil, rpc.Refused(job.Name+" is no shell: only agent.jobs.stop stops it", "type", "")
	case job.output == "":
		return nil, rpc.Refused(job.Name+": its output file is not recorded, so its process cannot be told apart; agent.jobs.stop stops all", "state", "")
	}
	output, name := job.output, job.Name
	return e.act(c, "agent.job.stop", params, false, func(handle string) (bool, string) {
		pane := e.Term.PanePid(handle)
		if pane == 0 {
			return false, "its terminal has no process this machine can read"
		}
		procs, err := processTable()
		if err != nil {
			return false, err.Error()
		}
		open, known := holders([]string{output})
		if !known {
			return false, "the processes writing " + name + "'s output cannot be read here"
		}
		group, err := jobGroup(procs, pane, open[output])
		if err == nil && !leads(group) { // it ended since the table was read, and its pid may be another's now
			err = errNoProcess
		}
		if err == nil {
			err = syscall.Kill(-group, syscall.SIGTERM)
		}
		if err != nil && !errors.Is(err, syscall.ESRCH) {
			return false, name + ": " + err.Error()
		}
		return true, ""
	})
}

// JobsCPU is the CPU time each listed shell job has used so far, in seconds by its id: the processes of the group agent.job.stop would
// end, with their ended children, summed. A job whose group is not found (its output file not recorded, nothing writes it, or not under pane, the agent's terminal)
// is left out, as is every job when ps or lsof cannot be read.
func JobsCPU(q db.Q, agent string, pane int, jobs []RunningJob) map[string]float64 {
	out := map[string]float64{}
	from, _, err := db.LastEventOfTS(q, agent, []string{"background_work_ended", "SessionStart"})
	if pane == 0 || err != nil {
		return out
	}
	outputs := map[string]string{}
	var files []string
	for _, j := range jobs {
		if j.Type != "shell" {
			continue
		}
		if f := db.BgJobOutput(q, agent, from, j.ID); f != "" {
			outputs[j.ID], files = f, append(files, f)
		}
	}
	if len(files) == 0 {
		return out
	}
	open, known := holders(files)
	if !known {
		return out
	}
	procs, err := processTable()
	if err != nil {
		return out
	}
	for id, f := range outputs {
		if group, err := jobGroup(procs, pane, open[f]); err == nil {
			out[id] = groupCPU(procs, group)
		}
	}
	return out
}

// groupCPU is the CPU time of the processes of group, summed.
func groupCPU(procs []proc, group int) float64 {
	sum := 0.0
	for _, p := range procs {
		if p.pgid == group {
			sum += p.cpu
		}
	}
	return sum
}

// leads is whether pid still leads its process group.
func leads(pid int) bool {
	g, err := syscall.Getpgid(pid)
	return err == nil && g == pid
}

// stopJobs asks Claude Code to stop all of the agent's background work: Ctrl+X Ctrl+K, twice, as its confirm takes it. Only while
// something runs and nothing covers the prompt: unclaimed, Ctrl+K would cut the person's draft.
func (e *Env) stopJobs(c *rpc.Conn, params map[string]any) (any, error) {
	agent, err := e.agent(params)
	if err != nil {
		return nil, err
	}
	if len(e.running(agent)) == 0 {
		return nil, rpc.Refused(agent+" runs nothing in the background", "state", "")
	}
	return e.act(c, "agent.jobs.stop", params, true, func(handle string) (bool, string) {
		if e.Term.Headless(handle) { // a Chat session stops its work on its stream, each piece by its id
			var ids []string
			for _, j := range e.running(agent) {
				ids = append(ids, j.ID)
			}
			switch n, err := e.Term.StopBackground(handle, ids); {
			case err != nil:
				return false, err.Error()
			case n == 0:
				return false, "its stream lists no background work"
			}
			return true, ""
		}
		if why := e.Term.ShellLeft(handle); why != "" {
			return false, why
		}
		for range 2 {
			if ok, out := e.Term.Send(handle, stopAllKeys, false); !ok {
				return false, truncate(out, 200)
			}
		}
		return true, ""
	})
}

// running is what the agent's last Stop listed; none without Running.
func (e *Env) running(agent string) []RunningJob {
	if e.Running == nil {
		return nil
	}
	return e.Running(agent)
}

// toolTimeout bounds a ps or lsof run: the background check calls them on the tick.
const toolTimeout = 10 * time.Second

// proc is one row of the process table: cpu is the CPU time it and its ended children have used, in seconds.
type proc struct {
	pid, ppid, pgid int
	cpu             float64
}

// processTable is every process of this machine.
func processTable() ([]proc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-axS", "-o", "pid=,ppid=,pgid=,time=").Output()
	if err != nil {
		return nil, err
	}
	var procs []proc
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 4 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		ppid, e2 := strconv.Atoi(f[1])
		pgid, e3 := strconv.Atoi(f[2])
		cpu, _ := procload.ParseCPUTime(f[3]) // a time that does not read is 0: the row still places its process
		if e1 == nil && e2 == nil && e3 == nil {
			procs = append(procs, proc{pid, ppid, pgid, cpu})
		}
	}
	return procs, nil
}

// holders is, for each file, the processes writing it as their output, fd 1 or 2 (lsof); known false when lsof could not say.
func holders(files []string) (map[string][]int, bool) {
	out := map[string][]int{}
	if len(files) == 0 {
		return out, true
	}
	named := map[string]string{} // lsof names a file by its real path: /private/var for /var on macOS
	for _, f := range files {
		real, err := filepath.EvalSymlinks(f)
		if err != nil {
			real = f
		}
		named[real] = f
	}
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	b, err := exec.CommandContext(ctx, "lsof", append([]string{"-w", "-F", "pfan", "--"}, files...)...).Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) { // 1: none of them is open
		return nil, false
	}
	pid, writes := 0, false
	for _, line := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(line, "p"):
			pid, _ = strconv.Atoi(line[1:])
		case strings.HasPrefix(line, "f"):
			writes = line == "f1" || line == "f2"
		case strings.HasPrefix(line, "a"):
			writes = writes && (line == "aw" || line == "au")
		case strings.HasPrefix(line, "n") && pid > 0 && writes && named[line[1:]] != "":
			if f := named[line[1:]]; !slices.Contains(out[f], pid) {
				out[f] = append(out[f], pid)
			}
		}
	}
	return out, true
}

var errNoProcess = errors.New("no process of its terminal writes its output (it may have ended)")

// jobGroup is the process group to end for a job whose output file the processes open hold: of those under root (the agent's
// terminal) and below its first level (the agent's CLI), the one nearest root that leads a group of its own, apart from every
// group above it and from this daemon's.
func jobGroup(procs []proc, root int, open []int) (int, error) {
	byPid := map[int]proc{}
	for _, p := range procs {
		byPid[p.pid] = p
	}
	best, bestDepth := 0, 0
	for _, pid := range open {
		p, ok := byPid[pid]
		if !ok || p.pgid != p.pid || p.pgid == syscall.Getpgrp() {
			continue
		}
		depth, under := 0, false
		for q, seen := p, 0; seen < len(procs); seen++ {
			up, ok := byPid[q.ppid]
			if !ok {
				break
			}
			depth++
			if up.pgid == p.pgid { // a group shared with what is above it is not the job's own
				break
			}
			if up.pid == root {
				under = true
				break
			}
			q = up
		}
		if under && depth >= 2 && (best == 0 || depth < bestDepth) {
			best, bestDepth = p.pgid, depth
		}
	}
	if best == 0 {
		return 0, errNoProcess
	}
	return best, nil
}

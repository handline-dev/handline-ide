package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/scheduler/leases"
)

// useCmd is hl use <kind> -- <command...> (wait for a lease, run, give it back, exit as the command did), hl use <kind> --hold and
// hl use --release [<kind>]. A shell that is not an agent's runs the command without a lease.
func useCmd(c *hlclient.Client, args []string) (int, bool) {
	var command []string
	for i, a := range args {
		if a == "--" {
			args, command = args[:i], args[i+1:]
			break
		}
	}
	p, ok := parse(args, nil, []string{"--hold", "--release"}, nil)
	if !ok || len(p.pos) > 1 {
		return 0, false
	}
	kind := ""
	if len(p.pos) == 1 {
		if kind = p.pos[0]; !leases.Known(kind) {
			return 0, false
		}
	}
	hold, release := p.flags["--hold"], p.flags["--release"]
	switch {
	case release && !hold && command == nil:
	case hold && !release && command == nil && kind != "":
	case !hold && !release && len(command) > 0 && kind != "":
	default:
		return 0, false
	}
	agent := c.CurrentAgent()
	if agent == "" {
		if command == nil {
			fmt.Println(hlclient.WhoRefusal("hold a " + kind))
			return 1, true
		}
		fmt.Fprintln(os.Stderr, "hl use: not an agent's session, so no lease is taken")
		return run(command), true
	}
	if release {
		got, err := c.Call("lease.release", map[string]any{"agent": agent, "kind": kind})
		if err != nil {
			fmt.Fprintf(os.Stderr, "hl use: %v\n", err)
			return 1, true
		}
		fmt.Printf("gave back %d lease(s)\n", num(got, "released"))
		return 0, true
	}
	waiting := *c
	waiting.Deadline = (leases.CeilingS + 30) * time.Second
	got, err := waiting.Call("lease.take", map[string]any{"agent": agent, "kind": kind, "hold": hold})
	var down *hlclient.DaemonDown
	var refused *hlclient.RPCError
	switch {
	case errors.As(err, &refused) && refused.Data["gate"] == "lease_wait":
		fmt.Println(refused.Message)
		return BlockedExit, true
	case errors.As(err, &down) && !hold:
		fmt.Fprintf(os.Stderr, "hl use: %v; running it without a lease\n", err)
		return run(command), true
	case err != nil:
		fmt.Fprintf(os.Stderr, "hl use: %v\n", err)
		return 1, true
	}
	lease := num(got, "lease")
	if w := num(got, "waited_s"); w > 0 {
		fmt.Fprintf(os.Stderr, "hl use: waited %s for a %s\n", time.Duration(w)*time.Second, kind)
	}
	if hold {
		fmt.Printf("holding a %s (lease %d) until hl use --release or this turn's end\n", kind, lease)
		return 0, true
	}
	code := run(command)
	if _, err := c.Call("lease.release", map[string]any{"agent": agent, "lease": lease}); err != nil {
		fmt.Fprintf(os.Stderr, "hl use: giving the %s back: %v (the daemon frees it once this process is gone)\n", kind, err)
	}
	return code, true
}

// BlockedExit is hl use's exit code when its ask waits in line as a blocker on the agent's task: nothing ran (EX_TEMPFAIL).
const BlockedExit = 75

// run runs command with this process's input and output and is its exit code. hl outlives it to give the lease back: an interrupt
// reaches the command from the terminal, and a termination is passed on.
func run(command []string) int {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "hl use: %v\n", err)
		return 127
	}
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	go func() {
		for s := range sigs {
			if s != os.Interrupt {
				_ = cmd.Process.Signal(s)
			}
		}
	}()
	err := cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if st, ok := exit.Sys().(syscall.WaitStatus); ok && st.Signaled() {
			return 128 + int(st.Signal())
		}
		return exit.ExitCode()
	}
	if err != nil {
		return 1
	}
	return 0
}

// leaseText is hl team's lines on the leases: a kind with a limit, a lease held or an ask waiting, each in one line;
// nothing when no kind has any.
func leaseText(got obj) string {
	out := ""
	for _, k := range list(got, "kinds") {
		held, queue, limit := list(k, "held"), list(k, "queue"), num(k, "max")
		if len(held) == 0 && len(queue) == 0 && limit == 0 {
			continue
		}
		line := fmt.Sprintf("%ss: %d held", leases.Noun(str(k, "kind")), len(held))
		if limit > 0 {
			line += fmt.Sprintf(" of %d", limit)
		} else {
			line += ", no limit"
		}
		if len(held) > 0 {
			line += " (" + agentsOf(held) + ")"
		}
		if len(queue) > 0 {
			line += "; waiting: " + agentsOf(queue)
		}
		out += line + "\n"
	}
	if out != "" {
		out = "\n" + out + "`hl use <heavy|simulator|browser> -- <command>` waits for one and gives it back when the command exits.\n"
	}
	return out
}

func agentsOf(rows []obj) string {
	who := make([]string, len(rows))
	for i, r := range rows {
		who[i] = str(r, "agent")
	}
	return strings.Join(who, ", ")
}

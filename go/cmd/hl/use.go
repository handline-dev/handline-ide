package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/handline/handline/go/internal/hlclient"
	"github.com/handline/handline/go/scheduler/leases"
)

// useCmd is hl use <kind...> -- <command...> (take each lease, run, give them back, exit as the command did), hl use <kind> --hold
// hl use --release [<kind>] and hl use --extend [<kind>]. Several kinds are asked in the fixed order leases.Kinds, so no two asks wait on each other. A shell
// that is not an agent's runs the command without a lease.
func useCmd(c *hlclient.Client, args []string) (int, bool) {
	var command []string
	for i, a := range args {
		if a == "--" {
			args, command = args[:i], args[i+1:]
			break
		}
	}
	p, ok := parse(args, nil, []string{"--hold", "--release", "--extend"}, nil)
	if !ok {
		return 0, false
	}
	for _, k := range p.pos {
		if !leases.Known(k) {
			return 0, false
		}
	}
	var kinds []string // p.pos in the fixed order, once each
	for _, k := range leases.Kinds {
		if slices.Contains(p.pos, k) {
			kinds = append(kinds, k)
		}
	}
	hold, release, extend := p.flags["--hold"], p.flags["--release"], p.flags["--extend"]
	switch {
	case extend && !hold && !release && command == nil && len(kinds) <= 1:
	case extend:
		return 0, false
	case release && !hold && command == nil && len(kinds) <= 1:
	case hold && !release && command == nil && len(kinds) == 1: // a hold may not wait beside another, so one kind
	case !hold && !release && len(command) > 0 && len(kinds) > 0:
	default:
		return 0, false
	}
	agent := c.CurrentAgent()
	if agent == "" {
		if command == nil {
			what := "give a lease back"
			switch {
			case hold:
				what = "hold a " + kinds[0]
			case extend:
				what = "extend a lease"
			}
			fmt.Println(hlclient.WhoRefusal(what))
			return 1, true
		}
		fmt.Fprintln(os.Stderr, "hl use: not an agent's session, so no lease is taken")
		return run(command), true
	}
	kind := ""
	if len(kinds) == 1 {
		kind = kinds[0]
	}
	if extend {
		got, err := c.Call("lease.extend", map[string]any{"agent": agent, "kind": kind})
		if err != nil {
			fmt.Fprintf(os.Stderr, "hl use: %v\n", err)
			return 1, true
		}
		for _, l := range list(got, "extended") {
			fmt.Printf("lease %d (%s) now runs to %s\n", num(l, "lease"), str(l, "kind"), str(l, "max_hold_at"))
		}
		return 0, true
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
	var taken []int64
	giveBack := func() {
		for _, id := range taken {
			if _, err := c.Call("lease.release", map[string]any{"agent": agent, "lease": id}); err != nil {
				fmt.Fprintf(os.Stderr, "hl use: giving lease %d back: %v (the daemon frees it once this process is gone)\n", id, err)
			}
		}
	}
	waiting := *c
	waiting.Deadline = (leases.CeilingS + 30) * time.Second
	var env []string
	for _, kind := range kinds {
		got, err := waiting.Call("lease.take", map[string]any{"agent": agent, "kind": kind, "hold": hold})
		var down *hlclient.DaemonDown
		var refused *hlclient.RPCError
		switch {
		case errors.As(err, &refused) && refused.Data["gate"] == "lease_wait":
			giveBack() // a task in line keeps nothing from others
			fmt.Println(refused.Message)
			return BlockedExit, true
		case errors.As(err, &down) && !hold: // leases taken before stay this process's until it exits
			fmt.Fprintf(os.Stderr, "hl use: %v; running it without a lease\n", err)
			return run(command), true
		case err != nil:
			giveBack()
			fmt.Fprintf(os.Stderr, "hl use: %v\n", err)
			return 1, true
		}
		lease, port := num(got, "lease"), num(got, "port")
		taken = append(taken, lease)
		if w := num(got, "waited_s"); w > 0 {
			fmt.Fprintf(os.Stderr, "hl use: waited %s for a %s\n", time.Duration(w)*time.Second, kind)
		}
		what := "a " + kind
		if kind == leases.Port {
			what = fmt.Sprintf("port %d", port)
			env = []string{fmt.Sprintf("HL_PORT=%d", port)}
		}
		if hold {
			fmt.Printf("holding %s (lease %d) until hl use --release or this turn's end\n", what, lease)
			return 0, true
		}
	}
	code := run(command, env...)
	giveBack()
	return code, true
}

// BlockedExit is hl use's exit code when its ask waits in line as a blocker on the agent's task: nothing ran (EX_TEMPFAIL).
const BlockedExit = 75

// run runs command with this process's input and output and env added to its environment, and is its exit code. hl outlives it to
// give the lease back: an interrupt reaches the command from the terminal, and a termination is passed on.
func run(command []string, env ...string) int {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
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
		out = "\n" + out + "`hl use <heavy|simulator|browser|port> -- <command>` waits for one and gives it back when the command exits.\n"
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

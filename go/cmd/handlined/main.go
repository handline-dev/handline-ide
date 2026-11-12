// Command handlined is the Go daemon: one binary, its version the git sha it was built from.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/handline/handline/go/agentctl/agent/claudecode/turns"
	dbpkg "github.com/handline/handline/go/data/db"
	"github.com/handline/handline/go/data/homeinit"
	"github.com/handline/handline/go/internal/compose"
	"github.com/handline/handline/go/internal/daemon"
	"github.com/handline/handline/go/internal/housekeeping"
	"github.com/handline/handline/go/internal/reads"
	"github.com/handline/handline/go/internal/sessionrpc"
	"github.com/handline/handline/go/kit/paths"
	"github.com/handline/handline/go/kit/version"
	"github.com/handline/handline/go/protocol/sessiontoken"
	"github.com/handline/handline/go/scheduler/procload"
	"github.com/handline/handline/go/scheduler/sysres"
	"github.com/handline/handline/go/taskboard/supervisor"
	"github.com/handline/handline/go/taskboard/taskwrites"
)

// resolveHome refuses the default home unless it was asked for: this daemon does not yet guard the real
// home against a build that is not the installed one, so nothing starts on it by accident. -home naming the
// real home is refused the same way.
func resolveHome(flagHome string, allowReal bool) (string, error) {
	if flagHome != "" {
		if !allowReal && isRealHome(flagHome) {
			return "", fmt.Errorf("-home %s is the real handline home (-allow-real-home starts on it)", flagHome)
		}
		return flagHome, nil
	}
	if !allowReal {
		return "", fmt.Errorf("-home is required while the daemon is in development (-allow-real-home uses %s)", paths.Home())
	}
	return paths.Home(), nil
}

// realPath is p with links followed, through its nearest ancestor that exists.
func realPath(p string) string {
	p = filepath.Clean(p)
	for rest := ""; ; {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest, p = filepath.Join(filepath.Base(p), rest), parent
	}
}

// isRealHome is whether p is, or is inside, the user's ~/.handline, by the account's home directory and not the
// test root's stand-in for it.
func isRealHome(p string) bool {
	u, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	p, real := realPath(p), realPath(filepath.Join(u, ".handline"))
	return p == real || strings.HasPrefix(p, real+string(filepath.Separator))
}

// missingTools logs, one line each, which of the tools agents are run with this machine lacks; daemon.status serves the same list.
func missingTools(claude string) {
	for _, m := range reads.MissingTools(claude) {
		log.Printf("handlined: %s not found: no agent can start (%s)", m.Tool, m.Fix)
	}
}

// fromParts gives the daemon what the composed parts serve: why a queued line waits, the sleep lock, the session methods (on ccfg as
// OnOpen leaves it), the wait before the database closes, and the agents' context figures.
func fromParts(cfg *daemon.Config, parts *compose.Parts, ccfg *compose.Config) {
	cfg.Delivery = parts.Delivery()
	cfg.KeepAwake = parts.KeepAwake
	cfg.Session = func() *sessionrpc.Env { return parts.SessionRPC(*ccfg) }
	cfg.BeforeClose = parts.Wait
	cfg.Context = parts.Context
}

// shareLoad has the daemon and the agent views read one process tracker: the daemon serves its load, the views carry its servers.
func shareLoad(cfg *daemon.Config, ccfg *compose.Config) {
	cfg.Load = &procload.Tracker{Home: ccfg.Home, Log: func(s string) { log.Print(s) }}
	ccfg.Servers = cfg.Load.ServersOf
}

func main() {
	home := flag.String("home", "", "the handline home (required while the daemon is in development)")
	allowReal := flag.Bool("allow-real-home", false, "with no -home, use the default home, which a running install uses")
	db := flag.String("db", "", "database file (default: <home>/db.sqlite)")
	sock := flag.String("socket", "", "local socket (default: <home>/daemon.sock)")
	listen := flag.String("listen", "", "address for the network listener; off when empty. With no -cert, :PORT binds the Tailscale address")
	cert := flag.String("cert", "", "certificate for the network listener (with -key); with neither it serves plain HTTP on Tailscale or loopback")
	key := flag.String("key", "", "private key for the network listener")
	serveFDs := flag.String("serve-fds", "", "comma-separated file descriptors of connections already open (socketpairs "+
		"made by the parent, for a run where no socket can be bound); each is served and no socket file is made")
	web := flag.String("web", "", "the built web UI to serve on the listener's own origin; empty serves none")
	advertise := flag.String("advertise", "", "the host:port a pairing code names, when it differs from -listen (a Tailscale host name)")
	serveNetFDs := flag.String("serve-net-fds", "", "like -serve-fds, but each connection is served as one of the network listener's "+
		"(past TLS), so it is unpaired until a device token says otherwise; needs -listen")
	claude := flag.String("claude", "", "the claude binary the usage poll and the plan reads run: with it the daemon builds its agent side (the lifecycle, the session methods, the supervisor's ports, and with -terminal-tick the policy tick); empty runs none")
	terminalTick := flag.Bool("terminal-tick", true, "run the policy tick over the terminal agents too, reading their sessions from <config dir>/sessions/<pid>.json (it wakes, closes, restarts and hands off their sessions); needs -claude, and the Python daemon must not tick beside it on the same home. The tick also reads every account's usage and token log")
	notifiedNeedsYou := flag.Bool("notification-needs-you", true, "with -claude: a permission prompt Claude Code's Notification hook reports as unanswered raises the agent's needs-you card, until the agent moves on")
	founderTyped := flag.Bool("founder-typed", true, "with -claude and -turns-from-hooks: the founder's own messages to a terminal agent are typed into its terminal as its person's prompt (it answers with hl say); one that cannot be typed then, and everything else, goes through the peer inbox. -founder-typed=false sends them all through the inbox")
	turnsFromHooks := flag.Bool("turns-from-hooks", true, "tell whether a terminal agent is mid-turn or has background work from its hooks and transcript instead of its screen; the screen is still read where no hook has said")
	showVersion := flag.Bool("version", false, "print the git sha this binary was built from")
	exitWithStdin := flag.Bool("exit-with-stdin", false, "stop when stdin closes: the app that started this daemon holds the other end of a pipe, so its quit, crash or kill -9 stops the daemon too (agents keep running in their terminals)")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Sha)
		return
	}
	homeDir, err := resolveHome(*home, *allowReal)
	if err != nil {
		log.Fatalf("handlined: %v", err)
	}
	if err := homeinit.Setup(homeDir, paths.HlExe(), func(f string, a ...any) { log.Printf("handlined: "+f, a...) }); err != nil {
		log.Printf("handlined: setting the home up failed (run `hl init` to see why): %v", err)
	}
	cfg := daemon.Config{Home: homeDir, DBFile: *db, SocketFile: *sock, Listen: *listen, CertFile: *cert, KeyFile: *key, Log: log.Printf,
		WebDir: *web, Advertise: *advertise, EnsureFounderKey: sessiontoken.New(homeDir).EnsureFounderKey}
	var tracker *turns.Tracker
	if *turnsFromHooks {
		tracker = &turns.Tracker{Log: log.Printf}
		cfg.Turns = tracker
	}
	var tk *compose.Tick                                     // the policy tick, once built
	var startTick func(stop context.Context) <-chan struct{} // set when the policy tick is on: runs it until stop is done, answers when it has returned
	if *claude != "" {
		missingTools(*claude)
		cfg.Claude = *claude
		if *founderTyped && !*turnsFromHooks {
			log.Printf("handlined: founder chat goes through the inbox: -founder-typed takes effect only with -turns-from-hooks")
		}
		ccfg := compose.Config{Home: homeDir, Log: log.Printf, CLI: *claude, NotifiedNeedsYou: *notifiedNeedsYou, FounderTyped: *founderTyped && *turnsFromHooks}
		if root, _ := paths.Getenv("TEST_ROOT"); root != "" {
			// a scratch daemon (a live run): its agents' hl and hooks reach it, not the real home's daemon
			ccfg.Env = []string{"HANDLINE_HOME=" + homeDir, "HANDLINE_TEST_ROOT=" + root}
		}
		if tracker != nil {
			ccfg.Turns = tracker
		}
		cfg.Resources = &sysres.Sampler{Home: homeDir, Log: func(s string) { log.Print(s) }}
		ccfg.Machine = cfg.Resources.LatestMachine
		shareLoad(&cfg, &ccfg)
		parts := compose.New(ccfg)
		cfg.Watch = *terminalTick
		fromParts(&cfg, parts, &ccfg)
		cfg.Transcripts = true
		cfg.OnOpen = func(db *sql.DB, env *supervisor.Env) {
			parts.Bind(db, env)
			if n, err := dbpkg.WithdrawMemoAsksOfEndedTasks(db); err != nil {
				log.Printf("withdrawing the comment asks of ended tasks: %v", err)
			} else if n > 0 {
				log.Printf("withdrew %d comment asks of ended tasks", n)
			}
			if n, err := taskwrites.WithdrawUnaddressedMemoAsks(db); err != nil {
				log.Printf("withdrawing the comment asks not addressed to the founder: %v", err)
			} else if n > 0 {
				log.Printf("withdrew %d comment asks not addressed to the founder", n)
			}
			if err := compose.RetryStartStalls(db, log.Printf); err != nil {
				log.Printf("clearing the start and wake stalls to try again: %v", err)
			}
			ccfg.DB = db
			env.Ports = parts.SupervisorPorts(parts.Lifecycle(ccfg, compose.LifecycleConfig{Env: env}))
			parts.SettlePeerSends()
			if *terminalTick {
				startTick = func(stop context.Context) <-chan struct{} {
					compose.CheckRepos(db, homeDir, env, log.Printf) // with the tick that would start the agents
					tk = parts.Tick(ccfg, compose.TickConfig{Stop: stop.Done(), Why: func() string { return causeOf(stop) }, Terminals: *terminalTick})
					return tk.Start()
				}
			}
		}
	}
	if cfg.DBFile == "" {
		cfg.DBFile = paths.DBFile(homeDir)
	}
	if cfg.SocketFile == "" && *serveFDs == "" && *serveNetFDs == "" {
		cfg.SocketFile = paths.SocketFile(homeDir)
	}
	descriptors := func(flagName, list string) []int {
		var fds []int
		for _, f := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' }) {
			n, err := strconv.Atoi(f)
			if err != nil {
				log.Fatalf("handlined: -%s: %q is not a descriptor", flagName, f)
			}
			fds = append(fds, n)
		}
		return fds
	}
	fds, netFDs := descriptors("serve-fds", *serveFDs), descriptors("serve-net-fds", *serveNetFDs)
	d, err := daemon.Start(cfg)
	if err != nil {
		log.Fatalf("handlined: %v", err)
	}
	for _, fd := range fds {
		if err := d.ServeFile(os.NewFile(uintptr(fd), "inherited")); err != nil {
			log.Fatalf("handlined: fd %d: %v", fd, err)
		}
	}
	for _, fd := range netFDs {
		if err := d.ServeNetFile(os.NewFile(uintptr(fd), "inherited")); err != nil {
			log.Fatalf("handlined: fd %d: %v", fd, err)
		}
	}
	defer housekeeping.DumpStacksOnUSR1()() // kill -USR1 <pid>: every goroutine's stack, in the err log
	// kill -USR2 <pid>: a CPU profile of the next 30 s, under the home's logs
	defer housekeeping.ProfileCPUOnUSR2(filepath.Join(homeDir, "logs"), func(s string) { log.Print(s) })()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *exitWithStdin {
		ctx = untilClosed(ctx, os.Stdin)
	}
	var tickDone <-chan struct{}
	if startTick != nil {
		tickDone = startTick(ctx)
	}
	select {
	case <-ctx.Done():
	case <-tickDone: // the loop ended by itself: dev mode found a changed executable (nil when there is no tick)
	}
	if tickDone != nil {
		<-tickDone // the tick in hand finishes before the database closes: a wake or a close is not cut off in the middle
	}
	d.Stop(daemon.StopLimit, os.Stderr, ctx.Err() != nil, os.Exit)
	if tk != nil && tk.Restarting() && ctx.Err() == nil { // the lock and the database are given up, as a clean exit gives them up
		if err := tk.Restart(); err != nil {
			log.Fatalf("handlined: dev-mode restart: %v", err)
		}
	}
}

// causeOf names what ended ctx ("terminated signal received", "stdin closed"), "" while it runs.
func causeOf(ctx context.Context) string {
	if c := context.Cause(ctx); c != nil {
		return c.Error()
	}
	return ""
}

// untilClosed is ctx, also done once r reads to its end: the owner's end of the pipe is gone, so the daemon stops as it does on SIGTERM.
func untilClosed(ctx context.Context, r io.Reader) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)
	go func() {
		io.Copy(io.Discard, r)
		log.Print("handlined: stdin closed, so the app that owns this daemon is gone: stopping")
		cancel(errors.New("stdin closed"))
	}()
	return ctx
}

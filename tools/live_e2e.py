#!/usr/bin/env python3
"""Drive one real Claude Code agent through a test daemon and time how fast the daemon's event stream reflects each step.

  HANDLINE_LIVE=1 tools/live-e2e.sh [--daemon BINARY] [--json] [--keep]

Spends real tokens: run it only on purpose, never from check.sh or CI. Any agent may run it. It spends the
account tools/live_account.py picks (the most usage headroom now). It never touches the live daemon: it starts its own
on a scratch home under a temp test root, with one haiku agent, added to its database once it runs, in permission mode `default` (the permission step needs a
real prompt), the real shared settings, and the picked account's real config dir (HANDLINE_LIVE_ACCOUNTS: the keychain
keeps the login under that path). Its hooks are that config dir's, so they run the installed hl: a change to `hl hook` shows here
only once it is installed. The agent's own hl and hooks reach the test daemon. --daemon is the handlined to run;
by default this checkout's is built.
It answers yes to every permission prompt the agent raises outside the permission step, so the run is unattended:
the agent works in a scratch checkout, but on this machine and with the real account's config dir.

It subscribes to the agents, tasks, chat and notifications topics and waits on each step's event with a deadline; nothing
is polled. A step the daemon reflects within 5 s is `pass`, later is `slow`, never is `missing`; `merged` is a state the
stream skipped because the next one came in the same scan. A step paced by the model
(the answer, the agent's own message, the task closing) gets MODEL_S instead, and its time is mostly the model's. A missing
step is followed by one agent.get so the table says whether the thing happened without an event. A missing step is a
finding for the table, not a failure: exit 1 only when a step FAILs.

The agent is a terminal agent, on a daemon with -turns-from-hooks, -notification-needs-you and the terminal tick, in a
tmux server of the run's own: a normal turn, a permission question allowed and one denied from its card (the hook's flow),
an Esc pressed mid-turn, session.interrupt mid-turn, and the CLI killed hard mid-turn. Its checkout is a temporary worktree of
this repository, which every account already trusts, so no folder-trust dialog shows and no account's .claude.json is written.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import secrets
import shutil
import signal
import socket
import sqlite3
import subprocess
import sys
import tempfile
import threading
import time
import uuid
from datetime import datetime
from pathlib import Path

import live_account
import tmuxsock

DEADLINE_S = 5.0
SLOW_S = 30.0      # how long past the deadline a step still counts as slow rather than missing
MODEL_S = 120.0
TOPICS = ["agents", "tasks", "chat", "notifications"]
AGENT = "live.e2e"
REPO = Path(__file__).resolve().parent.parent


class Refused(Exception):
    pass


class Daemon:
    """One JSON-RPC connection to the test daemon's socket, said hello to as its founder."""

    def __init__(self, sock: Path, key: str) -> None:
        self.sock = socket.socket(socket.AF_UNIX)
        self.sock.connect(str(sock))
        self.file = self.sock.makefile("rb")
        self.next_id = 0
        self.hello = self.call("hello", {"client": "live-e2e", "protocol": 1, "key": key})
        if self.hello.get("role") != "founder":
            raise Refused(f"the daemon took this connection as {self.hello.get('caller')}, not the founder")

    def read(self) -> dict | None:
        line = self.file.readline()
        return json.loads(line) if line else None

    def call(self, method: str, params: dict) -> dict:
        self.next_id += 1
        mid = self.next_id
        self.sock.sendall((json.dumps({"jsonrpc": "2.0", "id": mid, "method": method, "params": params}) + "\n").encode())
        while (msg := self.read()) is not None:
            if msg.get("id") == mid:
                if "error" in msg:
                    raise Refused(f"{method}: {msg['error'].get('message')}")
                return msg["result"]
        raise Refused(f"{method}: the daemon closed the connection")

    def write(self, method: str, params: dict) -> dict:
        return self.call(method, {**params, "request_id": f"live-e2e-{uuid.uuid4()}"})


class Stream:
    """A subscribed connection: a reader thread keeps every event with its arrival time and wakes whoever waits."""

    def __init__(self, sock: Path, key: str) -> None:
        self.conn = Daemon(sock, key)
        self.conn.call("subscribe", {"topics": TOPICS})
        self.events: list[tuple[float, dict]] = []
        self.cond = threading.Condition()
        self.closed = False
        threading.Thread(target=self._read, daemon=True).start()

    def _read(self) -> None:
        try:
            while (msg := self.conn.read()) is not None:
                if msg.get("method") == "event":
                    with self.cond:
                        self.events.append((time.time(), msg["params"]))
                        self.cond.notify_all()
        finally:
            with self.cond:
                self.closed = True
                self.cond.notify_all()

    def wait(self, since: float, pred, until: float) -> tuple[float, dict] | None:
        """The first event at or after `since` that `pred` takes, waiting until `until` for it."""
        i = 0
        with self.cond:
            while True:
                for at, ev in self.events[i:]:
                    if at >= since and pred(ev):
                        return at, ev
                i = len(self.events)
                left = until - time.time()
                if left <= 0 or self.closed:
                    return None
                self.cond.wait(left)

    def states_since(self, agent: str, since: float) -> list[str]:
        with self.cond:
            return [ev["data"].get("state") for at, ev in self.events
                    if at >= since and ev["type"] == "agent.changed" and ev.get("agent") == agent]


def agent_changed(agent: str, state: str):
    return lambda ev: ev["type"] == "agent.changed" and ev.get("agent") == agent and ev["data"].get("state") == state


def landed(req: int | None):
    """A founder message the daemon delivered into the agent's session (chat.send is how a structured session takes input)."""
    return lambda ev: ev["type"] == "chat.message" and ev["data"].get("id") == req and bool(ev["data"].get("landed_at"))


def yes(prompt: dict) -> str:
    """The option that answers yes: a structured prompt offers its own keys (allow, deny)."""
    return next((o["key"] for o in prompt.get("options") or [] if o.get("answer") == "yes"), "yes")


class Answerer:
    """Answers yes to the agent's permission prompts outside the permission step, on a connection of its own: a haiku agent in
    mode default asks before almost any command it improvises, and those prompts are not what a step measures."""

    def __init__(self, sock: Path, key: str, stream: Stream, agent: str) -> None:
        self.d, self.s, self.agent = Daemon(sock, key), stream, agent
        self.on = threading.Event()
        self.on.set()
        self.answered: set[str] = set()
        self.yeses = 0
        threading.Thread(target=self.loop, daemon=True).start()

    def loop(self) -> None:
        seen, since = set(), 0.0
        asks = lambda ev: (ev.get("agent") == self.agent or (ev.get("data") or {}).get("agent") == self.agent) and id(ev) not in seen \
            and (ev["type"] == "prompt_asked" or agent_changed(self.agent, "waiting")(ev))
        while got := self.s.wait(since, asks, time.time() + 3600):
            since, ev = got
            seen.add(id(ev))
            self.on.wait()
            try:
                prompt = self.d.call("agent.get", {"agent": self.agent}).get("prompt")
                if prompt and prompt["id"] not in self.answered:
                    self.answered.add(prompt["id"])
                    self.d.write("prompt.answer", {"agent": self.agent, "prompt": prompt["id"], "choice": yes(prompt)})
                    self.yeses += 1
            except (Refused, OSError):
                pass


class Run:
    def __init__(self, daemon: Daemon, stream: Stream, answerer: Answerer, agent: str) -> None:
        self.d, self.s, self.answerer, self.agent = daemon, stream, answerer, agent
        self.rows: list[dict] = []

    def record(self, row: dict) -> None:
        self.rows.append(row)
        took = f"  {row['seconds']}s" if row.get("seconds") is not None else ""
        print(f"  {row['result']:8} {row['step']}{took}" + (f"  ({row['note']})" if row.get("note") else ""), flush=True)

    def step(self, name: str, since: float, pred, model: bool = False, judge=None) -> dict | None:
        """Wait for one step's event and record it; a missing one is explained by one agent.get. judge(event) may return
        (result, note) to record in place of the timed result."""
        got = self.s.wait(since, pred, since + (MODEL_S if model else DEADLINE_S + SLOW_S))
        row = {"step": name, "paced_by": "model" if model else "daemon", "result": "missing", "seconds": None}
        if got:
            took = got[0] - since
            row.update(result="pass" if model or took <= DEADLINE_S else "slow", seconds=round(took, 2))
            if judge and (verdict := judge(got[1])):
                row["result"], row["note"] = verdict
        else:
            row["note"] = self.explain(since)
        self.record(row)
        return got[1] if got else None

    def explain(self, since: float) -> str:
        v = self.d.call("agent.get", {"agent": self.agent})
        seen = ", ".join(self.s.states_since(self.agent, since)) or "none"
        return f"states seen: {seen}; agent.get: state {v.get('state')}, terminal {v.get('terminal')!r}, session_open {v.get('session_open')}"

    def fail(self, name: str, why: str) -> None:
        self.record({"step": name, "paced_by": "daemon", "result": "FAIL", "seconds": None, "note": why})


def turn_end_at(config_dir: str, root: Path) -> float | None:
    """When the last turn's Stop hook ran, from the transcript's stop_hook_summary entry (millisecond timestamps; the hook rows
    in the database keep whole seconds)."""
    mark = re.sub(r"[^A-Za-z0-9]", "-", root.name)
    files = [f for f in Path(config_dir, "projects").glob(f"*{mark}*/*.jsonl")]
    if not files:
        return None
    at = None
    for line in max(files, key=lambda f: f.stat().st_mtime).read_text().splitlines():
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if e.get("subtype") == "stop_hook_summary" and e.get("timestamp"):
            at = e["timestamp"]
    return datetime.fromisoformat(at.replace("Z", "+00:00")).timestamp() if at else None


def pane(sock: str) -> tuple[str, int | None]:
    """The agent's pane and the CLI under it: the pane runs a shell, and the launch typed claude into it."""
    pane_id, shell = subprocess.run(["tmux", "-L", sock, "list-panes", "-a", "-F", "#{pane_id} #{pane_pid}"],
                                    capture_output=True, text=True).stdout.split()[:2]
    kids = subprocess.run(["pgrep", "-P", shell], capture_output=True, text=True).stdout.split()
    return pane_id, int(kids[0]) if kids else None


def hook_permission(r: Run, work: Path, allow: bool) -> None:
    """A permission question through the PermissionRequest hook: the card shows with the command, and its answer goes back as the
    hook's decision, with no key typed. A deny carries a note the agent reads."""
    d, a = r.d, r.agent
    word = "allow" if allow else "deny"
    nonce = uuid.uuid4().hex[:8]
    r.answerer.on.clear()
    t = time.time()
    d.write("chat.send", {"agent": a, "body": f"Run this Bash command and nothing else: touch e2e-{word}-{nonce}.txt"})
    asked = r.step(f"{word}: permission card from the hook", t,
                   lambda ev: ev["type"] == "prompt_asked" and ev["data"].get("agent") == a and nonce in " ".join(ev["data"].get("lines") or []),
                   model=True)
    if asked:
        prompt = asked["data"]
        r.answerer.answered.add(prompt["id"])
        t = time.time()
        if allow:
            d.write("prompt.answer", {"agent": a, "prompt": prompt["id"], "choice": yes(prompt)})
        else:
            d.write("prompt.answer", {"agent": a, "prompt": prompt["id"], "choice": "deny", "note": f"e2e {nonce}: reply NOTED"})
        if not allow:
            r.step("deny: the agent read the note", t, lambda ev: ev["type"] == "chat.message" and ev["data"].get("from") == a
                   and "NOTED" in (ev["data"].get("body") or ""), model=True)
        r.step(f"{word}: the agent went on to idle", t, agent_changed(a, "idle"), model=True)
        if (work / f"e2e-{word}-{nonce}.txt").exists() != allow:
            r.fail(f"{word}: the answer took effect", f"e2e-{word}-{nonce}.txt {'missing' if allow else 'made'} in {work}")
    r.answerer.on.set()


def terminal_turns(r: Run, sock: str, config_dir: str, root: Path, group: int) -> None:
    """Working and idle from hooks, timed: a normal turn, a permission prompt, an Esc mid-turn, and a CLI killed hard mid-turn."""
    d, a = r.d, r.agent
    t = time.time()
    try:
        d.write("session.start", {"agent": a})
    except Refused as e:
        # Under load hl launch can take longer to become claude than session.start waits to see it; the session still comes up.
        if "no session is running in it after" not in str(e):
            raise
        print(f"live-e2e: session.start stopped waiting before claude was up; the steps below wait on", flush=True)
    r.step("starting shown until ready", t, agent_changed(a, "starting"))
    if not r.step("ready (idle)", t, agent_changed(a, "idle"), model=True):
        return r.fail("terminal turns", "the terminal agent never came up idle; nothing after it can be timed")

    t = time.time()
    req = d.write("chat.send", {"agent": a, "body": "Reply with the single word PONG and nothing else."}).get("request")
    r.step("message landed", t, landed(req))
    r.step("working after the message was sent", t, agent_changed(a, "running"))
    # The founder's message is typed as the person's prompt (-founder-typed), so its reply is the turn's text, read from the transcript.
    reply = r.step("typed message's reply reached the founder chat", t,
                   lambda ev: ev["type"] == "chat.message" and ev["data"].get("from") == a and "PONG" in (ev["data"].get("body") or ""),
                   model=True)
    if reply:
        print(f"live-e2e: founder request {req}, reply {reply['data'].get('id')}: {reply['data'].get('body')!r}", flush=True)
    idle = r.s.wait(t, agent_changed(a, "idle"), time.time() + MODEL_S)
    replied = None
    for _ in range(25):  # Claude Code writes the hook's summary line after the hook ran, so it can land after the idle it caused
        replied = turn_end_at(config_dir, root)
        if replied:
            break
        time.sleep(0.2)
    if idle and replied:
        took = round(idle[0] - replied, 2)
        r.record({"step": "idle after the Stop hook (transcript time to agent.changed)", "paced_by": "daemon",
                  "result": "pass" if took <= DEADLINE_S else "slow", "seconds": took})
    else:
        r.record({"step": "idle after the Stop hook (transcript time to agent.changed)", "paced_by": "daemon", "result": "missing",
                  "seconds": None, "note": f"idle event {'seen' if idle else 'never'}, Stop in the transcript {'found' if replied else 'not found'}"})

    hook_permission(r, root / "work", allow=True)
    hook_permission(r, root / "work", allow=False)

    long_turn = "Write a 1500-word short story about a lighthouse keeper, in plain prose, with no tools."
    t = time.time()
    d.write("chat.send", {"agent": a, "body": long_turn})
    running = r.s.wait(t, agent_changed(a, "running"), t + MODEL_S)
    time.sleep(3)
    if not running or r.s.wait(running[0], agent_changed(a, "idle"), time.time()):
        r.fail("Esc: idle after the key", "the turn was not running when the Esc was due")
    else:
        t = time.time()
        subprocess.run(["tmux", "-L", sock, "send-keys", "-t", pane(sock)[0], "Escape"], check=True)
        r.step("Esc: idle after the key", t, agent_changed(a, "idle"))

    t = time.time()
    d.write("chat.send", {"agent": a, "body": long_turn})
    running = r.s.wait(t, agent_changed(a, "running"), t + MODEL_S)
    time.sleep(3)
    if not running or r.s.wait(running[0], agent_changed(a, "idle"), time.time()):
        r.fail("kill switch: idle after session.interrupt", "the turn was not running when the interrupt was due")
    else:
        t = time.time()
        d.write("session.interrupt", {"agent": a})
        r.step("kill switch: idle after session.interrupt", t, agent_changed(a, "idle"))

    t = time.time()
    d.write("chat.send", {"agent": a, "body": long_turn})
    running = r.s.wait(t, agent_changed(a, "running"), t + MODEL_S)
    pid = pane(sock)[1]
    if not running or not pid:
        return r.fail("hard kill mid-turn", f"running {bool(running)}, CLI pid {pid}")
    time.sleep(2)
    t = time.time()
    os.kill(pid, signal.SIGKILL)
    gone = r.s.wait(t, lambda ev: agent_changed(a, "idle")(ev) or agent_changed(a, "no_terminal")(ev), t + DEADLINE_S + SLOW_S)
    held = subprocess.run(["lsof", "-g", str(group)], capture_output=True, text=True).stdout
    fds = sum(1 for line in held.splitlines() if line.endswith(".jsonl") and "/projects/" in line)
    r.record({"step": "hard kill mid-turn: leaves running", "paced_by": "daemon",
              "result": ("pass" if gone[0] - t <= DEADLINE_S else "slow") if gone else "missing",
              "seconds": round(gone[0] - t, 2) if gone else None,
              "note": f"states seen: {', '.join(r.s.states_since(a, t)) or 'none'}; transcript fds the daemon still holds: {fds}"})


def scratch(root: Path, account: dict) -> tuple[Path, str]:
    """The test daemon's home under root, with its founder key where the daemon looks for one under a test root, the real shared
    settings, and a git checkout for the agent (a worktree of this repository: see the module doc). The account and the agent are
    rows the daemon's database does not have until it runs: seed() adds them."""
    home, real = root / "state", Path(account["home"])
    store = root / ".config" / "handline" / "protected" / re.sub(r"[^A-Za-z0-9]", "-", str(home)) / "founder-keys"
    store.mkdir(parents=True, mode=0o700)
    key = secrets.token_urlsafe(32)
    (store / "key").write_text(key)
    (store / "key").chmod(0o600)
    (home / "shared" / ".claude").mkdir(parents=True)
    shutil.copy(real / "shared" / ".claude" / "settings.json", home / "shared" / ".claude" / "settings.json")
    work = root / "work"
    subprocess.run(["git", "worktree", "add", "-q", "--detach", str(work), "HEAD"], cwd=REPO, check=True)
    return home, key


def seed(d: Daemon, home: Path, account: dict, work: Path) -> None:
    """The picked account (registered through account.add, with the reading the live daemon last took of it) and the agent: its
    workspace and agent rows, at haiku in permission mode `default`, and its folder. No RPC registers an agent without starting it,
    in the daemon's default mode, so these rows are inserted."""
    d.write("account.add", {"account": account["account"]})
    acct, real = home / "accounts" / account["account"], Path(account["home"]) / "accounts" / account["account"]
    for f in ("status.json", "subscription.json"):
        if (real / f).exists():
            shutil.copy(real / f, acct / f)
    workspace, name = AGENT.split(".")
    con = sqlite3.connect(home / "db.sqlite", timeout=10)
    owner = con.execute("SELECT 'p' || id FROM people WHERE role = 'owner' ORDER BY id LIMIT 1").fetchone()[0]
    ws = con.execute("INSERT INTO workspaces(name, folder, created_at) VALUES(?, ?, 0)", (workspace, str(work))).lastrowid
    lead = con.execute("INSERT INTO agents(workspace, name, owner, reports_to, model, effort, weight, mode, created_at) "
                       "VALUES(?, 'lead', ?, ?, 'haiku', 'medium', 'light', 'default', 0)", (ws, owner, owner)).lastrowid
    con.execute("UPDATE workspaces SET lead = ? WHERE id = ?", (lead, ws))
    agent_id = con.execute("INSERT INTO agents(workspace, name, owner, reports_to, model, effort, weight, mode, created_at) "
                      "VALUES(?, ?, ?, ?, 'haiku', 'medium', 'light', 'default', 0)", (ws, name, owner, f"a{lead}")).lastrowid
    con.execute("INSERT OR REPLACE INTO agent_state(agent, state, updated, worktree) VALUES(?, 'IDLE', 0, ?)", (AGENT, str(work)))
    con.commit()
    con.close()
    (home / "agents" / f"a{agent_id}").mkdir(parents=True)


def start_daemon(binary: str, root: Path, home: Path, account: dict, tmux: str) -> int:
    """The test daemon as production runs it (terminal agents, the tick, hook-driven turns), in a process group of its own so its
    sessions go with it; returns that group. Like launchd's daemon it has no parent: an agent's hl counts the Claude Codes
    above it, and the one running this run must not be one."""
    # OCP_* is the old name of HANDLINE_*, still in the environment of a session started before the rename: its OCP_HOME names the real home,
    # the agent's hooks refuse the two homes, and the agent never reports idle.
    env = {k: v for k, v in os.environ.items() if k not in ("HANDLINE_AGENT", "HANDLINE_HOME") and not k.startswith("OCP_")}
    env |= {"HANDLINE_TEST_ROOT": str(root), "HANDLINE_LIVE_ACCOUNTS": account["home"]}
    # A tmux server of the run's own, named so the run can stop it: the live one is never touched, and a stopped daemon leaves its own.
    env |= {"HANDLINE_TMUX_SOCKET": tmux}
    hl = str(REPO / "bin" / "hl")      # this checkout's hl, whose launch takes the real config dir under a test root
    subprocess.run([str(REPO / "tools" / "build-hl.sh")], check=True, stdout=subprocess.DEVNULL)
    env |= {"HANDLINE_HOME": str(home), "HANDLINE_BIN": hl}
    flags = ["-terminal-tick", "-turns-from-hooks", "-notification-needs-you"]
    claude = shutil.which("claude")
    if not claude:
        raise Refused("no claude on PATH")
    sh = subprocess.Popen(["/bin/sh", "-c", '"$@" >/dev/null 2>"$LOG" & echo $!', "sh", binary, "-home", str(home), "-claude",
                           claude, *flags],
                          env=env | {"LOG": str(root / "daemon.log")}, stdout=subprocess.PIPE, start_new_session=True)
    pid = int(sh.stdout.readline())
    sh.wait()
    sock = home / "daemon.sock"
    until = time.time() + 30
    while not sock.exists():
        if not alive(pid) or time.time() > until:
            stop_daemon(sh.pid)
            raise Refused(f"the test daemon did not come up; its log: {root / 'daemon.log'}")
        time.sleep(0.1)
    return sh.pid


def alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False


def stop_daemon(group: int) -> None:
    """Ends the test daemon's group: SIGTERM, then SIGKILL for whatever is still there 30 s later."""
    try:
        os.killpg(group, signal.SIGTERM)
    except ProcessLookupError:
        return
    until = time.time() + 30
    while time.time() < until:
        try:
            os.killpg(group, 0)
        except ProcessLookupError:
            return
        time.sleep(0.2)
    try:
        os.killpg(group, signal.SIGKILL)
    except ProcessLookupError:
        pass


def end_on_signals() -> None:
    """SIGTERM and SIGHUP (a closed session, a tool's timeout) end the run as Ctrl-C does, so its teardown runs: by default
    they end Python without it, and the test daemon and tmux server outlive the run."""
    def interrupt(signum, frame):
        raise KeyboardInterrupt
    for sig in (signal.SIGTERM, signal.SIGHUP):
        signal.signal(sig, interrupt)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--daemon", help="the handlined binary to test; by default this checkout's is built")
    ap.add_argument("--json", action="store_true", help="print the table as JSON at the end")
    ap.add_argument("--keep", action="store_true", help="keep the test root (home, daemon log, worktree) for a look afterwards")
    args = ap.parse_args()
    if os.environ.get("HANDLINE_LIVE") != "1":
        print("live-e2e: spends real tokens; HANDLINE_LIVE=1 runs it", file=sys.stderr)
        return 2

    end_on_signals()
    account = live_account.pick_live()
    print(f"live-e2e: account {account['account']}: {account['why']}", flush=True)
    root = Path(tempfile.mkdtemp(prefix="live-e2e.")).resolve()
    daemon, r = None, None
    tmux = f"live-e2e-{os.getpid()}"
    try:
        binary = args.daemon
        if not binary:
            binary = str(root / "handlined")
            subprocess.run(["go", "build", "-o", binary, "./cmd/handlined"], cwd=REPO / "go", check=True)
        home, key = scratch(root, account)
        daemon = start_daemon(binary, root, home, account, tmux)
        d = Daemon(home / "daemon.sock", key)
        seed(d, home, account, root / "work")
        s = Stream(home / "daemon.sock", key)
        r = Run(d, s, Answerer(home / "daemon.sock", key, s, AGENT), AGENT)
        print(f"live-e2e: {AGENT} on a test daemon {d.hello.get('daemon')}, home {home}", flush=True)
        terminal_turns(r, tmux, account["config_dir"], root, daemon)
        print(f"live-e2e: {r.answerer.yeses} permission prompt(s) outside the steps answered yes", flush=True)
    except (Refused, OSError, subprocess.CalledProcessError, KeyboardInterrupt) as e:
        print(f"live-e2e: {str(e) or 'interrupted'}", file=sys.stderr)
        return 2
    finally:
        if daemon:
            stop_daemon(daemon)
        tmuxsock.stop(tmux)
        if not args.keep:
            subprocess.run(["git", "worktree", "remove", "--force", str(root / "work")], cwd=REPO, capture_output=True)
        if args.keep:
            print(f"live-e2e: kept {root}", file=sys.stderr)
        else:
            shutil.rmtree(root, ignore_errors=True)
    if args.json:
        print(json.dumps(r.rows, indent=1))
    return 1 if any(row["result"] == "FAIL" for row in r.rows) else 0


if __name__ == "__main__":
    sys.exit(main())

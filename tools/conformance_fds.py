#!/usr/bin/env python3
"""Run contract/conformance.py against the built Go daemon without a socket file: tools/conformance_fds.py BINARY [--claude CLI] [conformance args]

A sandbox may refuse to bind a socket, so the daemon (`-serve-fds`) serves connections this script made as socketpairs and
passes to it, one per fixture. It starts on a scratch home with a founder key minted where it reads it, which is given to
the harness as --var founder_key. The role `agent` gets socketpairs made by a child process recorded as an agent's session in
that home's store, so the daemon tells their peer to be that agent. The network listener is on (loopback, port 0) and
`-serve-net-fds` serves a second set of socketpairs as its connections: the role `unpaired`. A fixture of a role with no
connection here (member) is reported SKIP and fails the run."""
import fcntl
import os
import re
import json
import secrets
import socket
import sqlite3
import subprocess
import ctypes
import ctypes.util
import struct
import sys
import tempfile
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "contract"))
import conformance  # noqa: E402

SESSION_NS = "session-tokens"  # the protected store's folder of agent session records


def session_line(pid: int) -> str | None:
    """`pid` as a line of an agent's session record, "pid start" (the start time from the kernel); None when no such process runs."""
    if sys.platform == "darwin":
        libproc = ctypes.CDLL(ctypes.util.find_library("proc"))
        buf = ctypes.create_string_buffer(136)
        if libproc.proc_pidinfo(pid, 3, 0, buf, 136) != 136:  # PROC_PIDTBSDINFO
            return None
        start = struct.unpack_from("QQ", buf, 120)  # pbi_start_tvsec, pbi_start_tvusec
        return f"{pid} {start[0]}.{start[1]:06d}"
    try:
        with open(f"/proc/{pid}/stat") as f:
            rest = f.read().rsplit(")", 1)[1].split()
    except (OSError, IndexError):
        return None
    return f"{pid} {rest[19]}"  # starttime in ticks

PAIRS = 512
AGENT_PAIRS = 160
AGENT = "c.agent"  # the agent the role `agent` connects as
LEAD = "c"  # its team's manager, who a done it files for review waits on
# The words a view stores (the spec's x-state-words that fixtures pin): one agent each, as the Python daemon's tick would have left it.
STATES = ["running", "idle", "queued", "parked", "waiting", "stalled", "no_terminal"]


def seed_states(home: Path, daemon: subprocess.Popen, fakes: bool) -> dict[str, str]:
    """An agent per state word: its row and a stored view, which the daemon serves as it is. The schema is the daemon's,
    so the views go in once it has made the database. Returns the --var for each."""
    names = {w: f"s.{w}" for w in STATES}
    output_agent = "s.output"
    ask_agent = ASK_AGENT
    order = {"order_a": names["idle"], "order_b": names["running"], **ORDER}
    for n in [*names.values(), NEVER_STARTED, output_agent, ask_agent, DOING_AGENT, AGENT, LEAD, *RETIRE, *ORDER.values(), *TURNS]:
        agent_dir(home, n)
    for _ in range(200):
        try:
            con = sqlite3.connect(home / "db.sqlite", timeout=5)
            for w, n in [*names.items(), ("idle", NEVER_STARTED)]:
                view = {"name": n, "role": "seeded by conformance_fds", "model": None, "effort": None, "state": w, "state_detail": "",
                        "state_since": None, "block": None, "wall_reason": None, "background_since": None, "would_wake": False,
                        "blocker": None, "account": None, "task": None, "cache_until": None, "has_dialog": w == "waiting",
                        "parent": None, "needs_you": None}
                con.execute("INSERT OR REPLACE INTO agent_view(agent, view, updated) VALUES(?, ?, 0)", (n, json.dumps(view)))
            seed_doing(con)
            seed_retro(con)
            seed_tool_items(con, output_agent)
            seed_session_start(con, output_agent)
            seed_workroom(home, con)
            seed_files(home, con)
            seed_open_question(con, ask_agent)
            typing = seed_typing_gates(home, con)
            seed_agents(con, [*names.values(), *([] if fakes else [NEVER_STARTED]), output_agent, ask_agent, DOING_AGENT, AGENT, LEAD, *RETIRE, *ORDER.values(), *typing.values(), *TURNS,
                              WORKROOM["create_parent"]], {"w": WORKROOM["create_repo"]})
            keys = rekey(home, con)
            con.commit()
            con.close()
            seeded = {**{f"state_{w}": n for w, n in names.items()}, "output_agent": output_agent, "session_agent": output_agent,
                    "ask_agent": ask_agent, "never_started_agent": NEVER_STARTED, "triggers_agent": names["idle"], "doing_agent": DOING_AGENT, "view_plain_agent": names["idle"], "retire_agent": RETIRE[0],
                    "retire_parent": RETIRE[1], "files_agent": FILES_AGENT, **WORKROOM, **typing, **order, "order_rename_to": "o.new",
                    **{f"turn_{k}": n for k, n in zip("abc", TURNS)}}
            # an agent's var is its key, as the daemon names it in what it answers; a name it has yet to take stays a name
            return {k: v if k == "order_rename_to" else keys.get(v, v) for k, v in seeded.items()}
        except sqlite3.Error:
            if daemon.poll() is not None:
                break
            time.sleep(0.05)
    raise RuntimeError("the daemon's database did not come up to seed")


# What agent_create.json and worktree_status.json need: a repo (a git directory with a commit), an agent that works in it, and the
# repo's path as that agent's worktree. create_name is not seeded: a new agent is started in a terminal, which this run has none of, so
# the done, failed and retry fixtures are named in go/conformance-needvar.txt and the Go tests play them over a fake host.
WORKROOM = {"create_repo": "", "create_parent": "w.parent", "status_worktree": ""}


def seed_account(home: Path, name: str) -> Path:
    """The account name's row, owned by p1 as the other seeds are, and its folder, accounts/<key>: named by the row's key."""
    con = sqlite3.connect(home / "db.sqlite", timeout=5)
    try:
        with con:
            cur = con.execute("INSERT INTO accounts(name, owner, created_at) VALUES(?, 'p1', 0)", (name,))
        folder = home / "accounts" / f"c{cur.lastrowid}"
    finally:
        con.close()
    folder.mkdir(parents=True)
    return folder


def seed_workroom(home: Path, con: sqlite3.Connection) -> None:
    repo = home.parent / "workroom"
    repo.mkdir()
    env = {**os.environ, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"}
    for args in (["init", "-q", "-b", "main"], ["commit", "-q", "--allow-empty", "-m", "first"]):
        subprocess.run(["git", *args], cwd=repo, env=env, check=True)
    agent_dir(home, WORKROOM["create_parent"])
    subprocess.run(["git", "remote", "add", "origin", "git@github.com:conf/workroom.git"], cwd=repo, check=True)
    extra = []
    for branch, commits, dirty in (("facts/clean", 0, 0), ("facts/dirty", 2, 2), ("facts/failing", 1, 0), ("facts/ready", 1, 0)):
        path = home.parent / branch.replace("/", "-")
        subprocess.run(["git", "worktree", "add", "-q", "-b", branch, str(path)], cwd=repo, check=True, env=env)
        for i in range(commits):
            (path / f"c{i}.txt").write_text("x")
            subprocess.run(["git", "add", "."], cwd=path, check=True)
            subprocess.run(["git", "commit", "-q", "-m", f"c{i}"], cwd=path, env=env, check=True)
        for i in range(dirty):
            (path / ("c0.txt" if i == 0 else "loose.txt")).write_text("changed")
        extra.append(str(path.resolve()))
    con.execute("INSERT OR REPLACE INTO agent_state(agent, state, updated, worktree, extra_worktrees) VALUES(?, 'idle', 0, ?, ?)",
                (WORKROOM["create_parent"], str(repo), json.dumps(extra)))
    WORKROOM["create_repo"] = WORKROOM["status_worktree"] = str(repo.resolve())


# What files_list.json needs: an agent whose worktree holds these files. build/out.js is ignored, notes/todo.md untracked.
FILES_AGENT = "f.files"
FILES = {".gitignore": "build/\n", "README.md": "hello\n", "docs/domain.md": "x", "lib/main.go": "x", "main/run.sh": "x", "src/app/Main.tsx": "x",
         "src/app/main_test.go": "x", "src/my_animation.ts": "x"}


def seed_files(home: Path, con: sqlite3.Connection) -> None:
    repo = home.parent / "filesroom"
    for rel, text in FILES.items():
        (repo / rel).parent.mkdir(parents=True, exist_ok=True)
        (repo / rel).write_text(text)
    env = {**os.environ, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"}
    for args in (["init", "-q", "-b", "main"], ["add", "-A"], ["commit", "-q", "-m", "first"]):
        subprocess.run(["git", *args], cwd=repo, env=env, check=True)
    for rel in ("notes/todo.md", "build/out.js"):
        (repo / rel).parent.mkdir(parents=True, exist_ok=True)
        (repo / rel).write_text("x")
    con.execute("INSERT OR REPLACE INTO agent_state(agent, state, updated, worktree) VALUES(?, 'idle', 0, ?)", (FILES_AGENT, str(repo)))


def seed_open_question(con: sqlite3.Connection, agent: str) -> None:
    """One open question from the agent to the person, for chat_parent.json."""
    con.execute("INSERT INTO requests(ts, requester, target, body, response, kind, state) VALUES(?, ?, 'human', 'which one?', 'async', 'ask', 'delivered')",
                (int(time.time()), agent))


# The gh the daemon finds first: the pull requests worktrees.list.facts_of_a_branch_whose_checks_fail and ..._with_a_ready_pr read.
GH_STUB = """#!/bin/sh
while [ $# -gt 0 ]; do [ "$1" = --head ] && head=$2; shift; done
case "$head" in
facts/failing) echo '[{"number":901,"url":"https://github.com/conf/workroom/pull/901","state":"OPEN","isDraft":false,"reviewDecision":"","statusCheckRollup":[{"conclusion":"FAILURE"},{"conclusion":"TIMED_OUT"},{"conclusion":"SUCCESS"}]}]';;
facts/ready) echo '[{"number":902,"url":"https://github.com/conf/workroom/pull/902","state":"OPEN","isDraft":false,"reviewDecision":"APPROVED","statusCheckRollup":[{"conclusion":"SUCCESS"}]}]';;
*) echo '[]';;
esac
"""


ASK_AGENT = "c.ask"  # in the agent role's workspace, so it may write to it
# task_turns.json: three agents only its fixtures put on a task, so the turns they take touch no other fixture's agent.
TURNS = ("t.one", "t.two", "t.three")
# agent.get.doing_names_the_tool_call: a running agent whose turn's newest tool call is Bash `sleep 30`, as the tick stores it.
DOING_AGENT = "s.doing"
NEVER_STARTED = "s.never"  # a stored view and no state row; on the fakes its agent row waits for seed_never_started


def seed_retro(con: sqlite3.Connection) -> None:
    """stats_retro.json: founder's task 9001 with 9002 under it, both done; grp.one did 9002 and grp.two 9001, each for 1800 s, with one
    call of grp.one after its window; 9003 answers to an agent."""
    for a in ("grp.one", "grp.two"):
        con.execute("INSERT OR REPLACE INTO agent_state(agent, state, account, worktree, updated) VALUES(?, 'IDLE', 'a', ?, 0)", (a, f"/retro/{a}"))
    con.execute("INSERT OR REPLACE INTO tasks(id, title, owner, opened_by, reporter, created_at, done_at, done_evidence) VALUES(9001, 'retro', 'grp.two', 'human', 'human', 1000, 4600, 'ok')")
    con.execute("INSERT OR REPLACE INTO tasks(id, title, owner, opened_by, reporter, parent_task, created_at, done_at, done_evidence) VALUES(9002, 'retro child', 'grp.one', 'grp.two', 'grp.two', 9001, 1000, 2800, 'ok')")
    con.execute("INSERT OR REPLACE INTO tasks(id, title, owner, opened_by, reporter, created_at, done_at, done_evidence) VALUES(9003, 'retro plain', 'grp.one', 'human', 'grp.two', 1000, 4600, 'ok')")
    for ts, agent, kind, task in ((1000, "grp.one", "task_start", 9002), (2800, "grp.one", "mark_task_done", 9002),
                                  (2800, "grp.two", "task_start", 9001), (4600, "grp.two", "mark_task_done", 9001)):
        con.execute("INSERT INTO events(ts, agent, kind, payload) VALUES(?, ?, ?, ?)", (ts, agent, kind, json.dumps({"task_id": task})))
    for ts, who, n in ((1500, "grp.one", 100), (3000, "grp.one", 500), (3000, "grp.two", 200)):
        con.execute("INSERT OR REPLACE INTO token_calls(account, session, req, ts, model, cwd, side, inp, cc, cr, out, thinking) VALUES('a', 's', ?, ?, 'm', ?, 0, ?, 0, 0, 0, 0)",
                    (f"retro-{who}-{ts}", ts, f"/retro/{who}", n))


def seed_doing(con: sqlite3.Connection) -> None:
    view = {"name": DOING_AGENT, "role": "seeded by conformance_fds", "model": None, "effort": None, "state": "running", "state_detail": "",
            "state_since": None, "block": None, "wall_reason": None, "background_since": None, "would_wake": False, "blocker": None,
            "account": None, "task": None, "cache_until": None, "has_dialog": False, "parent": None, "needs_you": None,
            "doing": {"text": "Bash sleep 30", "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}}
    con.execute("INSERT OR REPLACE INTO agent_view(agent, view, updated) VALUES(?, ?, 0)", (DOING_AGENT, json.dumps(view)))
# agent_retire.json: an agent nothing depends on, which it retires, and a workspace lead with an agent under it, which it may not.
RETIRE = ["r.gone", "r", "r.kid"]
# prefs.json's nav order fixtures: an agent they retire and one a proposal renames (to o.new); neither has a session.
ORDER = {"order_retire_agent": "o.gone", "order_rename_agent": "o.old"}


def agent_dir(home: Path, name: str) -> None:
    """The agent's folder, with the role text its view shows."""
    (home / "agents" / name).mkdir(parents=True, exist_ok=True)
    (home / "agents" / name / "ROLE.md").write_text("seeded by conformance_fds\n")


def seed_agents(con: sqlite3.Connection, names: list[str], folders: dict[str, str]) -> None:
    """Each agent as a row, named by its old dotted name as the daemon reads it until names change: a bare name is the lead of the
    workspace it names, ws.agent an agent of workspace ws, reporting to that lead. The owner (p1) has every one; folders sets a
    workspace's folder. AGENT is in LEAD's workspace, so LEAD is its lead."""
    for n in sorted(set(names), key=lambda n: "." in n):
        ws, _, agent = n.partition(".")
        con.execute("INSERT OR IGNORE INTO workspaces(name, folder, created_at) VALUES(?, ?, 0)", (ws, folders.get(ws, "")))
        ws_id, lead = con.execute("SELECT id, lead FROM workspaces WHERE name = ?", (ws,)).fetchone()
        if not agent and lead:
            continue
        reports_to = f"a{lead}" if agent and lead else "p1"
        cur = con.execute("INSERT INTO agents(workspace, name, owner, reports_to, created_at) VALUES(?, ?, 'p1', ?, 0)",
                          (ws_id, agent or "lead", reports_to))
        if not agent:
            con.execute("UPDATE workspaces SET lead = ? WHERE id = ?", (cur.lastrowid, ws_id))


# The columns that name a who: the seeds write old dotted names and human, and rekey puts each agent's key and p1 in their place.
WHO_COLUMNS = {"agent", "requester", "target", "owner", "opened_by", "reporter", "discarded_by", "author", "pushed_by", "queued_by",
               "accepted_by", "updated_by"}


def agent_keys(con: sqlite3.Connection) -> dict[str, str]:
    """Each agent's key by its old dotted name."""
    return dict(con.execute("SELECT CASE WHEN w.lead = a.id THEN w.name ELSE w.name || '.' || a.name END, 'a' || a.id "
                            "FROM agents a JOIN workspaces w ON w.id = a.workspace").fetchall())


def rekey(home: Path, con: sqlite3.Connection) -> dict[str, str]:
    """Every seeded row and folder that names an agent by its old name named by its key instead, as the daemon writes them (base-v2 1b),
    and the founder by p1. Returns the keys by old name."""
    keys = agent_keys(con)
    tables = [t for (t,) in con.execute("SELECT name FROM sqlite_master WHERE type = 'table'")]
    for t in tables:
        for col in [c[1] for c in con.execute(f"PRAGMA table_info({t})")]:
            if col in WHO_COLUMNS:
                con.execute(f"UPDATE OR REPLACE {t} SET {col} = 'p1' WHERE {col} = 'human'")
                for name, key in keys.items():
                    con.execute(f"UPDATE OR REPLACE {t} SET {col} = ? WHERE {col} = ?", (key, name))
    for agent, view in con.execute("SELECT agent, view FROM agent_view").fetchall():
        v = json.loads(view)
        if v.get("name") in keys:
            v["name"] = keys[v["name"]]
            con.execute("UPDATE agent_view SET view = ? WHERE agent = ?", (json.dumps(v), agent))
    for name, key in keys.items():
        for old, new in ((home / "agents" / name, home / "agents" / key), (home / "locks" / f"{name}.terminal", home / "locks" / f"{key}.terminal")):
            if old.exists() and not new.exists():
                new.parent.mkdir(parents=True, exist_ok=True)
                old.rename(new)  # a lock's flock stays with its file
    return keys


def seed_tool_items(con: sqlite3.Connection, agent: str) -> None:
    """Three Bash calls as the store holds them, for transcript_output.json: output only, output with eight lines cut, no result."""
    now = int(time.time())
    calls = [("printf 'a\\nb\\n'", {"text": "a\nb\n", "truncated": False}),
             ("seq 1 10", {"text": "1\n2\n", "truncated": True, "dropped_lines": 8}),
             ("true", None)]
    seq = 0
    for i, (command, result) in enumerate(calls):
        events = [("tool_use", {"tool_use": {"id": f"u{i}", "tool": "Bash", "input": {"command": command}}})]
        if result:
            events.append(("tool_result", {"tool_result": {"tool_use_id": f"u{i}", "ok": True, **result}}))
        for kind, body in events:
            seq += 1
            con.execute("INSERT OR REPLACE INTO transcript_events(agent, seq, session, turn, at, kind, item, body) VALUES(?,?,?,?,?,?,?,?)",
                        (agent, seq, "seed", None, now, kind, None, json.dumps(body)))


def seed_typing_gates(home: Path, con: sqlite3.Connection) -> dict[str, str]:
    """An agent whose terminal lock is held for the whole run (an flock on its lock file, the one go/kit/lock takes), and one
    whose input another person holds. Returns the --var for each."""
    locked, held = "s.locked", "s.held"
    for n in (locked, held):
        agent_dir(home, n)
        con.execute("INSERT OR REPLACE INTO agent_state(agent, state, updated, terminal) VALUES(?, 'idle', 0, '%9')", (n,))
    (home / "locks").mkdir(exist_ok=True)
    f = open(home / "locks" / f"{locked}.terminal", "a")
    fcntl.flock(f, fcntl.LOCK_EX)
    HELD.append(f)  # kept open, so held, until this process ends
    now = int(time.time())
    con.execute("INSERT OR REPLACE INTO input_holders(agent, kind, name, since, last_typed) VALUES(?, 'person', 'someone else', ?, ?)",
                (held, now, now))
    return {"locked_agent": locked, "held_agent": held}


HELD: list = []


def seed_session_start(con: sqlite3.Connection, agent: str) -> None:
    """A conversation the agent began, bound to its session, for transcript_session.json."""
    con.execute("INSERT INTO session_starts(agent, ts, session_id, cause, resumed, text, prompt, actor) VALUES(?, ?, 'seed', 'launch', 0, '', '', '')",
                (agent, int(time.time())))


def seed_never_started(home: Path) -> None:
    """NEVER_STARTED's agent row, once the tick has settled. The daemon's start gives every agent it finds with no state row an IDLE
    one; seeded before that start has run, it would be found or not by how the two raced."""
    con = sqlite3.connect(home / "db.sqlite", timeout=5)
    try:
        seed_agents(con, [NEVER_STARTED], {})
        rekey(home, con)
        con.commit()
    finally:
        con.close()


def tick_settled(home: Path) -> None:
    """Wait until the tick has rewritten what the run seeded for the tick-off run: s.running's stored view and f.files' worktree. A fixture
    that runs before that sees the seeded rows and one after it the tick's, so the fakes run starts only once both are the tick's; the
    fixtures that pin them are on go/conformance-fakes-skip.txt."""
    for _ in range(600):
        con = sqlite3.connect(home / "db.sqlite", timeout=5)
        try:
            keys = agent_keys(con)
            view = con.execute("SELECT view FROM agent_view WHERE agent = ?", (keys["s.running"],)).fetchone()
            wt = con.execute("SELECT worktree FROM agent_state WHERE agent = ?", (keys[FILES_AGENT],)).fetchone()
        finally:
            con.close()
        if view and json.loads(view[0]).get("state") != "running" and not (wt and wt[0]):
            return
        time.sleep(0.1)
    raise RuntimeError("the tick did not settle the seeded rows within 60 s")


def agent_pairs(store: Path, n: int) -> tuple[list, socket.socket, int]:
    """n socketpairs made by a child process recorded in store as AGENT's session: the kernel names their maker as the peer of
    either end, so the daemon takes a connection on them for one from inside that session. The child lives until the
    returned socket closes, as the parent ends or dies; its pid is returned to be waited for."""
    ctl, child_end = socket.socketpair()
    pid = os.fork()
    if pid == 0:
        try:
            ctl.close()
            pairs = [socket.socketpair() for _ in range(n)]
            for i in range(0, n, 50):
                socket.send_fds(child_end, [b"p"], [s.fileno() for pair in pairs[i:i + 50] for s in pair])
            for pair in pairs:
                for s in pair:
                    s.close()
            child_end.recv(1)
        finally:
            os._exit(0)
    child_end.close()
    fds = []
    while len(fds) < 2 * n:
        _msg, got, _flags, _addr = socket.recv_fds(ctl, 1, 100)
        if not got:
            raise RuntimeError("the agent's session process ended before it made its connections")
        fds += got
    (store / SESSION_NS).mkdir(mode=0o700)
    (store / SESSION_NS / AGENT).write_text(session_line(pid))
    return [(socket.socket(fileno=fds[i]), socket.socket(fileno=fds[i + 1])) for i in range(0, 2 * n, 2)], ctl, pid


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
    binary, rest = argv[0], argv[1:]
    claude = []
    if rest[:1] == ["--claude"]:  # the daemon builds its agent side (the session methods, the supervisor) over this CLI
        claude, rest = ["-claude", rest[1]], rest[2:]
    fakes = rest[:1] == ["--fakes"]  # a -tags fakes binary: agents run on the fake CLI in fake terminals, and the tick runs
    if fakes:
        rest = rest[1:]
    # the service's defaults run the hook-fed views, which this run does not start; the tick only on the fakes
    claude += ["-turns-from-hooks=false", "-notification-needs-you=false"] + ([] if fakes else ["-terminal-tick=false"])
    with tempfile.TemporaryDirectory(prefix="handlined.") as tmp:
        test_root = Path(tmp).resolve()
        home = test_root / "state"
        home.mkdir()
        store = test_root / ".config" / "handline" / "protected" / re.sub(r"[^A-Za-z0-9]", "-", str(home.resolve()))
        (store / "founder-keys").mkdir(parents=True, mode=0o700)
        key = secrets.token_urlsafe(32)
        (store / "founder-keys" / "key").write_text(key)
        (store / "founder-keys" / "key").chmod(0o600)
        (test_root / "bin").mkdir()
        (test_root / "bin" / "gh").write_text(GH_STUB)
        (test_root / "bin" / "gh").chmod(0o755)
        mine, session, holder = agent_pairs(store, AGENT_PAIRS)  # first: the child would hold a copy of any pair made before it
        pairs = [socket.socketpair(socket.AF_UNIX, socket.SOCK_STREAM) for _ in range(PAIRS)]
        net_pairs = [socket.socketpair(socket.AF_UNIX, socket.SOCK_STREAM) for _ in range(PAIRS)]
        theirs = [b.fileno() for _a, b in pairs + mine]
        net_theirs = [b.fileno() for _a, b in net_pairs]
        # the network fixtures pin a fingerprint: a listener with no -cert would serve plain HTTP and have none
        crt, crt_key = test_root / "net.crt", test_root / "net.key"
        subprocess.run(["openssl", "req", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes", "-days", "1",
                        "-subj", "/CN=handline daemon", "-keyout", str(crt_key), "-out", str(crt)], check=True, capture_output=True)
        daemon = subprocess.Popen([binary, "-home", str(home), "-listen", "127.0.0.1:0", "-cert", str(crt), "-key", str(crt_key),
                                   "-serve-fds", ",".join(map(str, theirs)),
                                   "-serve-net-fds", ",".join(map(str, net_theirs)), *claude], pass_fds=theirs + net_theirs,
                                  stderr=subprocess.PIPE,
                                  # the founder's daemon, even when an agent runs the check: it writes the protected store
                                  # OCP_* (the old name of HANDLINE_*) in a session started before the rename names the real home
                                  env={**{k: v for k, v in os.environ.items() if not k.startswith("OCP_")},
                                       "HANDLINE_TEST_ROOT": str(test_root), "HANDLINE_AGENT": "",
                                       **({"HANDLINE_TERMINAL": "fake"} if fakes else {}),
                                       "PATH": f"{test_root / 'bin'}:{os.environ['PATH']}"})
        for _a, b in pairs + net_pairs + mine:
            b.close()

        def taker(pool):
            near = [a for a, _b in pool]

            def connect():
                if not near:
                    raise RuntimeError("out of connections: raise PAIRS")
                return near.pop(0)
            return connect

        local = taker(pairs)
        rc = 1
        try:
            seeded = seed_states(home, daemon, fakes)
            if fakes:
                tick_settled(home)
                seed_never_started(home)
            seeded |= {"add_account": "conf-added", "add_used_account": "conf-used", "add_retry_account": "conf-retried"}  # names no account has
            main_dir = seed_account(home, "main") / ".claude"  # a logged-in account: it has a credential file
            main_dir.mkdir()
            (main_dir / ".credentials.json").write_text("{}")
            seeded |= {"account": "main", "schedule_agent": seeded["ask_agent"], "task_owner": seeded["ask_agent"]}
            rc = conformance.main(["--var", f"founder_key={key}", "--var", "dialogs_retired=1",
                                   *(a for k, v in seeded.items() for a in ("--var", f"{k}={v}")), *rest],
                                  connectors={"founder": local, "anonymous": local, "agent": taker(mine), "unpaired": taker(net_pairs)})
            return rc
        finally:
            daemon.terminate()
            err = daemon.communicate()[1].decode(errors="replace")
            session.close()
            os.waitpid(holder, 0)
            if rc and err.strip():
                print("daemon log:\n" + err, file=sys.stderr)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))

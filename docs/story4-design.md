# Story 4 design: one master, lent machines, several people

Design only; nothing is built. Decisions lists the founder's answers (11-17); Build slices orders the
build. The earlier designs (a leader and replicas of the board on every machine, 10-16 and 11-17 morning) are
replaced: the founder's direction is one master that keeps the board, machines people lend to it, and
people who only connect.

**Names used throughout:** the **master** is the one daemon that keeps the board and decides. A **runner** is
a machine lent to the master: it runs the agents the master sends it and keeps nothing of the board. The
master's own machine is a runner too. A **person** connects with the app; a person may lend their machine,
which makes it a runner, or only connect.

## The shape

The board (tasks, memos, blockers, turns, chat, requests, the team, agents' memory and handoffs, usage) lives
in one SQLite database, on the master. Nothing replicates it: every runner and every app reads and writes it
through the master's API, over the network the gateway already serves (Tailscale today). One writer, so
SQLite stays; a second database server is needed only if the master itself must be two machines, which is
later work.

A runner is the daemon in **runner mode**: no board, a local socket for its own agents, and an API the master
drives. The master decides everything that needs the board (which task, which agent, which account, which
machine) and sends the runner one concrete step at a time: open this terminal with this command in this
folder, press these keys, show me this pane, stop that session. The runner does it and reports what happened
(hooks, transcript usage, the terminal's screen, its load).

- **An agent's calls** (`hl task ...`, `hl say`) on a runner go to the runner's local socket, as today. The
  runner proves which agent is calling the way the daemon does now (the caller descends from a session it
  launched) and forwards the call to the master, which answers as for a local agent.
- **The master moves.** It copies its board to each of the owner's runners as it writes; when it is gone, one
  of them takes its place by itself (section 4). Until then, runners finish the turn they are in and start
  nothing new, and apps say the master is offline.

## Who does what

| Duty | Who |
|---|---|
| Keep the board; every write and read of it | master |
| Pick the task, the agent, the account and the machine for a session | master |
| Triggers, timeouts, deliveries, the reclaim timer | master |
| Build the launch: command, settings, prompt, MCP config | master |
| Open and stop terminals, press keys, capture the screen | the runner the session is on, told by the master |
| Make the agent's worktree from the project's remote; push its branch | that runner |
| Bring the agent's memory down at launch and send its changes up | that runner, to and from the master |
| Record hooks and transcript usage | that runner, sent to the master |
| Its leases (simulator, browser, build slot, port) | that runner, from its own pools; the master sees them in the heartbeat |
| Forward its agents' `hl` calls | that runner |
| Keep a copy of the board, a second or two behind; take over when the master is gone | each of the owner's runners |

## 1. Lending a machine

A person lends a machine with the app ("Lend this Mac") or `hl node join <master address>`, using a pairing
code from the master: the `device.pair` flow, issuing a **node credential** that belongs to that person.

1. The machine's daemon starts in runner mode. If it ran alone before, its own board is set aside unchanged
   and not merged; bringing old tasks over is later work.
2. **The project:** for each workspace, the runner clones the project's repo from its remote into its own
   folder, at join and when a workspace is added. A workspace whose project has no remote runs only on the
   machine that holds its folder.
3. **Accounts:** the runner uses the Claude accounts logged in on it. The master's logins are never copied to
   a runner. The master picks among every runner's accounts by the one-machine rule (5-hour window, week,
   sessions already on it), counting the sessions each runner reports.
4. **Heartbeat:** every few seconds the runner sends its load (cores, memory, free slots, free leases), its
   accounts and their sessions, and the sessions it runs.

A lent machine runs code the master's agents write, so lending is the person's explicit act, shown in their
app, and they can take the machine back at any time (it goes offline, section 3).

## 2. Agents and sessions

An agent is a name, a role, a memory, a handoff and its place in the team: all on the master. It is **not
pinned to a machine**. Each session is placed when it starts: on an online runner that holds the project, has
a free slot and an account with room, and has the lease the work needs (a simulator for app work).

- **Worktree:** the runner keeps one worktree per agent per project, made from the remote and named as today.
  A session placed on another runner starts from the branch's last pushed commit there.
- **Push per commit:** an agent's branch is pushed after each commit, so a session moved elsewhere loses at
  most uncommitted work.
- **Memory:** brought down to the runner at launch, and each change is sent up as it is written. One session
  per agent at a time, so there is one writer.
- **Continuing:** a session that ends normally writes its handoff to the master. The next session, on any
  runner, starts from the handoff, as a new session does today. A transcript is resumed only on the runner
  that holds it.
- **Terminal:** the app attaches through the master, which relays the bytes from the runner.

## 3. A runner goes offline

- **Offline:** 60 s without a heartbeat. Its agents show `offline: <machine> since <time>`; nothing new
  starts there.
- **Reclaim:** after 10 minutes offline (a setting), the master takes back every task in progress on that
  runner. Each goes back to its queue with a memo naming the branch's last pushed commit, and is started on
  another online runner by the usual placement.
- **Return:** the runner comes back online. Sessions it still holds for reclaimed tasks are stopped, and any
  uncommitted or unpushed work in their worktrees is committed to a `rescue/<agent>-<date>` branch and pushed,
  so nothing is lost and nothing runs twice. Its other agents take new work as placement gives it.
- A runner the person took back (left the master) is offline for good: its tasks are reclaimed at once.

## 4. The master dies

The board is copied as it is written, and when the master is gone another of the owner's machines takes its
place by itself.

- **The copy:** Litestream, run as a library inside the master, writes each committed change of the board to
  a folder on the master; each of the owner's runners mirrors that folder through the node API, a second or
  two behind. Other people's runners get no copy, so no one else's machine holds the board's chats and memos.
  The copy is only ever restored, never read from.
- **Taking over:** an owner's runner that has not reached the master for 30 seconds, but does reach the
  internet (`api.anthropic.com`, which its agents need anyway), restores its copy and starts as master the
  way the daemon starts today, with timers and deliveries rebuilt from the board. It raises the **master
  number** by one and tells every runner in the node table. Several runners take over in the order they
  joined: the next one waits until the one before it has been unreachable for 30 seconds too. What the old
  master wrote in its last second or two is lost.
- **Why the internet check:** with two machines, a master that is off and a cut network look the same. A
  machine without the internet does not take over, so a laptop on a plane stays a runner, and its agents,
  which cannot work offline, wait. Two masters happen only when both machines reach the internet but not each
  other, which Tailscale's relays make rare.
- **One master:** runners and apps obey only the highest master number they have seen. An old master that
  comes back learns the higher number from the first runner it reaches and becomes a runner. Its own board is
  set aside unchanged, not merged, and its app says so. The sessions on its machine were reclaimed like any
  offline runner's, and its return is handled as in section 3.
- **A master cut off:** one that has owner runners but has reached neither them nor the internet for 30
  seconds starts nothing new, and its app says that what is written there may be set aside.
- **A planned move:** "Make this Mac the master" on a runner (or `hl node promote`). The master stops its
  writes, waits until the copy has them all, and hands over, so nothing is lost.
- **Apps** keep the list of machines and their device token. When the master is unreachable they ask the
  others which one is master now and connect to it with the same token, which the copy carries.
- **No vote:** a majority vote among three or more machines (Raft, as rqlite does) would also keep the last
  second of writes and rule out two masters, for about 20 more hours. It is not built. If it is ever needed,
  it replaces the copy and the takeover above for three or more machines and keeps everything else here; two
  machines cannot hold a vote, so they keep this takeover.

## 5. People

People and roles exist today (`people`, `devices`; the RPC roles `owner` and `member`). This story widens them.

- **Invite:** the owner invites a person with a pairing link; each person has their own devices and login.
- **Roles:** the owner decides (trust, settings, people, gates); a member gives work, comments, chats and may
  lend machines.
- **Turns:** a turn that waits on a person names that person. Each person's Requests shows the turns that
  wait on them; `--turn human` on a task means the task's owner.
- **Connecting without lending** is the app's connect mode, which stays: the app talks to the master and
  starts no daemon.

## 6. What the app needs from the daemon

1. `node.status` and a `nodes` topic: the master, each runner with its person, online or offline since when,
   load, accounts, sessions.
2. Lend and take back: `node.join`, `node.leave`, and the pairing code.
3. Each agent's current machine in `agents.list` and `agent.changed`; terminal attach relayed.
4. The reclaim time setting.
5. People: invite, list, role, remove; each person's Requests.
6. Each runner's copy and how far behind it is, in `node.status`; `node.promote`; the master number on every
   reply, so an app notices a new master; a board set aside or cut off.

## Decisions

1. **Master:** one, this Mac to start. The board is copied to the owner's runners as it is written; when the
   master has been gone for 30 seconds, the next of them that reaches the internet takes over by itself,
   losing a second or two of writes. This works with two machines.
2. **Reclaim:** offline after 60 s without a heartbeat; tasks reclaimed after 10 minutes (a setting) and
   started on another online runner. Agents push their branch after each commit; a returning runner's leftover
   work goes to a rescue branch.
3. **Accounts:** a runner uses the Claude accounts logged in on it; the master's are not copied.
4. **Project:** each runner clones the project from its remote.
5. **Agents are not pinned:** an agent's memory and handoff are on the master; each session is placed on a
   runner with room.
6. **People:** an owner and members; a member may lend a machine; connecting without lending is the app's
   connect mode.
7. **Board:** SQLite on the master; the owner's runners hold a copy, through Litestream, only to restore from.
   No replicated database: rqlite cannot run the daemon's transactions, dqlite does not run on macOS, and
   younger replication libraries are not trusted with the board yet.

## Build slices

Phase 1 is one person and several machines (the founder's own Macs), including the master that moves; phase
2 adds people. Each slice lands and
is reviewed on its own; "after" names the slices it starts after. Most slices are tested with a master and a
runner on one Mac (slice 3); the last check is two real Macs.

| # | Slice | Area | Hours | After |
|---|---|---|---|---|
| 1 | Node identity and the node table on the master; `node.status` and the `nodes` topic | daemon | 6 | |
| 2 | Lending: `device.pair` issues a node credential owned by a person; `hl node join` and `node.leave`; a joining machine's own board set aside | daemon, hl | 6 | 1 |
| 3 | A master and a runner on one Mac for tests: two daemons with their own homes and ports, joined | tooling | 4 | 2 |
| 4 | Runner mode: no board; the API the master drives (terminal open, keys, screen, stop, worktree make, push); the local socket forwards its agents' calls with the agent proven by process descent | daemon | 16 | 3 |
| 5 | The machine seam on the master: launches, the terminal tick, key presses and stops go through one interface; this machine's own tmux behind it, and a runner's API | daemon | 12 | 4 |
| 6 | Agent files on a runner: the launch's settings, MCP config and prompt written there; memory down at launch and each change up; the handoff written to the master | daemon | 8 | 5 |
| 7 | Hooks and transcript usage recorded on the runner and sent to the master; working and idle state from them | daemon | 8 | 5 |
| 8 | Projects on runners: each workspace's repo cloned from its remote at join and when a workspace is added; the branch pushed after each commit | daemon | 6 | 4 |
| 9 | Heartbeats (load, leases, accounts and their sessions); offline after 60 s; reclaim after the setting's time, back to the queue with the last pushed commit named; a returning runner's leftovers to a rescue branch | daemon | 10 | 5, 8 |
| 10 | Placement: each session on an online runner holding the project, with a slot, an account with room and the lease the work needs | daemon | 8 | 6, 9 |
| 11 | Account choice across runners: the one-machine rule over every runner's accounts and sessions | daemon | 6 | 9, 10 |
| 12 | Terminal attach relayed through the master to the runner | daemon | 6 | 5 |
| 13 | The app's screens designed and shown: Machines in Settings (lend, take back, online or offline, reclaim time, "Make this Mac the master"), each agent's machine | app | 7 | 1 |
| 14 | The app built to them, "Lend this Mac" starting runner mode | app | 10 | 10, 12, 13 |
| 15 | People: invite by pairing link, list, roles, remove; a member may lend | daemon, app | 8 | 2 |
| 16 | Turns per person: a turn names the person it waits on; each person's Requests | daemon, app | 10 | 15 |
| 17 | The board's copy: Litestream inside the master writing to a folder; each of the owner's runners mirrors it; how far behind, in `node.status`; the copy and the takeover kept in one package, so a vote can replace them | daemon | 10 | 4 |
| 18 | Taking over: after 30 s without the master and with the internet, the next runner in join order restores the copy and raises the master number; runners obey the highest; an old master that returns becomes a runner with its board set aside; a cut-off master starts nothing new; a planned move (`node.promote`) loses nothing; tested with a master and two runners on one Mac | daemon, hl | 14 | 9, 17 |
| 19 | The app after a new master: the machine list and token kept, the new master found and joined; "Make this Mac the master"; a set-aside or cut-off board said so | app | 5 | 14, 18 |

About 160 hours: phase 1 (slices 1 to 14 and 17 to 19) about 142, phase 2 about 18. Not in this list: a
majority vote among three or more machines, bringing a set-aside board's tasks over, the phone.

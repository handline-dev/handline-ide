# Story 4 design: one master, lent machines, several people

Design only; nothing is built. Decisions lists the founder's answers (11-17, 11-18, 11-21); Build slices orders
the build. The earlier designs (a leader and replicas of the board on every machine, 10-16 and 11-17 morning) are
replaced: the founder's direction is one master that keeps the board, machines people lend to it, and
people who only connect.

**Names used throughout:** the **master** is the one daemon that keeps the board and decides. A **runner** is
a machine lent to the master: it runs the agents the master sends it and keeps nothing of the board. The
master's own machine is a runner too. A **person** connects with the app; a person may lend their machine,
which makes it a runner, or only connect. Every Mac, server, phone and browser is a **device**, added the same
way and listed in one Settings screen, Devices.

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
- **The master moves by hand.** It copies its board to each of the owner's runners as it writes; when it is
  gone, the owner makes one of them the master (section 4). Until then, runners finish the turn they are in
  and start nothing new, and apps say the master is offline.

## Who does what

| Duty | Who |
|---|---|
| Keep the board; every write and read of it | master |
| Pick the task, the agent and the account for a session, and the agent's device | master |
| Triggers, timeouts, deliveries, the reclaim timer | master |
| Build the launch: command, settings, prompt, MCP config | master |
| Open and stop terminals, press keys, capture the screen | the runner the session is on, told by the master |
| Make the agent's worktree from the project's remote; push its branch | that runner |
| Bring the agent's memory down at launch and send its changes up | that runner, to and from the master |
| Record hooks and transcript usage | that runner, sent to the master |
| Its leases (simulator, browser, build slot, port) | that runner, from its own pools; the master sees them in the heartbeat |
| Say what it has: agent CLIs, MCP programs, folders outside the repo, Xcode and simulators, its Handline version | that runner, at connect and in each heartbeat |
| Forward its agents' `hl` calls | that runner |
| Keep a copy of the board, a second or two behind; become the master when the owner says | each of the owner's runners |

## 1. Lending a machine

A machine is added like any device: Settings > Devices > **Connect a device**, the same pairing code a phone or
a browser uses (`device.pair`), or `hl device join <link>` on a machine without the app, such as a rented
server. The device belongs to the person whose code it redeemed, and it comes in running nothing. A Mac's or a
server's row in Devices has **Run · Pause · Off** (a phone's or a browser's has none), set only by the device's
own person. It comes in Off; Run lends the machine. The master's own Mac is on Run, as today.

1. The machine's daemon starts in runner mode, with its device token as its credential. If it ran alone before, its own board is set aside unchanged
   and not merged; bringing old tasks over is later work.
2. **The project:** for each workspace, the runner clones the project's repo from its remote into its own
   folder, at join and when a workspace is added. A workspace whose project has no remote runs only on the
   machine that holds its folder.
3. **Accounts:** the runner uses the Claude accounts logged in on it. The master's logins are never copied to
   a runner. The master picks among the accounts on the agent's device by the one-machine rule (5-hour
   window, week, sessions already on it); an account logged in on two devices is one account, known by its
   email, and its sessions on both count.
4. **Heartbeat:** every 15 seconds the runner sends its load (cores, memory, free slots, free leases), its
   accounts with each one's newest usage reading (it reads its own, at least every 2 minutes; the master keeps
   the newest of every device's for an email) and their sessions, the sessions it runs, what it has (agent CLIs, MCP programs, folders outside
   the repo, Xcode and simulators) and its Handline version. One at a time: the next goes only after the
   master answered the last, so a slow network never stacks them. A runner on another version than the
   master takes no work until it is updated.
5. **Pause and Off:** Pause starts nothing new there; its sessions finish what they are doing. Off also ends
   them: each agent finishes its turn, writes its handoff and goes on on another device (section 3).
6. **How much it lends:** the memory the agents may use, `agents_memory_max_gb`, which This Mac has today, is
   set per device. A device over it is full: it starts nothing new.
7. **Battery:** a switch at the bottom of Devices: a Mac taken off power goes to Off, and back to Run when it is
   plugged in again. It ships off until the founder has tried it.

A lent machine runs code the master's agents write, so lending is the person's explicit act, shown in their
app, and they can take the machine back at any time with Off. The same Run · Pause · Off for this device is in
the popover of the pause control at the bottom right.

## 2. Agents and sessions

An agent is a name, a role, a memory, a handoff and its place in the team: all on the master. It **stays on
one device while that device has room**, so its worktree, transcript and build caches stay warm. The master
picks a device for it, the one with the most room among those that hold the project and have everything the
agent needs (its MCP programs, folders, a simulator for app work), only when the agent has none, or when a
session starts and the agent's device is full, on Pause or Off, offline past the reclaim time, or lacks what
the agent needs. The agent then stays on the new device. Load spreads at starts, never in the middle of a turn.
A Devices row says what a device lacks for an agent ("ui not here: needs Pencil").

- **Worktree:** the runner keeps one worktree per agent per project, made from the remote and named as today.
  A session placed on another runner starts from the branch's last pushed commit there.
- **Push per commit:** an agent's branch is pushed after each commit, so a session moved elsewhere loses at
  most uncommitted work.
- **Memory:** brought down to the runner at launch, and each change is sent up as it is written. One session
  per agent at a time, so there is one writer.
- **Shared skills, commands and subagents:** the folder every account sees on the master is sent to the runner
  at launch and again when it changes.
- **Attachments:** a file the founder attached is fetched from the master when an agent opens it.
- **Continuing:** a session that ends normally writes its handoff to the master. The next one is on the same
  device and resumes as today; one that starts on another device starts from the handoff, as a new session
  does today, since a transcript does not cross machines.
- **Terminal:** the app attaches through the master, which relays the bytes from the runner.

## 3. A runner goes offline

- **Offline:** 60 s without a heartbeat. Its agents show `offline: <machine> since <time>`; nothing new
  starts there.
- **Reclaim:** after 30 minutes offline (a setting), long enough for a closed lid, the master takes back every
  task in progress on that runner. Each goes back to its queue with a memo naming the branch's last pushed
  commit, and is started on another device by the usual placement.
- **Return:** the runner comes back online. Sessions it still holds for reclaimed tasks are stopped, and any
  uncommitted or unpushed work in their worktrees is committed to a `rescue/<agent>-<date>` branch and pushed,
  so nothing is lost and nothing runs twice. Its other agents take new work as placement gives it.
- **Off** is not offline: the runner is still there, so each of its agents finishes its turn and writes its
  handoff, and its tasks start elsewhere at once. A revoked device is reclaimed at once.

## 4. The master dies

The board is copied as it is written, and when the master is gone the owner makes another of their machines
the master.

- **The copy:** Litestream, run as a library inside the master, writes each committed change of the board to
  a folder on the master; each of the owner's runners mirrors that folder through the node API, a second or
  two behind. Other people's runners get no copy, so no one else's machine holds the board's chats and memos.
  The copy is only ever restored, never read from. It is `go/internal/boardcopy`: the master writes
  `<home>/board-copy` (a snapshot an hour, two hours kept), each owner's runner pulls what it lacks every second
  (`node.copy.files`), and `node.status` shows per runner how long since its copy last held all of it.
- **Taking over, by hand:** "Make this Mac the master" on one of the owner's runners (or `hl node promote`).
  When the master is reachable, it stops its writes, waits until the copy has them all and hands over, so
  nothing is lost. When it is gone, the runner restores its copy and starts as master the way the daemon
  starts today, with timers and deliveries rebuilt from the board; what the old master wrote in its last
  second or two is lost. Either way it raises the **master number** by one. Runners do not listen, so it does
  not tell them: a runner whose link drops asks each of the owner's machines in the node table (its address and
  certificate fingerprint, which each heartbeat's answer lists) and joins the one that is master with the
  highest number.
- **Where the master lives:** on the device that stays on (a desktop or a server). A laptop master that sleeps
  stops new work everywhere until it wakes or the owner moves the master.
- **One master:** runners and apps obey only the highest master number they have seen. An old master that
  comes back learns the higher number so: at its start, before it fires a timer, it asks the owner's machines
  in its node table; and while it was gone, the new master dials its address each 15 s until it answers, for an
  old master that never restarts (a laptop woken from sleep, a cut link healed). Each master keeps a device of
  its own, which the copy carries, so either side proves itself with a device token. It then becomes a runner
  with that device. Its own board is set aside unchanged, not merged, and its app says so. The sessions on its machine were reclaimed like any
  offline runner's, and its return is handled as in section 3.
- **Apps** keep the list of machines and their device token. When the master is unreachable they redial that
  list, pinning each certificate, and connect with the same token, which the copy carries, to the one that is
  master with the highest number (in its hello).
- **Later: taking over by itself.** A runner that has not reached the master for 30 seconds but reaches the
  internet could take over on its own. No agent tool surveyed does this (research section 6), and with two
  machines that cannot reach each other it is the one way two masters appear. It is a later slice, behind an
  Auto/Manual setting that ships on Manual.
- **No vote:** a majority vote among three or more machines (Raft, as rqlite does) would keep the last second
  of writes and rule out two masters, for about 20 more hours. It is not built; if it is ever needed, it
  replaces the copy and keeps everything else here.

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
2. Lend and take back: a device's Run · Pause · Off, set only by the device's own person, its memory cap and
   the battery switch; the pairing code is the one Connect a device shows.
3. Each agent's device in `agents.list` and `agent.changed`, and what a device lacks for an agent; terminal
   attach relayed.
4. The reclaim time setting.
5. People: invite, list, role, remove; each person's Requests.
6. Each runner's copy and how far behind it is, in `node.status`; `node.promote`, also when the master is
   gone; the master number on every reply, so an app notices a new master; a board set aside.

## Decisions

1. **Master:** one, this Mac to start, best on a device that stays on. The board is copied to the owner's
   runners as it is written; the owner moves the master by hand, also when it is gone, losing a second or two
   of writes then (11-21; taking over by itself is later work).
2. **Reclaim:** offline after 60 s without a heartbeat; tasks reclaimed after 30 minutes (a setting; 10 until
   11-21) and started on another device. Off hands them over at once. Agents push their branch after each
   commit; a returning runner's leftover work goes to a rescue branch.
3. **Accounts:** a runner uses the Claude accounts logged in on it; the master's are not copied.
4. **Project:** each runner clones the project from its remote.
5. **An agent stays on one device while it has room (11-21):** its memory and handoff are on the master; it
   moves, at a session's start, when its device is full, on Pause or Off, offline past the reclaim time or
   lacks what it needs.
6. **People:** an owner and members; a member may lend a machine; connecting without lending is the app's
   connect mode.
7. **Board:** SQLite on the master; the owner's runners hold a copy, through Litestream, only to restore from.
   No replicated database: rqlite cannot run the daemon's transactions, dqlite does not run on macOS, and
   younger replication libraries are not trusted with the board yet.
8. **One Devices screen (11-18):** Macs, servers, phones and browsers are all devices, added by Connect a
   device. A Mac's row has the lending control (Run · Pause · Off since 11-21) and opens what This Mac shows
   today (resources, agents, simulators, keep awake) for that Mac, so there is no This Mac or Machines screen
   of its own.
9. **Run · Pause · Off per device (11-21):** Pause starts nothing new, Off hands the work over; the share is
   the memory cap per device, not a percentage; going Off on battery is a switch that ships off. Lending only
   at night and resting while the person uses the machine are later work.
10. **What a device has (11-21):** it says what it has and its version; an agent goes only where all it needs
    is, and a device on another version takes no work. Shared skills, commands and subagents are sent to it;
    attachments are fetched when opened.
    A lent Mac of the master's OS and architecture on another version takes the master's own build (handlined
    and hl, sha256 checked) and starts again on it, its sessions going on; Auto by default, Manual waits for its
    row's Update; another person's machine takes it only at its own person's Update (11-22).

## Build slices

Phase 1 is one person and several machines (the founder's own Macs), including the master that moves; phase
2 adds people. Each slice lands and
is reviewed on its own; "after" names the slices it starts after. Most slices are tested with a master and a
runner on one Mac (slice 3); the last check is two real Macs.

| # | Slice | Area | Hours | After |
|---|---|---|---|---|
| 1 | Node identity and the node table on the master; `node.status` and the `nodes` topic | daemon | 6 | |
| 2 | Lending: a device belongs to the person whose code it redeemed; a Mac's or a server's Run · Pause · Off, set only by that person: Run makes it a runner, Pause starts nothing new there, Off takes it back; `hl device join` for a machine without the app; a lending machine's own board set aside | daemon, hl | 8 | 1 |
| 3 | A master and a runner on one Mac for tests: two daemons with their own homes and ports, joined | tooling | 4 | 2 |
| 4 | Runner mode: no board; the API the master drives (terminal open, keys, screen, stop, worktree make, push); the local socket forwards its agents' calls with the agent proven by process descent | daemon | 16 | 3 |
| 5 | The machine seam on the master: launches, the terminal tick, key presses and stops go through one interface; this machine's own tmux behind it, and a runner's API | daemon | 12 | 4 |
| 6 | Agent files on a runner: the launch's settings, MCP config and prompt written there; memory down at launch and each change up; the shared skills, commands and subagents sent and re-sent when they change; attachments fetched when opened; the handoff written to the master | daemon | 11 | 5 |
| 7 | Hooks and transcript usage recorded on the runner and sent to the master; working and idle state from them | daemon | 8 | 5 |
| 8 | Projects on runners: each workspace's repo cloned from its remote at join and when a workspace is added; the branch pushed after each commit | daemon | 6 | 4 |
| 9 | Heartbeats (load, leases, accounts with their newest usage readings and their sessions, what it has, its version); another version takes no work; offline after 60 s; reclaim after the setting's time (30 min), back to the queue with the last pushed commit named; Off hands over at once; the battery switch; a returning runner's leftovers to a rescue branch | daemon | 12 | 5, 8 |
| 10 | Placement: an agent stays on its device; it gets one, the one with the most room among those that hold the project and have all it needs, when it has none or its device is full (memory cap, slots, leases), on Pause or Off, offline or lacking; what a device lacks for an agent | daemon | 6 | 6, 9 |
| 11 | Account choice on a runner: the one-machine rule over the agent's device's accounts, an account logged in on two devices counted once | daemon | 4 | 9, 10 |
| 12 | Terminal attach relayed through the master to the runner | daemon | 6 | 5 |
| 13 | The app's screens designed and shown: Settings > Devices with one row per device; a Mac's row with Run · Pause · Off, its memory cap, online or offline, what it lacks for an agent, and what This Mac shows today for that Mac; the battery switch; reclaim time; "Make this Mac the master"; the pause popover; each agent's device | app | 8 | 1 |
| 14 | The app built to them: This Mac folded into Devices, Run · Pause · Off and the pause popover, Run starting runner mode | app | 12 | 10, 12, 13 |
| 15 | People: invite by pairing link, list, roles, remove; a member may lend | daemon, app | 8 | 2 |
| 16 | Turns per person: a turn names the person it waits on; each person's Requests | daemon, app | 10 | 15 |
| 17 | The board's copy: Litestream inside the master writing to a folder; each of the owner's runners mirrors it; how far behind, in `node.status`; the copy and the takeover kept in one package, so a vote can replace them | daemon | 10 | 4 |
| 18 | Taking over by hand: `node.promote` on an owner's runner; with the master reachable it loses nothing, without it the runner restores the copy; either way the master number goes up; runners obey the highest; an old master that returns becomes a runner with its board set aside; tested with a master and two runners on one Mac | daemon, hl | 6 | 9, 17 |
| 19 | The app after a new master: the machine list and token kept, the new master found and joined; "Make this Mac the master"; a set-aside board said so | app | 4 | 14, 18 |
| 20 | The desktop app on a runner Mac: the window and the shell's own connection go to the master its daemon obeys (the joined master and the owner's machines it was told of, each certificate pinned), with this machine's device token; with none answering, the page on the master it joined says "Can't reach the master" with "Make this Mac the master"; made the master, the app starts again on this Mac's own door | app | 4 | 19 |
| 22 | A lent Mac runs the master's build: a heartbeat's answer names the master's build to a runner of its platform on another version (node_build_auto, or its row's Update, device.update); the runner reads it over its link (node.build.read), checks each sha256, keeps the newest under its home and starts again on it, and at a later start runs it while its master is on that version | daemon | 6 | 9 |
| 23 | Cluster names: every cluster has a name, the global setting `cluster_name` kept on the board (so a new master keeps it), two random words in Title Case given at the first start without one and renamed by the owner only; hello's capabilities and `node.status` give the name with how many machines the cluster has, so a joined app draws a device or a cluster without another call | daemon | 2 | 1 |

About 169 hours: phase 1 (slices 1 to 14, 17 to 20, 22 and 23) about 151, phase 2 about 18. Not in this list:
taking over by itself, a majority vote among three or more machines, lending only at night, resting while the
person uses the machine, bringing a set-aside board's tasks over, the phone.

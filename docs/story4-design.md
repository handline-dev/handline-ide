# Story 4 design: one master, lent machines, several people

Slices 1 to 27 are built. Decisions lists the founder's answers (11-17 to 11-24); Build slices orders the build.
The earlier designs (a leader and replicas of the board on every machine, 10-16 and 11-17 morning) are
replaced: the founder's direction is one master that keeps the board, machines people lend to it, and
people who only connect. Since 11-24 (section 7) light agents stay on the master's machine and heavy ones go,
whole, to the lent machine with the most room, using Handline's own browser and simulator tools there.

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

| Duty                                                                                                            | Who                                                                    |
| --------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| Keep the board; every write and read of it                                                                      | master                                                                 |
| Pick the task, the agent and the account for a session, and the agent's device                                  | master                                                                 |
| Triggers, timeouts, deliveries, the reclaim timer                                                               | master                                                                 |
| Build the launch: command, settings, prompt, MCP config                                                         | master                                                                 |
| Open and stop terminals, press keys, capture the screen                                                         | the runner the session is on, told by the master                       |
| Make the agent's worktree from the project's remote; push its branch                                            | that runner                                                            |
| Bring the agent's memory down at launch and send its changes up                                                 | that runner, to and from the master                                    |
| Record hooks and transcript usage                                                                               | that runner, sent to the master                                        |
| Its leases (simulator, browser, build slot, port)                                                               | that runner, from its own pools; the master sees them in the heartbeat |
| Say what it has: agent CLIs, the programs it is asked about, Xcode and simulators, its Handline version         | that runner, at connect and in each heartbeat                          |
| Forward its agents' `hl` calls                                                                                  | that runner                                                            |
| Keep a copy of the board, a second or two behind; become the master when the owner says                         | each of the owner's runners                                            |

## 1. Lending a machine

A machine is added like any device: Settings > Devices > **Connect a device**, the same pairing code a phone or
a browser uses (`device.pair`), or `hl device join <link>` on a machine without the app, such as a rented
server. The device belongs to the person whose code it redeemed, and it comes in running nothing. A Mac's or a
server's row in Devices has **Run · Pause · Off** (a phone's or a browser's has none), set only by the device's
own person. It comes in Off; Run lends the machine. The master's own Mac is on Run, as today.

1. A Mac with the app joins from the app (slice 24): its own daemon keeps its board, and the app runs a separate engine for that
   cluster beside it, a daemon in a home of its own in runner mode with its device token as its credential; nothing is set aside.
   A machine joined with `hl device join` runs its own daemon in runner mode; if it ran alone before, its own board is set aside
   unchanged and not merged; bringing old tasks over is later work.
2. **The project:** for each workspace, the runner clones the project's repo from its remote into its own
   folder, at join and when a workspace is added. A workspace whose project has no remote runs only on the
   machine that holds its folder.
3. **Accounts:** the runner uses the Claude accounts logged in on it. The master's logins are never copied to
   a runner. The master picks among the accounts on the agent's device by the one-machine rule (5-hour
   window, week, sessions already on it); an account logged in on two devices is one account, known by its
   email, and its sessions on both count.
4. **Heartbeat:** every 15 seconds the runner sends its load (cores, memory, free slots, free leases), its
   accounts with each one's newest usage reading (it reads its own, at least every 2 minutes; the master keeps
   the newest of every device's for an email) and their sessions, the sessions it runs, what it has (agent CLIs, the programs
   it is asked about, Xcode and simulators) and its Handline version. One at a time: the next goes only after the
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

An agent is a name, a role, a memory, a handoff and its place in the team: all on the master. A light agent
stays on **the master's machine** (11-24); a heavy one goes to the lent device with the most room among those
that hold the project and run its agent CLI, at a session's start (section 7). It comes back once it is no
longer heavy and the master's machine has room. Load spreads at starts, never in the middle of a turn. On
Manual an expert pins an agent to a device instead. Its tools (MCP servers, build tools) are its own and never
checked per device (section 7).

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
- **Taking over, by hand:** "Make this Mac the master" on one of the owner's runners (or `hl node promote`);
  in the Mac app it acts on the engine of that cluster, which becomes its master in its own home (slice 27).
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
- **Connecting without lending** is joining on Off: the Mac's engine for that cluster runs nothing until its
  person sets it to Run.

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
7. Each agent's load and why it is where it is (heavy, or kept here for what a device lacks); the Auto/Manual
   setting, and on Manual each agent's pinned device.

## 7. Heavy agents on lent devices, with Handline's own tools (11-24)

Light agents stay on the master's machine; the heavy ones go, whole, to the lent device with the most room. The
founder's direction (T3890 8654, 8664, 8667; T9440 8642): nobody should have to think about it, Handline picks,
an expert can set it by hand. Why whole agents and not single commands: the heavy work sits in a few agents
(over 7 days lander did 48% of this team's heavy command time, the top four 84%), moving an agent is already
built, and an agent that drives a simulator over many commands keeps it beside it.

- **Load:** each agent's heavy command time over the last 7 days (the presets `@build-test` and `@simulator`,
  and any command seen over 60 s, from the `PreToolUse` and `PostToolUse` hooks) and its sessions' peak memory
  with what they started (a simulator, a browser). Found, not set.
- **Placement:** at a session's start the heaviest agents go to the device with the most room that holds the
  project and runs their agent CLI; the rest stay. The rule is replayed on the last 7 days before it is on.
- **Tools inside Handline:** another agent IDE ships its browser, screen and simulator tools inside its app, so
  a second machine installs nothing for them (T3890 8664). Handline does the same: `hl browser` and `hl sim`
  ship in the app and need nothing beyond what Apple and Google require (Xcode, the Android SDK). They are
  commands with skills, not MCP servers: a skill is read when needed, an MCP server's tools sit in every
  session. The browser installed by hand today (Playwright MCP in `~/.handline/browser`) is removed after.
- **Skills installed by themselves (T3890 8667):** Handline writes its own skills into the shared skills folder
  every account sees, which runners already receive, and replaces them with each version; it never asks and
  never touches a skill it did not write.
- **What a lent Mac still needs:** a Claude login once. Everything else an agent uses there, build tools and
  MCP servers alike, is the agent's own, never Handline's (T3890 8673, T9452 8749): Handline neither checks nor
  shows any of them per device. A session gets the shared MCP config as it is; a server whose program is not
  on that device does not start, and the session goes on without it. The agent installs what it needs, and
  asks the founder with `hl propose run` for what needs a person (Xcode's Apple login, another company's app).
  Placement only puts first a device that already has the programs the agent's heavy commands ran, so Xcode
  is not installed twice.
- **Auto and Manual:** one setting in Settings > Devices, Auto by default. Manual pins an agent to a device.

## Decisions

1. **Master:** one, this Mac to start, best on a device that stays on. The board is copied to the owner's
   runners as it is written; the owner moves the master by hand, also when it is gone, losing a second or two
   of writes then (11-21; taking over by itself is later work).
2. **Reclaim:** offline after 60 s without a heartbeat; tasks reclaimed after 30 minutes (a setting; 10 until
   11-21) and started on another device. Off hands them over at once. Agents push their branch after each
   commit; a returning runner's leftover work goes to a rescue branch.
3. **Accounts:** a runner uses the Claude accounts logged in on it; the master's are not copied.
4. **Project:** each runner clones the project from its remote.
5. **An agent stays on the master's machine (11-24; 11-21 kept it on any one device while it had room):** it
   moves to a lent device at a session's start only when the master's machine is full, and comes back once
   there is room; on Manual an expert pins it.
6. **People:** an owner and members; a member may lend a machine; connecting without lending is joining on Off
   (11-22; the app's connect mode is gone).
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
10. **What a device has (11-21):** it says what it has and its version; an agent goes only where its project
    and its agent CLI are (11-24: its other tools are its own), and a device on another version takes no work.
    Shared skills, commands and subagents are sent to it; attachments are fetched when opened.
    A lent Mac of the master's OS and architecture on another version takes the master's own build (handlined
    and hl, sha256 checked) and starts again on it, its sessions going on; Auto by default, Manual waits for its
    row's Update; another person's machine takes it only at its own person's Update (11-22).
11. **Several clusters in one list (11-22):** a Mac's app shows its own work and every cluster it joined in one
    list, each cluster named (a random name, changed in Settings); a joined cluster's menu has only Leave, and lending
    is that cluster's Settings > Devices Run · Pause · Off.
12. **Heavy agents leave, whole (11-24, T3890 8654):** an agent's load is found from its last 7 days; at a
    session's start the heaviest go to the device with the most room, the rest stay on the master's machine. A
    Mac with much memory takes the simulator agents without being set.
13. **Auto, and Manual for experts (11-24, T9440 8642):** one setting, Auto by default; Manual pins an agent.
14. **Handline's own tools, skills installed by themselves (11-24, T3890 8664, 8667):** `hl browser` and `hl sim`
    ship in the app as commands with skills; Handline installs its skills for every agent without asking.
15. **An agent's tools are its own (11-24, T9452 8749):** placement goes by load and room; no MCP server or
    program is checked or shown per device, and a session's MCP config is the same on every device. Ends the
    MCP part of slice 29 and T9465's per-device filter (slice 34).
16. **Adding a device, one link (11-25, T9477 8802, 8811, 8813, 8815, 8816):** Connect a device asks nothing:
    Handline picks the address, the Tailscale name while Tailscale is up, else this Mac's Wi-Fi address, and shows
    one link with its QR and code. The link is a browser address; `handline://` shows on no screen, only the link's
    page uses it, for Open in the Handline app beside Use in this browser. A browser on another device comes only
    through Tailscale, on Tailscale's own certificate for the name once the tailnet has HTTPS (plain `http://` on
    the name until then). The apps keep `ws://` on the tailnet, whose WireGuard already proves the machine, and the
    Tailscale door answers both on one port, so nothing paired before HTTPS moves. A Wi-Fi address is for Mac apps
    only, which pin its certificate: a browser's warning for a self-made certificate cannot tell a fake server from
    this Mac, and a code taken there runs agents here, so a Wi-Fi link shows no QR. A
    link never names a host the apps refuse (the bare Tailscale IP is named by its MagicDNS name), and `hl device
    join` trusts a link as the app does. Another Mac's app has a visible Join a Mac that takes the link or the
    code and asks once: Control from this Mac, or Let the cluster run agents on this Mac (Run at once). A device's
    page has Make master, pressed on the master or on that device. The device list drops the state word the Run ·
    Pause · Off buttons already say, puts a reason that does not fit behind a warning mark with the whole reason on
    hover, draws Mac or server as an icon, and says Controls only for a browser.

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
| 24 | Joining a cluster from the Mac app: a link from that cluster's Connect a device; `hl device join` on an engine home of its own (on Off) run beside this Mac's daemon, which keeps its board; the page gets the clusters, join and Leave; the connect mode and the runner-Mac window go | app | 6 | 22 |
| 26 | Update a device's app from Devices: an engine's heartbeat says its Mac app's version and the newer one its update channel has ready (app.json, which the shell writes); the master's row says Update ready with its Update (device.update); the heartbeat's answer then has the app install it (app_update, kept in app-update.json for the shell), at once for Update, on Auto (node_build_auto, the owner's Mac) once no window is open there; the engine and its sessions go on through the restart | app | 4 | 24 |
| 27 | Make this Mac the master in the Mac app: `node.promote` on the engine of that cluster, which listens on a port kept for it and serves the network_addresses chosen in this Mac's own home; made the master, the cluster stays listed at its door on this Mac and cannot be left; the app's own daemon is never a runner, so the own-runner path goes | app | 2 | 24 |

| 28 | Each agent's load (agentload.Of): heavy command time over the last 7 days in heavy_runs, by shape only (a preset however short, else its program and first word past 60 s); a Bash call timed from its PreToolUse to its PostToolUse (not while a person answers its prompt), a background shell from its start to its output's exit line, kept at the next Stop; the most memory its processes held in a minute (load_minutes), a simulator counted as the agent's whose simulator command ended nearest its start | daemon | 4 | |
| 29 | Placement by load: the heaviest agents go to the device with the most room, the rest stay on the master's machine; replayed on the last 7 days first; Auto/Manual, a pinned agent on Manual | daemon | 6 | 28 |
| 30 | Handline's own skills installed for every agent by themselves, replaced with each version, a user's own left alone | daemon, hl | 3 | |
| 31 | `hl browser`: Handline's browser tool shipped in the app, its own profile per agent, with its skill; the hand-installed browser removed after | hl, app | 8 | 30 |
| 32 | `hl sim`: Handline's iOS Simulator and Android emulator tool, nothing to install beyond Xcode and the Android SDK, held as the `@simulator` lease, with its skill | hl | 10 | 30 |
| 33 | Devices shows where each agent runs and why, and Auto/Manual with a pinned agent: designed, then built | app | 7 | 29 |
| 34 | An agent's tools are its own: placement and the launch stop reading MCP servers (the agent's and the shared ones it used, mcp_used, the runner's per-program filter); a device is told only the programs heavy commands ran; Devices shows no per-tool mark | daemon, app | 2 | 29, 33 |
| 35 | One link for a device: device.pair picks the address (the Tailscale name while Tailscale is up, else the first Wi-Fi address) and gives the browser link only; the Tailscale door serves Tailscale's own certificate for the name when the tailnet has HTTPS, renewed by itself, and a browser reaches it over a verified wss while the apps keep ws on the tailnet, on the same port; Wi-Fi doors serve no web page; `hl device pair` prints the link and `hl device join` takes it, trusting a link as the app does | daemon, hl | 6 | |
| 36 | Joining from another Mac's app: the link's page says Open in the Handline app or Use in this browser; the Mac app opens `handline://` links; a visible Join a Mac takes the link or the code; one question, Control from this Mac or Let the cluster run agents on this Mac (Run at once); `handline://` gone from every screen | app | 6 | 35 |
| 37 | Make master on a device's page: on the master it asks that device to take over, on the device it is `node.promote`; one confirm line | daemon, app | 3 | |
| 38 | The device list and Connect a device to the T9477 pictures: no state word, a warning mark with the reason on hover, an icon for Mac or server, Controls only for a browser, no address to pick | app | 3 | 35 |

About 239 hours: phase 1 (slices 1 to 14, 17 to 20, 22 to 24, 26 and 27) about 163, phase 2 about 18, heavy
agents and Handline's tools (28 to 34) about 40, adding a device with one link (35 to 38) about 18. Not in this list: taking over by itself, a majority vote among three or more machines,
lending only at night, resting while the person uses the machine, bringing a set-aside board's tasks over, the
phone, sending single heavy commands to another machine.

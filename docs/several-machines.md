# Several machines: one master, lent machines

The **master** is the one daemon that keeps the board and decides. A **runner** is a machine lent to the master: it runs
the agents the master sends it and keeps nothing of the board. The master's own machine runs agents too. A **person**
connects with the app and may lend their machine or only connect. Every Mac, server, phone and browser is a **device**,
added the same way and listed in Settings > Devices. A master and its runners are a **cluster**, with a name
(`cluster_name`, two random words at the first start, renamed by the owner).

## The shape

The board (tasks, comments, blockers, turns, chat, requests, the team, agents' memory and handoffs, usage) lives in one
SQLite database on the master. Every runner and app reads and writes it through the master's API, over the network the
daemon serves (Tailscale).

A runner is the daemon in runner mode: no board, a local socket for its own agents, and an API the master drives
(`go/internal/runner`; the master's side is `go/internal/nodes`). The master decides everything that needs the board
(which task, agent, account and machine) and sends the runner one concrete step at a time: open this terminal with this
command in this folder, press these keys, show this pane, stop that session. The runner does it and reports back (hooks,
transcript usage, the terminal's screen, its load). An agent's `hl` calls on a runner go to the runner's local socket;
the runner proves which agent is calling (the caller descends from a session it launched) and forwards the call to the
master.

| Duty | Who |
|---|---|
| Keep the board; pick the task, agent, account and device; triggers, timeouts, deliveries, the reclaim timer; build the launch | master |
| Open and stop terminals, press keys, capture the screen | the runner the session is on |
| Make the agent's worktree from the project's remote; push its branch after each commit | that runner |
| Bring the agent's memory and the shared skills down at launch, send memory changes up; record hooks and usage | that runner |
| Its leases (ports and shared runs), from its own pools | that runner, shown in its heartbeat |
| Keep a copy of the board, a second or two behind | each of the owner's runners |

## 1. Lending a machine

1. A machine joins with the link from Connect a device: the Mac app's Join with a link, or `hl device join <link>` on a
   machine without the app. It belongs to the person whose code it redeemed and comes in on **Off**. In the Mac app a
   joined cluster runs in an engine of its own (a daemon in runner mode, in a home of its own) beside this Mac's daemon,
   which keeps its board. A machine joined with `hl device join` sets its own board aside, unchanged and not merged.
2. **Run · Pause · Off**, on a Mac's or a server's row, set only by that device's person. Run lends the machine; Pause
   starts nothing new there; Off also ends its sessions: each agent finishes its turn, writes its handoff and goes on on
   another device.
3. **Accounts:** a runner uses the Claude accounts logged in on it; the master's logins are never copied. An account
   logged in on two devices is one account, and its sessions on both count.
4. **Heartbeat:** a runner sends one when its link comes up, then 15 s after each answer, so one is in flight at a time:
   its load, its accounts with their newest usage readings and sessions, the sessions it runs, what it has (agent CLIs,
   the programs it is asked about, Xcode and simulators) and its Handline version. A runner on another version takes no
   work. One of the master's OS and architecture takes the master's own build (`node.build.read`, sha256 checked) and
   starts again on it: at once on Auto (`node_build_auto`), else at its row's Update (`device.update`).
5. **Projects:** the runner clones each workspace's repo from its remote. A workspace with no remote runs only on the
   machine that holds its folder.
6. **How much it lends:** a memory cap per device (`agents_memory_max_gb` on the master, the device's own cap on a lent
   one). A device at its cap starts nothing new.
7. **Battery:** with `node_battery_switch` on, a Mac taken off power goes from Run to Off, and back when plugged in.

## 2. Agents and sessions

An agent (name, role, memory, handoff, place in the team) lives on the master. Where its next session opens is
placement's (section 7). A session placed on another runner starts from the branch's last pushed commit there, from the
handoff, since a transcript does not cross machines. Attachments are fetched from the master when an agent opens one. The
app attaches to a runner's terminal through the master, which relays the bytes.

## 3. A runner goes offline

- **Offline:** 60 s without a heartbeat. Nothing new starts there.
- **Reclaim:** after `node_reclaim_s` offline (30 minutes by default), the master takes back every task in progress on
  that runner. Each goes back to its queue with a comment naming the branch's last pushed commit, and starts elsewhere.
  Off and a revoked device are reclaimed at once.
- **Return:** sessions the runner still holds for reclaimed tasks are stopped, and any uncommitted or unpushed work in
  their worktrees is committed to a `rescue/<agent>-<date>` branch and pushed, so nothing is lost and nothing runs twice.

## 4. The master moves

- **The copy** (`go/internal/boardcopy`): Litestream, as a library inside the master, writes each committed change of the
  board to `<home>/board-copy`; each of the owner's runners pulls what it lacks every second (`node.copy.files`).
  `node.status` shows per runner how far behind its copy is. Other people's runners get no copy. The copy is only ever
  restored, never read from.
- **Taking over, by hand:** Make master on a device's page, or `hl node promote` on one of the owner's machines. With the
  master reachable, it stops its writes, waits until the copy has them all and hands over, losing nothing. Without it,
  the runner restores its copy and starts as master; the last second or two of writes is lost. Either way the
  **master number** goes up by one.
- **One master:** runners and apps obey only the highest master number they have seen. A runner whose link drops asks
  each of the owner's machines in its node table and joins the master with the highest number. An old master that comes
  back learns the higher number, becomes a runner with its own device, and sets its board aside unchanged.
- **Apps** keep the list of the owner's machines and their device token; when the master is unreachable they redial
  that list, pinning each certificate, and connect to the master with the highest number.
- **Where the master lives:** on a device that stays on. A laptop master that sleeps stops new work everywhere until it
  wakes or the owner moves the master.

## 5. People

The owner decides (trust, settings, people, gates); a member gives work, comments, chats and may lend machines. Each
person has their own devices. A turn that waits on a person names that person.

## 6. What the app reads

`node.status` and the `nodes` topic (the master, each runner with its person, online or offline, load, accounts,
sessions, its copy's lag); a device's Run · Pause · Off, memory cap and the battery switch; each agent's device and why
it is there; `node.promote`; the master number on every reply.

## 7. Placement by load (`go/internal/placement`)

- **Load** (`go/scheduler/agentload`): each agent's heavy command time over the last 7 days (the presets `@build-test`
  and `@simulator`, and any command over 60 s, timed from its `PreToolUse` to its `PostToolUse` hook) and the most
  memory its sessions held. An agent is heavy at 2 hours of heavy command time or 6 GB of peak memory.
- **Auto** (`agent_placement`, the default): at a session's start a light agent stays on the master's machine; a heavy
  one goes to the lent device with the most room among those that hold the project and run its agent CLI, a device that
  already has the programs its heavy commands ran first. Load spreads at starts, never mid-turn.
- **Manual:** an agent pinned to a device (`agents.device`) starts there; the others are placed as on Auto.
- An agent's own tools (MCP servers, build tools) are never checked per device: a session gets the shared MCP config as
  it is, and a server whose program is missing there does not start. `hl browser` and `hl sim` ship with Handline, and
  Handline installs its own skills into the shared skills folder every account sees, which runners receive.

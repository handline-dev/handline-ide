# Story 4-0: running agents across several machines

Research only. Nothing is built. Licences come from `gh api repos/<r>` on 2026-10-14. Behaviour
marked "general knowledge" was not checked in the project's docs and is unverified. It builds on the
resource-pool model of `docs/story2-load-research.md` (host cap `H`, account cap, held-reason queue).

## 1. How others spread work over machines

| Project | Licence | Repos and worktrees | Build cache | Simulators / emulators | Secrets and accounts | A machine drops out |
|---|---|---|---|---|---|---|
| GitHub Actions runner (`actions/runner`) | MIT | Each job checks out into a per-runner work folder; no shared worktree | Cache is a hosted service keyed by string; the runner has none of its own (general knowledge) | Whatever the host has; macOS runners must be real Macs | Secrets delivered per job by the server; none stored on the runner | Server marks it offline; a queued job waits for another runner with the same label |
| Buildkite agent (`buildkite/agent`) | MIT | Agent keeps a git mirror per repo and checks out a clean copy per job (general knowledge) | Local to the agent; sharing is the operator's job | Host's own; routing by agent tags | Agent token per machine; job secrets from a hook or the secrets service | Heartbeat lost: job is failed or retried by policy, tags route the retry |
| Cirrus CLI + Tart (`cirruslabs/cirrus-cli`, `cirruslabs/tart`) | CLI MIT; Tart is FSL-1.1-ALv2 (source-available, turns Apache-2.0 after two years) | A fresh macOS/Linux VM per task, cloned from an image; the repo is cloned inside | Image layers hold warm caches | macOS VMs on Apple silicon, so iOS Simulator runs inside | Nothing persists in a thrown-away VM; secrets injected per task | The task is lost with the VM and re-run elsewhere |
| Nomad (`hashicorp/nomad`) | BUSL-1.1 (API: NOASSERTION) | Task directory per allocation | None built in | Host drivers only | Vault integration (general knowledge) | Missed heartbeat: allocations are rescheduled on other nodes |
| Dev containers (`devcontainers/spec` CC-BY-4.0, `devcontainers/cli` MIT) | see left | One container per workspace, repo bind-mounted or cloned into a volume; worktrees are the host's business | Named volumes for package caches | Linux only; no iOS | Env and mounted credential helpers; forwarded SSH agent | Not addressed; the container is a single machine's |
| Coder (`coder/coder`) | AGPL-3.0 | One workspace per task from a Terraform template; repo cloned on start | Persistent volume per workspace | Whatever the template provisions, including Mac hosts if the operator has them | Per-workspace token; external auth (git provider) broker | Workspace shows offline; agent reconnects on return; no automatic move |
| Gitpod (`gitpod-io/gitpod`) | AGPL-3.0 | Prebuilt workspace per branch, repo cloned in | Prebuilds are the cache | Linux | Per-user secrets injected | Workspace restarts from the last backup |
| E2B (`e2b-dev/E2B`, `e2b-dev/infra`) | Apache-2.0 both | Sandbox per agent run; files go in over an API | Template snapshots | Linux microVM; none for iOS | Passed as env at create time | Sandbox is gone; the caller creates another |
| Firecracker (`firecracker-microvm/firecracker`) | Apache-2.0 | n/a (the isolation layer under sandboxes) | Snapshot and restore of a warm VM | Linux guests only | n/a | n/a |
| Daytona (`daytonaio/daytona`) | Not detected by the API; no LICENSE file at the repo root (unverified) | Sandbox per run, snapshots | Snapshots | Linux | Env at create | Unverified |

Reading across:

- **Nobody moves a running agent.** Every system above either fails the job and re-queues it, or
  restarts it from a snapshot. Work that is already mid-turn stays with its machine.
- **Placement is by label or declared need**, not by live load (Buildkite tags, Nomad reservations).
  This matches story 2: a fixed integer and declared reservation beat anything adaptive.
- **The repo is cloned per machine** (mirror plus clean checkout in Buildkite). No tool shares a
  worktree across machines.
- **iOS is the hard constraint.** The simulator only exists on a Mac; Linux sandboxes (E2B,
  Firecracker, dev containers) cannot run it. Only real Macs, or macOS VMs on Macs, can.
- **Secrets stay off the box until a job needs them**, and thrown-away machines hold none.

## 2. What we would borrow

Ideas only. Coder and Gitpod are AGPL and Nomad is BUSL: no code from those three. MIT/Apache code
(Buildkite agent, E2B infra, Firecracker) could be used with its notice kept; Tart's licence also
forbids offering it as a competing service, so read it before depending on it.

1. **Buildkite: labels and a bare mirror.** Each machine reports tags (`mac`, `simulator`, `gpu`);
   a task asks for tags; placement picks a machine with a free slot. A per-machine git mirror makes
   a new worktree a cheap local checkout.
2. **Buildkite/Nomad: the machine is one more pool**, next to the account cap and `H` from story 2.
   A start that finds every fitting machine full waits with the reason "host full" on that machine.
3. **GitHub runner: the server never trusts the runner for secrets.** An account's credentials stay
   on the machine that owns that account; a task asks for the account and runs where it lives.
4. **Nomad: a lost machine's work is re-queued, not moved.** An agent that was mid-turn on a machine
   that drops out is marked lost with the reason, and the task goes back to queued with its worktree
   branch pushed (or flagged unpushed).
5. **E2B: an agent in a throw-away box** is useful only for pure-code work with no simulator and no
   account; worth a later step, not a first one.

## 3. Open questions for the founder

- Which machines exist beyond this one (a second Mac, a Linux box, cloud)? Everything below assumes
  at least one more Mac.
- Does an account's login (Claude session files) have to stay on one machine, or may two machines
  hold the same account? Quota is shared, so the account cap would have to be counted across them.
- Is `docs/story2-load-research.md` (PR #96) merged first? The host cap this builds on lives there.

## 4. Proposal: what moves first, what stays

**Stays on the main machine:** the daemon and its database, the gateway, the phone and PC apps'
connection, the chat and task state, every account's login, and anything that needs the iOS
Simulator or the founder's own desktop session. One source of truth; the other machines never hold
the database.

**Moves first (4-1):** one extra machine runs *workers*: agent sessions that need only a repo and a
shell. Candidates in order:

1. Test and build runs (the largest consumer in story 2: 4 GB, half the cores). A remote test slot,
   same lock-and-queue shape as the local one.
2. Agents on reviewing and research tasks (read-mostly, small worktree, no simulator).
3. Implementation agents on a pure-code branch, pushing to a branch the main machine fetches.

**Stays until later (4-2):** simulator and emulator work, anything that needs an account that is not
on the worker, and moving agents between machines mid-turn (nobody in the survey does it).

**What 4-1 needs from the daemon:** a machine record (name, tags, cores, memory, free slots, last
heartbeat), a placement test beside the account cap, a "lost" state with a reason, and the queue
reason "no machine with `<tag>` free". The admin view of story 2 gains a machine column.

## 5. Leader and follower: building blocks

The founder's direction: the leader writes, its state is replicated to followers, a follower is
elected leader when the leader dies, and work goes by each node's load. Licences and archive status
from `gh api` (2026-10-14); "general knowledge" is not checked in a doc. Today's daemon keeps one
SQLite file through `modernc.org/sqlite` (pure Go, no cgo; `go/go.mod`).

| Block | Licence | What it is | Fit |
|---|---|---|---|
| `hashicorp/raft` | MPL-2.0 (file-level copyleft; fine to link unmodified) | Raft library for Go: leader election, a replicated log, snapshots, your own state machine | Closest to "leader writes, followers replicate": the log entries would be the daemon's writes |
| `etcd-io/raft` | Apache-2.0 | Raft core only: the caller supplies network, storage and timers | More work than hashicorp/raft; the same model |
| `lni/dragonboat` | Apache-2.0 | Multi-group Raft library; last push 2025-07 | Heavier than we need; slower upstream |
| `rqlite/rqlite` | MIT | A server: SQLite replicated with Raft, SQL over HTTP, leader writes, read consistency tunable (README, "Tunable Consistency") | Gives leader/follower for free, but the daemon would talk to rqlite instead of opening its own file: the whole db layer changes |
| `canonical/dqlite` | LGPLv3 plus a special exception allowing static or dynamic linking in a Combined Work (its LICENSE file; API: NOASSERTION) | SQLite replicated with Raft, a C library (README) | Needs cgo; the daemon is pure Go today |
| `nats-io/nats-server` (JetStream) | Apache-2.0 | Messaging with replicated streams, Raft inside (general knowledge) | Good for events, not for SQL state |
| `superfly/litefs`, `benbjohnson/litestream` | Apache-2.0 both | Replicate or back up a SQLite file; LiteFS has a leader lease (general knowledge); Litestream is backup | Whole-file shipping; no election of its own in Litestream |
| `hashicorp/memberlist`, `serf` | MPL-2.0 | Gossip membership and failure detection | Useful for "who is alive" only |

What the survey in section 1 adds: no coding-agent tool in it elects a leader; Nomad and Consul use hashicorp/raft
and are the shape the founder describes (a Raft server set, workers that join), but Nomad is BUSL
and Consul's licence is NOASSERTION, so we use ideas and not code (Nomad's BUSL is from its `LICENSE` file, read in section 1).

**Recommendation (a choice, not built):** do not write consensus. Pick between two:

1. **`hashicorp/raft` inside the daemon.** The log carries the daemon's write calls (the same
   `rpc` write methods apps already send), each node applies them to its own SQLite. Keeps pure Go
   and the single-binary daemon. Cost: every write path must be a deterministic replayable command;
   anything that touches the machine (spawning an agent, a clock) must stay outside the log.
2. **rqlite under the daemon.** Least consensus code, but the db layer moves from a file to HTTP
   and `modernc.org/sqlite` is replaced: a rewrite of every query site.

Option 1 is the smaller rewrite of behaviour and keeps what we have; option 2 is the smaller amount
of new concurrency code. Neither makes agents move: a running agent is a process on one machine, so a
leader's death means its agents are lost and their tasks re-queued (section 2, point 4).

**Load balancing** stays our own and small: each node reports cores, memory and slots (the story 2
numbers) through the log or a heartbeat; the leader places new work on the node with the most free
slots that has the task's tags. A rule, not a model.

**Open for the founder:** how many nodes (Raft wants an odd number, three at least to survive one
loss; with two machines a failed leader leaves no majority and no automatic election)? That one
question may decide the whole shape, so ask it before choosing between 1 and 2.

## 6. Agent tools that run on several machines (11-21)

Asked by the founder (T3890 8135): how agent tools spread agents over machines, what to take and what to
leave, without logic that grows too complex. Eight tools, read from their own docs on 2026-11-21; they are
not named here (the founder, T3890 8143).

| Kind | Joining | Where work goes | A machine drops out | Logins, MCP, skills |
|---|---|---|---|---|
| A cloud agent service with self-hosted workers | A worker command dials out, no inbound port | Personal: the person picks the machine, no fallback. Team pools: a run waits until a free worker with its label claims it | Pools: a reconnect window, then the run may move to a new machine | The machine keeps its own tools, logins and network and gets no secrets from the service; local MCP servers and hooks run on it |
| Two agent team platforms with a daemon per machine | A login and a join token; the daemon dials the server | An agent is bound to the machine the person picked (or the only one online); queued work never moves; no load balancing | Marked lost after about 5 min; queued work waits; one fails a running task only after 3 h; agents never move by themselves, a person moves one | Agent CLIs found by scanning PATH; logins stay on the machine; MCP config merged with the machine's own; a bare repo per project and a worktree per run; a cap of runs per machine; the version in every heartbeat |
| Two desktop agent apps with SSH hosts | An SSH host; the agent installed there | The person picks the host | One stops while the laptop sleeps; the other keeps running and rejoins | MCP, skills and logins are the host's own |
| Two agent frameworks with remote backends | An HTTP adapter or a terminal backend per agent | Set by hand, per agent | Not covered | No machine concept |
| A device cluster for models | Finds devices on the network, no master | Splits one model over devices by memory | Not covered | Models, not agents |

Reading across:

- Every machine dials out with its own credential and keeps its own logins, programs and MCP servers.
  Nobody copies a login to another machine. Story 4 already does both.
- Placement is by hand or sticky: an agent is bound to a machine. Only the cloud service's team pools pick a
  machine per run, by label, for throw-away workers. Nobody balances by load.
- A machine that drops out is waited for (5 minutes to 3 hours), not emptied at once; a move is a person's
  act or comes after a long grace.
- None moves its own server: there is one hub, in the cloud or self-hosted. None has a pause or drain for a
  machine; cluster tools do (T3890 8122).
- What a machine has is found, not configured: a PATH scan at start, the version in every heartbeat.

Taken into Story 4 (the founder's OK, T3890 8140; `docs/story4-design.md`):

1. **An agent stays on one device while it has room.** Its sessions, transcript, worktree and build
   caches stay there. A device is picked, the one with the most room, only when the agent has none, or
   at a session's start its device is full (memory cap, slots, simulators), Off, offline past the wait,
   or lacks what the agent needs; the agent then stays on the new one. Load spreads at starts, never
   mid-turn. This replaces placing each session (design Decisions 5).
2. **A device that drops out is waited for** 30 minutes (a setting, was 10), so a closed lid does not
   scatter a laptop's agents; Off moves them at once.
3. **The master stays where the owner puts it**, moved by hand ("Make this Mac the master"), also when
   it is gone. Taking over by itself becomes a later slice; with two machines it is the one place two
   masters can appear.
4. **A device says what it has** (agent CLIs, MCP programs, Xcode and simulators, Handline version) at
   connect and in each heartbeat; an agent goes only where all it needs is, and a device on another
   version takes no work.

Left: placing each run on any machine (it suits throw-away workers, not agents with warm caches),
copying logins or MCP programs to a machine, and a master run as a cloud service.

## 7. Moving agents or moving heavy jobs (11-24)

Asked by the founder (T9440): maybe only heavy work should leave this Mac, and agent work stay on one machine.
Tools read again on 2026-11-24 from their own docs and code; not named here.

| Kind | What leaves the machine | What the other machine needs | When it cannot |
|---|---|---|---|
| Editor agents with remote hosts or self-hosted workers; agent team platforms with a daemon per machine | The whole agent session | The agent CLI, its logins, MCP servers, skills and the repo | The run waits or fails; a person moves it |
| A remote build tool for coding agents, driven by a `PreToolUse` hook | Build and test commands only; the agent stays | The build tools and a synced copy of the workspace | Runs locally (fail-open) |
| A remote build service for agents | Builds and tests, with the agent's uncommitted diff on top of a commit | Warm snapshots of the repo and its caches | Runs locally |
| A monorepo build tool's distributed runs | Tasks of one run, spread over machines by CPU and memory | The repo at the commit and the tools | Runs on fewer machines |

Reading across:

- Tools that move heavy jobs say why the same way: many agents at once saturate one machine's CPU and memory,
  and it is the builds and tests that do it, not the agents.
- A heavy job needs the code and the build tools; an agent needs those plus its logins, MCP servers, skills and
  version. Each of these is one more thing that can be missing (T3890: four were on 11-21, the version again on
  11-22).
- Every heavy-job tool falls back to running locally; none waits on a remote machine.
- Measured here (7 days of this team's transcripts, T9440): the full check is 64% of heavy command time; one full
  check uses about 4 GB, all 13 agents together about 4.5 GB, one agent about 350 MB
  (`docs/story2-load-research.md`). Simulators were not measured here; forum reports put one at several GB.

The same 7 days split by agent: one agent (the one that lands every branch, running the full check each time)
did 48% of the heavy command time, the next 16%, the top four 84%. Heavy work sits in a few agents.

An agent IDE that runs workers on several machines ships its own tools inside its app: a small helper app that
holds the screen-control permission, a browser and a browser-driving program, simulator and emulator control,
and its command line, taught to agents by skills it bundles (read from the installed app, 11-24). A second
machine running that app has every tool with nothing to install; agent logins and build tools are still set up
by hand once.

Taken into Story 4 (`docs/story4-design.md` section 7, the founder's answers T3890 8654 to 8667): heavy agents
go whole to the machine with the most room, the rest stay; Handline ships its own browser and simulator tools
with skills it installs by itself; a lent Mac needs only a Claude login, and agents install their own build tools.

## 8. Reaching a local agent tool from another device (11-25)

How tools that run on the person's own machine let a phone, a browser or another computer in, from each
tool's own documentation (the pages are named in T9477 comment 8815):

- **A terminal coding agent's remote control:** the machine opens no port. The local session connects out to
  the vendor's service and the phone or browser comes in through the vendor's https site; the signed-in
  account is the key.
- **Self-hosted agent orchestrators:** reachable only on the machine by default; for other devices the
  person turns on a login and binds a LAN, VPN or Tailscale address.
- **Browser IDE servers:** an SSH tunnel is the advice; a self-made certificate is the last resort, as the
  browser warns, and a tablet's Safari refuses its WebSocket unless the certificate is installed as a profile.
- **File sync tools:** devices trust each other by the hash of each one's certificate, as our Mac apps pin
  the fingerprint; the browser screen is local by default and wants a password and https when opened up.
- **Tailscale's own serving:** the `*.ts.net` name gets a real certificate by itself, reachable only inside
  the tailnet, once the tailnet has HTTPS on (which publishes the machine's and the tailnet's names in the
  public certificate log).

So a browser from another device is common, but the pattern is local by default, then a private network or
the vendor's relay, on a real certificate. A browser page over plain `http://` on a name that is not
localhost is not a secure context: copying to the clipboard, notifications and service workers are refused.
Decision 16 in `story4-design.md` follows it.

## Sources

`gh api repos/<r>` for every repository in the table (licence and description), and
the `LICENSE` files of `cirruslabs/tart` and `hashicorp/nomad`. Buildkite agent behaviour is from the project's README only and mostly
general knowledge. No page for Daytona, Coder's docs or Nomad's task-driver docs was fetched.

# Setup

The README gets you to a first task. This page covers each step in more detail and what to do when
one fails.

## Requirements

| Needs | Why | Check |
|---|---|---|
| macOS | The Handline app runs the daemon. | |
| Go | `hl` is a Go program; `tools/build-hl.sh` builds it. | `go version` |
| git | Each agent works in a git worktree. | `git --version` |
| Claude Code | The daemon starts it. | `claude --version` |
| Orca | Provides the terminals the agents run in. | `orca --help` |

## Install the command

`bin/hl` is a shell wrapper around the built `go/hl`. It finds its own repository through the
symlink you make, so link it instead of copying it:

```bash
ln -s "$PWD/bin/hl" /usr/local/bin/hl
```

If `/usr/local/bin` is not writable, link it into any directory on your PATH, or add the repo's
`bin/` to PATH.

Then build it:

```bash
tools/build-hl.sh
```

## Choose where state lives

Everything `hl` stores is under `~/.handline`. To use another place, set `HANDLINE_HOME` before every
command. This is how you try it without touching a real install:

```bash
export HANDLINE_HOME=$(mktemp -d)
hl init
```

`hl init` writes the config, installs the hooks that report agent events, and makes the
directory a git repository, so you can see every change to your setup.

Note: the protected store described under [Configuration](configuration.md) lives under
`~/.config/handline`, keyed by the `HANDLINE_HOME` path. A throwaway `HANDLINE_HOME` therefore still writes
there.

## Accounts

Add each account in the app's Accounts page. Adding one registers the name and then starts Claude's
own login, which you complete yourself. Use a different Anthropic login for each account, because
that is how you get more usage windows. An account registered without a login is logged in from the
same page later.

`hl` never reads, writes, or copies credentials. They stay in Claude's own config directory for
the account, `~/.handline/accounts/<name>/.claude`. A refresh token works once and then rotates,
so two copies of one login would invalidate each other. That is why `hl` does not copy them.

Removing an account in the same page logs it out and moves its directory aside. It refuses while an
agent is running on it.

## Agents

Create each agent in the app. It needs a name, a repo (the daemon starts the agent in it when work
arrives), a model and an effort. Smaller models cost less usage, and `low` to `max` effort is a real
cost lever. See [Configuration](configuration.md) for the files an agent keeps.

## The team file

`~/.handline/team.yaml` names the agents and who reports to whom. `hl team` lists them. After you
edit `team.yaml` or any `agent.yaml` by hand, seal your change on the Team page of the app (see
[Configuration](configuration.md#sealed-files)).

## Run the daemon

Open the Handline app. It starts the daemon as its own process, and the daemon stops when the app quits; the agents keep running
in their terminals and the next start picks them up. Only one daemon runs. A second one exits. `hl team` shows what each agent is doing now.

## When something fails

| You see | Cause and fix |
|---|---|
| An agent never gets its task | The app's menu-bar icon shows whether the daemon is running, and `hl team` what each agent is doing. |
| Task refused with "unknown target" | The name is not on `hl team`. Use the full name shown there. |
| An agent is stuck on a dialog | The bridge lists it under WAITING ON YOU, with the dialog named. `hl` does not answer dialogs. Answer it in the terminal (`t` in the bridge). |

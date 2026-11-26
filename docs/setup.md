# Setup

The README gets you to a first task. This page covers each step in more detail and what to do when
one fails.

## Requirements

| Needs | Why | Check |
|---|---|---|
| macOS | The Handline app runs the daemon. | |
| Go | `hl` and the daemon are Go programs (`tools/build-hl.sh`, `tools/build-go.sh`). | `go version` |
| Node and pnpm | The app is built with them. | `pnpm --version` |
| git | Each agent works in a git worktree. | `git --version` |
| tmux | Each agent's terminal is a tmux pane. | `tmux -V` |
| Claude Code | The daemon starts it. | `claude --version` |

## Install the command

`bin/hl` is a shell wrapper around the built `go/hl`. It finds its own repository through the
symlink you make, so link it instead of copying it:

```bash
ln -s "$PWD/bin/hl" /usr/local/bin/hl
```

If `/usr/local/bin` is not writable, link it into any directory on your PATH, or add the repo's
`bin/` to PATH. Then build it with `tools/build-hl.sh`.

## Choose where state lives

Everything `hl` stores is under `~/.handline`. To use another place, set `HANDLINE_HOME` before every
command. This is how you try it without touching a real install:

```bash
export HANDLINE_HOME=$(mktemp -d)
hl init
```

`hl init` sets the home up: its folders, the shared settings and the hooks that report agent events,
and Handline's own skills. The daemon does the same at start.

The protected store, which holds what only the founder may decide and no agent can write, lives under
`~/.config/handline/protected`, keyed by the `HANDLINE_HOME` path. A throwaway `HANDLINE_HOME`
therefore still writes there.

## Accounts

Add each account in the app's Accounts page. Adding one registers the name and then starts Claude's
own login, which you complete yourself. Use a different login for each account, because that is how
you get more usage windows. An account registered without a login is logged in from the same page later.

`hl` never copies credentials. They stay in Claude's own config directory for the account, under
`~/.handline/accounts/`. A refresh token works once and then rotates, so two copies of one login
would invalidate each other.

Removing an account in the same page logs it out and deletes its directory; the conversations stay in
the database. It refuses while an agent is on the account.

## Agents and workspaces

Create each agent in the app. It needs a name, a repo (the daemon starts the agent in it when work
arrives), a model and an effort. Smaller models cost less usage, and effort is a real cost lever. The
workspaces and their agents are rows in `~/.handline/db.sqlite`, changed in the app; see
[Configuration](configuration.md#workspaces-and-agents). `hl team` lists them.

## Run the daemon

Open the Handline app. It starts the daemon as its own process; quitting the app leaves the daemon
and its agents running, and the next app start connects to it. Only one daemon runs on a home: a
second one exits. `hl team` shows what each agent is doing now.

## When something fails

| You see | Cause and fix |
|---|---|
| An agent never gets its task | The app shows whether the daemon is running, and `hl team` what each agent is doing. |
| Task refused with "unknown target" | The name is not on `hl team`. Use the full name shown there. |
| An agent is stuck on a dialog | `hl` does not answer dialogs. Answer it in the agent's terminal. |

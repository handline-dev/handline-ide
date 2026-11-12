# Agent-proposed commands the founder runs with one press (`hl propose run`)

Founder (T9022): "배시 명령을 우리 시스템에서 돌리도록 구현하고자 안 했나?" An agent made a delete command during disk cleanup, but the founder had to type `! rm -rf ...` in a terminal, because the permission classifier blocks the agent's own deletes. The way to run it for them is missing.

## Founder's answer (T9022 comment 6963)

"클로드코드처럼 명령어를 실행하면 터미널 하나 탭을 열어서 거기서 실행시키기. 처리중, 처리결과를 채팅 안에서 볼 수 있도록 하기." So the command runs in a terminal tab, as Claude Code does, and its progress and result show inside the chat. This is what T7389's command card already does for a command the founder types, so the run reuses it.

## What exists, and what is reused

T7389 (`go/internal/cmdruns`, `termchan/shell.go`): `shell.open` with a command and a block opens a window of the terminal host (`CreateRun`), records a run (`cmdruns.Start`, one `meta` row), `Settle` ends it from the window (exit status, end time), the chat shows it running and then done or failed, and `cmdruns.Tell` tells the agent with the window's last rows. What is missing is only who chooses the command and the press.

Reused as they are: the run, its window, the live chat state, and `Tell`. Also the proposal path (`proposal.propose` / `proposal.decide`, `go/internal/proposalwrites`): one card per proposal, a `subject` hash the founder must send back to approve (a changed command cannot be approved unseen), the decision recorded where no agent can write it, owner-only decide.

## Decision: a proposal kind `run` whose approve starts a `cmdruns` run

Approve = the daemon calls the same code as `shell.open` (extracted to a function both call) with the proposal's command and cwd, and the block `p<proposal id>:0`. `cmdruns.ParseBlock` gets the owner letter `p` (like `r` for a reply item); `Told` says "your proposed command" for it. The agent's sandbox and classifier wrap only the agent's own tool calls, so the window, opened by the daemon at the founder's press, is outside them.

## The agent's side

`hl propose run "<command>" --cwd <path> --why "<reason>"` = `proposal.propose` with `kind: "run"`, `args: {name: <the agent>, command, cwd}`, `why` required.

- `command` at most 4000 chars (the same `maxCommand` as shell.open), no NUL; one proposal is one command string (run by the window's shell), so a pipeline or `&&` is one card.
- `cwd` must exist as a directory when proposed; relative is resolved against the agent's worktree and stored absolute, so the card shows what will really run.
- At most 5 open (not yet decided) run proposals per agent; the sixth is `refused`.
- The agent can only propose and withdraw (`proposal.withdraw`). `proposal.decide` and the new `proposal.stop` are owner-only, so an agent cannot run its own command: its role fails the gate before anything is read.

## The card

Command in full (scrollable, never cut), cwd, why, who proposed, and the **risk marks**:

| Mark | Shown when the text has |
|---|---|
| delete | `rm`, `rmdir`, `find ... -delete`, `shred`, `git clean`, `git branch -D` |
| overwrite | `>` or `>>` redirect, `mv`, `cp` to an existing path, `dd`, `truncate`, `sed -i`, `git reset --hard`, `git checkout -- `, `git push --force` |
| network | `curl`, `wget`, `ssh`, `scp`, `rsync` to a host, `git push/pull/fetch`, `npm/pip install/publish` |
| outside | an absolute or `~` or `..` path that is not under the cwd; one outside the founder's home is marked stronger |
| privilege | `sudo`, `su`, `chmod -R`, `chown -R` (no terminal, so sudo fails at the password; the mark says so) |

The marks are a pure function of the text (`cmdruns.Risks`), with table tests. They are hints for reading, never a gate: they never approve, never block, and a command with no mark is not "safe" (the card says "marks are hints - read the command"). Buttons: **Run** (sends the card's `subject`) and **Reject** (with a note, as any proposal).

## Running, in a terminal tab, shown in chat

- The window is a real terminal (a pane of the host) in `cwd` that the founder can open as a tab and watch; a program that asks for input (a password for `sudo`) gets it from the founder there. The run is in the chat of the proposing agent as a card: **Running** with a live tail and "open the tab", then **Done** (exit 0) or **Failed** (exit N) with the last rows, the command, the seconds.
- The window's host limits stay: `MaxShells` 8 per agent (a ninth approve is refused, the card stays `proposed`), the window kept for a finished command.
- Stop = closing the window (`shell.close`; the founder's Stop on the card). State `stopped`, the agent told as T7389 tells it.
- **States**: the proposal's own states stay (`proposed` -> `approved` -> `applied`, or `rejected` / `withdrawn`). For kind `run`, `apply` starts the window and `applied` means started; the run's progress is the `cmdruns` state (`running`, `done`, `failed`, `stopped`), shown in the row's `run` and the chat. A start that fails (no host, nine windows open) is the existing `approved` + error (`apply_failed`), and approving it again retries.
- **Once**: for kind `run`, decide's state check (decide.go:316-320) refuses an approve once a `cmdruns` row exists for the proposal, so a started command never runs twice, even if it failed. A failed run is final: the agent proposes a new card. The `shell.open` start refuses a rerun of a `p` block.
- **The command cannot change after approval**: `apply` already refuses a draft whose hash differs from the recorded subject (`unapproved`, decide.go:392) before it starts the window. `subject()` for kind `run` covers command and cwd. An edit of the file after the press makes the run not start, and the card says "changed after approval".
- A daemon restart: the window lives in the host, so `Settle` ends the run from it as it does for T7389 (done, failed, or stopped if the window is gone). A run approved but not started is `approved` without a `cmdruns` row, so approving it again starts it. Nothing runs again by itself.
- No timeout: the founder sees it running and stops it. The window is the log; its tail is what the card and the agent get.

## What comes back

To the card: state, exit, seconds, the window's last rows, through the chat state T7389 already has (extended to `p` blocks). To the agent: the `Told` say, one line (`Your proposed command ended with exit 1 after 12s: <command, 200 chars>`) and the last 40 rows, cut at 6000 chars. The output goes to the agent, so the card says so before the press. The decision and the exit stay in the proposal file.

## Contract (additive; clients ignore unknown fields)

- `ProposalArgs`: add `command`, `cwd`. `proposal.propose` `kind` enum: add `run`.
- `proposal.list` row: add `run` = `{command, cwd, risks[], terminal, started, ended, exit}`; the chat's command-run state (T7389) accepts the block `p<proposal>:0`. An old client shows an unknown state as text.
- New `proposal.stop` (owner, write; closes the window).
- Errors: `bad_request` for a command too long or a missing cwd, `refused` for the sixth open card.
- Nothing is off or on: no agent can start a run, so the feature does nothing until an agent proposes and the founder presses.

## Builder tasks (draft)

1. Contract: the spec, `gen`, contracttest for the kind, the two methods, the states, the gate. Owner: contract builder.
2. `cmdruns.Risks` (pure) and its table tests, including quoting (`'rm'`, `$(rm ...)`, `rm\ x`) marked or documented as not caught. 
3. Extract `shell.open`'s run start into a function; `ParseBlock` and `Told` learn the `p` block; the chat's command-run state shows it. Tests: exit 0, exit 3, a stop, a restart with a running row, the ninth window refused.
4. `proposalwrites` integration: the kind's validation, subject over command + cwd, decide to start, once-only, recheck at start, tell the agent on every end (`cmdruns.Told`-shaped).
5. `hl propose run` and `hl propose list` showing the run state, with `--help`; agent-role tests: an agent cannot decide, stop or approve its own.
6. The card (architect-web draws it: the risk marks, Run/Reject, running with a Stop, the ended card with the tail and "output goes to the agent"). Needs 1.
7. End to end: propose `rm -r <tmp dir>`, approve, the dir is gone, the agent has a say with exit 0; the same with the file edited after approval (does not run); a reject (does not run).

## Not in this design

A standing allow ("always run this command for this agent") would make the founder's press a one-time default for later cards. It is a trust decision, not a build detail: left out, to ask the founder only if the cards become a chore.

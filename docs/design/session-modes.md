# Agent session modes: chat by default, terminal on demand

Status: proposal (T9412). Builds on [structured-sessions.md](structured-sessions.md), the October design that was built,
paused on 10-10 while the move to the Go daemon went first, and removed on 10-16 (d960d6562 in the earlier repository).

## Why now

- Cost is the same as the terminal. Anthropic's help page "Use the Claude Agent SDK with your Claude plan" says the Agent
  SDK and `claude -p` draw from the plan's subscription limits; the separate programmatic credit announced for June 15
  is paused.
- The terminal path guesses from the screen whether a turn runs and whether a message landed. Chat mode gets each of
  these as a frame.

## Two modes, one session

|                    | Chat (default)                                                | Terminal                                        |
|--------------------|---------------------------------------------------------------|-------------------------------------------------|
| process            | `claude -p --input-format stream-json --output-format stream-json`, owned by the daemon, no pane | `claude --resume <id>` in the agent's tmux pane |
| a message in       | a `user` frame with a uuid; its echo is the delivery         | typed keys (today's path)                       |
| turn state         | `session_state_changed`                                       | screen and hooks (today's path)                 |
| a permission       | `can_use_tool`, answered from the rules or as a Requests row | Claude Code's prompt in the pane                |
| a background job   | task frames; Stop is `stop_task`                              | T9413: found by the process group writing its output |
| who types          | nobody; people write in the chat                              | a person, in the pane                           |

Both modes keep the same session file in the account's config dir, which is what `--resume` reads. A session runs in one
mode at a time.

## Switching

- To terminal: Open in terminal on the agent. The daemon lets a running turn end (Stop first to cut it), closes the chat
  process and proves its process tree gone, then opens `claude --resume <id>` in the pane. A message that arrives during
  the switch is held and delivered in the new mode.
- Back to chat: closing the terminal tab. At the next idle the pane closes and the next message resumes the session with
  `-p --resume <id>`. A terminal left open stays a terminal; there is no timer.
- One setting, Settings > Agents run in: Chat | Terminal. Terminal until the founder has tried chat, then Chat. If
  Anthropic ever bills `-p` apart again, this one switch puts every agent back in the terminal.

## Kept from October

The October code, in the commit before d960d6562, is the starting point, not a new design:

- launch, the `initialize` handshake and the CLI version pin with its live check;
- delivery: operation id, `now` or `queue`, the echo as confirmation (`go/internal/delivery` still holds the types);
- permissions: the shared allow rules and the shell guard answer `can_use_tool`, so a person is asked only for what no
  rule covers;
- the sandbox settings file, and a close that proves the process tree gone.

## What must hold in chat mode that did not exist in October

- the per-agent sandbox and its allowances (T9410, T9411);
- run proposals (T9407) and the handoff note, both of which are only messages to the agent;
- account choice and moves, usage readings and limits: the account's `CLAUDE_CONFIG_DIR`, as in the terminal;
- a lent Mac runs chat sessions the same way;
- `hl` inside a chat session knows its agent (in October it was taken for the founder).

## Order

1. Daemon: a chat runner for one agent behind the setting, from the October code; one live check on a real account.
2. Permissions: `can_use_tool` from the rules, or a Requests row; the agent's sandbox settings.
3. App: the chat view of a chat agent from its frames; its terminal tab offers Open in terminal.
4. Switching both ways.
5. Background jobs: listed from task frames, each with Stop.
6. The founder tries chat on two or three agents; then the setting's default becomes Chat.

## Risks

- Control frames marked alpha change between CLI versions: the pin and the live check guard each upgrade.
- What only the full-screen terminal offers stays one button away, through Open in terminal.

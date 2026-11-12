# Codex agents beside Claude Code (T9027)

Founder (T8943): onboarding must offer Codex as well as Claude Code.

## What exists

The agent side is vendor-shaped: `go/agentctl/agent` has the `Agent` interface (binary, launch line, env, screen driver, hooks, transcript dir, delivery) and the `Account` facts (`ConfigDirName`, `ConfigDirEnv`, `LoginArgv`, `WebLoginArgv`, `CredentialPresent`), registered by name; `agent/vendors` links `claudecode` and `opencode`. `account.add` takes `vendor`, and until a codex package is linked it is refused as an unknown vendor (`CheckVendor`). So the work is a **codex vendor package** and four small contract additions, not a new mechanism.

## Decisions

1. **Agent kind = `vendor`**, sent on `account.add` and in the `vendor` field of a roster proposal (`proposal.propose add`, team.json `ProposalArgs`). No new field.
2. **Install check: `daemon.status.vendors`** (additive) `[{vendor, binary, installed, fix}]`, read from the registered `Agent`s (so V1 registers an `Agent`, not only an `Account`). `missing_tools` keeps its meaning, a tool needed now: `reads.Env` gets a func "which vendors are in use" (accounts and agents), and codex is listed there only when one is, so a Claude-only founder never sees it missing.
3. **Login: `account.login` is unchanged for the caller.** Codex's `WebLoginArgv` is `codex login --device-auth` (a URL and a one-time code, no code to bring back). weblogin already ends a login on the child's exit plus `CredentialPresent`; codex's `CredentialPresent` runs `codex login status` under the account's `CODEX_HOME` (as opencode's runs `auth list`). The one real change: the login row gains `device_code` beside `url`, kept only in memory and never logged, which relaxes weblogin's "hand on the URL only" rule for this vendor. `account.login_code` on a codex login is `refused` (gate `state`).
4. **Existing logins on this Mac.** `accounts.discover` (owner, read) `[{vendor, source, logged_in}]` over `~/.claude` and `~/.codex`, from `CredentialPresent` only (`logged_in` null when it cannot be told; never a credential's content). `account.adopt {vendor: "codex", source, name}` makes the account's own config dir with only `auth.json` linked to the found one, so hooks, instructions and sessions stay out of the person's `~/.codex`. A logout in either place logs both out; the screen says so. No adopt for claude: its keychain entry is keyed by config dir, so a found Claude login is shown as found and the account logs in again.
5. **Usage: codex is unmetered by construction** (`usage.Metered` is true only for claude or a vendor with no driver), so the room test is skipped. But an **unpinned codex agent can never be placed** (`NotFor` drops unmetered accounts from the pick), so the add that onboarding makes **pins** the agent to its codex account.
6. **Starting an agent with codex** is the `Agent` implementation: `LaunchArgv`/`LaunchEnv` (`CODEX_HOME`, model, effort, sandbox), a screen driver, hooks through `hooks.json`, a transcript lookup, delivery by typing settled as unknown (the rule for a CLI with no inbox). The transcript lookup scans only today's and yesterday's session folders for the session's cwd, then falls back to a full scan once.
7. **Rules and leases (settled by S1).** Rules go in on the launch line as `-c developer_instructions=...`, so nothing is written into the repo; the repo's own `AGENTS.md` is still read beside them. A `PreToolUse` hook can refuse both shell commands and `apply_patch` edits, so the lease guard works for codex. The guard has to be checked at start: a hook command that fails lets the tool run (see S1 findings).

## S1 findings (codex-cli 0.157.1, 2026-11-12)

**Launch line** for cwd `D` (the TUI, one process per agent):

```
CODEX_HOME=<account dir> codex --no-daemon -C D -m <model> -c model_reasoning_effort=<low|medium|high> \
  -s workspace-write -a on-request -c check_for_update_on_startup=false \
  -c 'projects={"D"={trust_level="trusted"}}' -c 'developer_instructions="""<base.md + ROLE.md + team rules>"""'
```

- `--no-daemon`: without it the TUI attaches to a shared background app-server, which would mix agents' environments.
- The inline `projects` table skips the trust screen without saving anything. The dotted form `projects."D".trust_level` did not skip it.
- The model must be one the ChatGPT account accepts: `gpt-5.6-sol` was refused (`model_not_found` for ChatGPT accounts). `$CODEX_HOME/models_cache.json` listed `gpt-6-luna`, `gpt-5.6-terra`, `gpt-5.6-luna`; it is a cache, so the driver also reports a `model_not_found` turn as the agent's error.

**Device auth** (`codex login --device-auth`), on stdout and coloured with ANSI codes, then waiting:

```
Follow these steps to sign in with ChatGPT using device code authorization:
1. Open this link in your browser and sign in to your account
   https://auth.openai.com/codex/device
2. Enter this one-time code (expires in 15 minutes)
   AB12-CD3EF
```

**Screens** the driver has to recognise, in the order they appear:

- *Login* (no `auth.json`): `Welcome to Codex ...` with options `1. Sign in with ChatGPT`, `2. Sign in with Device Code`, `3. Provide your own API key`. The launch never answers it; the account is logged in first.
- *Update*: `Update available · 0.157.1 → 0.167.2` with options `Update now`, `Skip`, `Skip until next version`.
- *Trust*: `Trust this folder? ... Your trust decision will be saved.` with `› 1. Trust and continue` and `2. Quit`.
- *Hook review*: `Hooks need review` / `N hooks are new or changed.` with `1. Review hooks`, `2. Trust all and continue`, `3. Continue without trusting (hooks won't run)`. Trust is saved in the account's `config.toml` as `[hooks.state."<hooks.json path>:<event>:<group>:<index>"] trusted_hash = "sha256:..."`, a hash of the hook entry, so the screen comes back only when Handline changes its hooks. An untrusted hook is skipped without saying so: in `codex exec` it never ran. Handline writes the account's `hooks.json` itself, so V5 launches with `--dangerously-bypass-hook-trust` rather than answering this screen or computing codex's hash.
- *Idle*: a header box `>_ OpenAI Codex (v0.157.1)` / `model: ... directory: ...`, then the composer `› Ask Codex to do anything` and the footer `? for shortcuts`.
- *Busy*: `• Working (1s • esc to interrupt)` above the composer.
- Startup warnings (`⚠ N warnings · f2 to view`) do not block.

**Typing a message:** text sent with its Enter in the same write was not submitted (it was taken as a paste). The text, a pause (0.7 s worked; a guess, not a documented value), then Enter on its own submits it; the driver confirms the composer emptied and the busy line appeared.

**Hooks** live in `$CODEX_HOME/hooks.json` (the same shape as Claude's: `{"hooks":{"<Event>":[{"matcher"?,"hooks":[{"type":"command","command":...,"timeout":s}]}]}}`). Events: `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `SubagentStart`, `SubagentStop`, `Stop`, `Interrupt`. The JSON on stdin has `session_id`, `turn_id`, `transcript_path`, `cwd`, `hook_event_name`, `model`, `permission_mode`, plus `tool_name`/`tool_input`/`tool_use_id` (`Bash` gives `{"command"}`, `apply_patch` gives `{"command":"*** Begin Patch\n*** Add File: z.txt ..."}`, so the lease guard reads paths from the `*** Add|Update|Delete File:` lines), and on `Stop` also `last_assistant_message`.

**Refusing a tool call works.** On `PreToolUse`, the hook prints `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"<why>"}}` and exits 0. The TUI shows `• Blocked by hook └ <why>`, the command does not run (checked for `touch` and an `apply_patch` add) and the model reads the reason. `deny` without a reason is refused, as are `allow` and `ask`. A hook whose command is missing shows `Hook failed └ hook exited with code 127` and the tool runs anyway, so a broken guard leaves the agent unguarded.

**Session file:** `$CODEX_HOME/sessions/YYYY/MM/DD/rollout-<local time>-<session id>.jsonl`. Its first line is `{"type":"session_meta","payload":{"session_id","cwd",...}}`, Every hook passes the path as `transcript_path`, so the folder scan in decision 6 is only the fallback before the first hook.

## Contract (additive; clients ignore unknown fields)

See decisions 2-4: `daemon.status.vendors`, `accounts.discover`, `account.adopt`, login `device_code`. A founder who never picks codex changes nothing.

## Build tasks, in order

- **S1 spike** (done): see S1 findings.
- **C1 contract** (parallel with S1): decisions 2-4 in the spec, `gen`, contracttest.
- **V1** `agent/codex` Account and Agent shell linked in `vendors`; `daemon.status.vendors`; the in-use func for `missing_tools`. After C1.
- **V2** login in `accounts/weblogin` and `CredentialPresent`. After S1, C1.
- **V3** `accounts.discover` and `account.adopt` in `data/team` and `adminwrites`, tested with a fake home. After C1.
- **V4** the launch line, screen driver (captured screens as test data), the pin at add. After S1.
- **V5** hooks, turn state, transcript lookup, delivery. After V4.
- **V6** live test (`HANDLINE_LIVE=1`): log in, start, message, read the reply, stop. After V2, V4, V5.

The onboarding screen (a11) builds against C1's contract with fixtures and waits for none of V1-V6.

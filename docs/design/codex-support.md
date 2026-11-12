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
7. **Two unknowns S1 settles before V4:** where `base.md`/`ROLE.md`/team rules are injected (Claude gets them in the launch system prompt; Codex reads `AGENTS.md`, and nothing may be written into the repo); and whether a codex hook can refuse a tool call. If it cannot, the lease guard cannot work, and the choice is to refuse starting a codex agent in a repo that uses leases, not to ship a limit that is not enforced.

## Contract (additive; clients ignore unknown fields)

See decisions 2-4: `daemon.status.vendors`, `accounts.discover`, `account.adopt`, login `device_code`. A founder who never picks codex changes nothing.

## Build tasks, in order

- **S1 spike** (first): run the real codex on this Mac and add what it found to this file: the interactive launch line for a cwd, model, effort and sandbox; the device-auth output; the trust, update, idle and busy screens; hook events and whether one can refuse a tool call; the injection point; the session file of a cwd. No daemon code.
- **C1 contract** (parallel with S1): decisions 2-4 in the spec, `gen`, contracttest.
- **V1** `agent/codex` Account and Agent shell linked in `vendors`; `daemon.status.vendors`; the in-use func for `missing_tools`. After C1.
- **V2** login in `accounts/weblogin` and `CredentialPresent`. After S1, C1.
- **V3** `accounts.discover` and `account.adopt` in `data/team` and `adminwrites`, tested with a fake home. After C1.
- **V4** the launch line, screen driver (captured screens as test data), the pin at add. After S1.
- **V5** hooks, turn state, transcript lookup, delivery. After V4.
- **V6** live test (`HANDLINE_LIVE=1`): log in, start, message, read the reply, stop. After V2, V4, V5.

The onboarding screen (a11) builds against C1's contract with fixtures and waits for none of V1-V6.

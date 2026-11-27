# Agents on an handline team

True for any agent, whatever tool runs it. Your name is in `$HANDLINE_AGENT`.

<!-- protocol: terminal ends legacy progress wait review founder command -->
- Talk to people only through hl; what you write in your terminal reaches no one.
- A task you were handed ends with `hl task done T<n> "<evidence>"` or `hl task handoff T<n> <agent> "<note>"` - that is your report, even a deferral: the next task arrives only then, and an answer typed in chat reaches no one.
- When your change leaves something that only old data or old habits still need (a compatibility path, an old name, a file nothing reads any more), open a task to remove it before you finish, saying when it can go, and name that task in your done.
- Work in steps is reported at each step with `hl progress --done N --of M "<what you are doing now>"`: the task shows the count.
- Never end a turn holding a task you are not working on: it blocks the tasks queued behind it. Finish it, or block it on what it waits for (`hl task edit T<n> --blocked-by T<m>`); it comes back to your queue when that ends.
- A done closes the task; nothing waits on an accept. Review is a subagent you run before the push, not a handoff. Ask the founder for a look by finishing the task, never by an "OK?" comment: the daemon puts a finish before them when it is theirs to see. A goal too big for one task is split into a story's drafts, and that split is what the founder reads before Run.
- When you cannot go on without the founder's decision, put them on the task you hold: `hl task comment T<n> add "<first line: the decision and what you recommend; then why>" --turn human`, with `--attach <screenshot>` when it is about something visible, then take your next task or stop; their comment gives it back to you. News for the founder that needs no decision is a comment on the task that starts with `@human`; it moves no turn. An `@human` or `--turn human` comment already shows in the founder's chat with you; do not repeat it with `hl say`. A question to the founder carries its content: put a file on with `--attach`, never a bare path, which they cannot open. Work you open from the founder's comment that another agent passed to you serves his request: `hl task new ... --request T<the task he commented on>`.
- A command you cannot run yourself (a refused permission, the founder's login or keychain) goes to the founder as `hl propose run "<command>" --cwd <absolute dir> --why "<one line>"`: they run it with one button and you are told how it ended. Never ask another agent to run it, and never ask the founder to type it.
<!-- /protocol -->
- There is one kind of message, `hl say <target> "..."`: chat, typed at once, needs
  no answer, starts no work, and carries no task: what is written about a task is a comment on it. Anything that needs an answer or a result is a
  task: `hl task new --assign <target> -t <time> "<title>" "<body>"`. Work
  reaches another agent only as a task; a subagent is for research or review
  inside your own task.
- You hold one task at a time. Its done or handoff is how the result reaches
  whoever asked, never a second task repeating it.
- A memo meant for someone names them: `@<agent>`; a memo that only informs names no one.
- Close with `hl task done T<n> <pushed sha>`. A wrong result is reopened by its asker (`hl task reopen T<n> "<why>"`).
- Work lands on main directly, as above (Landing), with no story branches and no founder merge step.
- New work goes in Go. The Python package `handline/` is gone; Python stays only in `tools/` and `contract/` (dev and test harnesses).
- A screen change takes a before and after picture of each screen it adds or changes (PC and phone
  where both change) and runs the `ui-fresh-eyes` subagent with only those pictures, the request line, and whether a test app is running.
  When its answer matches the request, post the pictures and its answer verbatim on the task
  (`hl task comment T<own> add --attach before.png --attach after.png "<its answer>"`) and land.
  Put the founder on the task (`hl task comment T<own> add "Look: <one line>" --turn human`) only when it misreads the screen twice or a design choice (A/B) is needed.
  Their comment gives the task back: "Approved" closes the look; "Change asked: <words>" is what to fix, then run it again.
- A task or a say can be refused; the refusal says why, and the policy behind
  it is deliberate, not a fault to route around. A name that is not on
  `hl team` (a bare `auditor` for `<team>.auditor`) is refused with "unknown
  target" in the command's own output - read it, or you wait for a reply that
  never comes.
- Agents never open a task for the founder. News on a task is `hl task comment T<n> add "@human ..."`; news with no task is `hl say human "..."`.

## Repositories

- `handline-dev/handline-ide`: the working repo. Clean history, no old PRs.
- `handline-dev/handline-legacy` and `handline-dev/handline-old3`: archives. Read only; do not push there.
- `handline-dev/handline-old`: private archive of the old history and PRs. Read only.
- `handline-dev/handline`: the tidy public repo, opened by the founder after the improvements. Nothing is made public before then.
- The mobile app is hands-off: no Android builds.

## Hard rules

1. Never read, write or copy a credential: `.credentials.json`, the Keychain.
   Adding an account is one `CLAUDE_CONFIG_DIR=<dir> claude /login` run by a person; a refresh token rotates
   once, so copies invalidate each other. Two exceptions: a driver's meter
   path may read an opencode `auth.json` in memory, once, for the usage question
   only; `Driver.profile` and `Driver.login_id` in the claude driver are the only readers of account values in
   `.claude.json`: the first returns only the subscription start and the two rate-limit
   tier strings, the second only a one-way fingerprint of the account id, which keys the
   stored email, so it is asked of claude only when the login is new. The founder:
   "Go with A. Reading is fine. Just do not expose a token or the like that you read."
   Read is allowed; exposing a token, key or other secret from it is not. No key,
   token, email or other value from either file reaches an agent's env, prompt,
   terminal, log, db or any file Handline writes. The daemon writes four non-secret keys there, under
   claude's own lock, every other byte left as it was: the worktree it launches an agent in is marked
   trusted (`projects[<path>].hasTrustDialogAccepted`), and `resumeReturnDismissed`,
   `hasCompletedOnboarding` and `hasSeenAutoModeOutsideReadPrompt` are set true so a launch or a read outside
   the working folders does not stop on a screen nobody answers. One more, temporary, by the founder's word: a claude
   account's coding-plan key goes into the env of the agent launched on it, nowhere else.
2. Do not code on an assumption nobody verified. Probe it first.
3. No logic in `SKILL.md`: it is a stub of at most 20 lines, and detail loads on
   demand from the docs.
4. No model in orchestration. Triggers, account picks and resume decisions are
   deterministic and the daemon makes them. The one place a model spends tokens is
   a fold.
5. Never dump `--json` output whole into a log or the db (a token leak path):
   take the fields with a reader, by allowlist.
6. Nothing the repo keeps (commit messages, code, comments, tests, docs) carries personal
   data or an id that only means something on this machine: no email, real name, home
   path (`/Users/<name>`), token or key; no task id (`T<n>`), chat message number (`#<n>`)
   or internal agent name. Write the why in words instead, so a stranger reading the
   repo can follow it. The task board keeps the ids. A real need (a fixture that must hold one) gets
   `local-ref-ok` on its line, or its file an entry in `tools/localrefs.allow`. In a Markdown
   file an id that is an argument of an `hl` command is the product's own syntax and passes.
   A commit's author and committer email must be a noreply address too
   (`git config user.email <id>+<name>@users.noreply.github.com`).
   `tools/localrefs.py` enforces it (a push to main also runs `localrefs.py authors` in `check.sh --group files`, so a fresh clone without the hooks is caught; set the email before the first commit in any clone), and refuses an @-mention outside code in a commit message, in the git hooks
   (`tools/install-git-hooks.sh`), and CI (pr-rules) runs it on each push:
   `tools/localrefs.py diff main...<sha>` and `tools/localrefs.py log main..<sha>`.
7. A leftover on one machine (a stuck row, a stale card) is cleaned by hand and reported on the task: cancel it in
   the app, or run one sql line and quote it. Never with code the repo keeps: no migration, sweep or helper for it.
   A migration is for a schema or data change every install needs. Auditors FAIL a diff that adds one-off cleanup code.

Easy to get wrong:

- Count a transcript once per API response (`requestId`), never per line: one
  response is several JSONL lines repeating its usage (`token_calls` is keyed on it).
- A fold runs outside the session, so its input does not grow with the session.
- A fold's cursor is a row id, never a time, and moves only after the output
  validates (schema, and nothing `done` before is gone).
- Triggers are level triggers: an edge trigger misses a crossing that happened
  while the daemon was down.
- Everything under `~/.handline` is writable by every agent, so it cannot prove
  the founder asked for something. What only the founder may grant (the declared
  trust roots, the founder's answer to a roster or trust proposal) is kept in `go/kit/protected`, outside it. The
  proposal drafts stay in `~/.handline`: an agent writes those, so only the
  recorded answer counts.

Your role and standing context are injected per
agent from the daemon's database; this file does not repeat them.

## How we work

- Tests: write unit tests for logic (Go, contract, app-core, app-data, electron main) in the same
  commit as the change. A screen change posts pictures and adds no render test; if it changes a kept
  test file, update that file (`apps/web/test/<name>.test.tsx`, `apps/mobile/test/<name>.test.tsx`).
  To see or press a screen in the desktop app, `node tools/app.mjs start|shot|click|type|text|eval|drag-check|stop` runs a separate test instance (never the user's window); `stop` it when done.
  Kept test files, web: app, chat-markdown, terminal-xterm-wiring, terminal-copy-paste, route-leave, task-refs, spans,
  command-card, resolve-images, sendKey, by-team, team, chat-fold, screenKeys, solved-stall. Mobile: dev-pair,
  termspans, app-update, scan-pair, chat-scroll. Work is not done until the tests pass. While iterating, run only the
  tests you touched (`cd go && go test ./internal/<pkg> -run <name>`; for an app, the test files you touched and
  the package typecheck); run the whole suite only through `tools/check.sh` before the push. For a Go change, also run Go with `-race` at the exact sha
  (`HANDLINE_GO_RACE=1 tools/test-go.sh`: the variable adds `-race` to the Go tests, and only this run sets it; in the foreground with a 600000 ms Bash timeout, never in the
  background); a count you quote is not a verdict. `tools/check.sh` runs nothing for documentation; for anything else
  it runs, in order: the app checks (`pnpm turbo test typecheck lint` when an app or JS workspace file changed, else
  `pnpm --dir packages/app-data test`), the contract's generated files being current, then the Go suite (`tools/test-go.sh`).
- Live checks: run them yourself, one short run each, and put the output in the task. They are opt-in:
  `HANDLINE_LIVE=1 tools/live-e2e.sh`, or `HANDLINE_LIVE=1 go test -run TestLive... ./internal/...`. Each spends
  the account with the most headroom now (`tools/live_account.py` says which and why) and runs a test daemon on a
  scratch home, never the live one. Never from check.sh or CI. Do not send a live run to the founder.
- TypeScript: run the tests of the files you touched and the package typecheck before you hand a task over. Aim for files under 250 lines and
  functions under 60; lint fails at 400 and 120 (tests 600, components 250), counted without blank or comment lines.
  A `max-lines` error means split the file by responsibility, not raise the limit or add a disable. The numbers are in
  `packages/config/eslint/limits.js`.
- Landing: no pull requests. Keep your meaningful commits (no squash); fold review fixups into the commit they fix. Just
  before pushing, `git fetch` and `git rebase origin/main` (no merge commits), get `tools/check.sh` green, have a review
  subagent read the diff, then `git push origin HEAD:main` (fast-forward only, never force). If the push is refused,
  fetch, rebase and check again. Whoever turns main red fixes it first.
- A new limit, hold or refusal merges off by default, behind a setting the app can change; its default becomes on only when the founder asks for that change.
- UI: don't design a screen alone. Agree on its shape with the human first,
  and after you build it, the human checks it by eye. Passing tests do not
  finish UI work. When the founder must look, write the review memo (above).
- Language: agents write to each other, write tasks, code and comments in
  English. Only a reply to the founder may be in Korean.
  The app's own text (labels, buttons, messages) is English; a Korean memo
  quotes screen text in English, as the screen will show it.
- Run `hl` as its own Bash call with literal text: no `$(...)`, no `$VAR`, no
  `&&`, `;` or pipe. A value such as a sha comes from a separate call first.
- Permission prompts: edit files with the Edit tool, never `sed -i`, which can
  write any file and so always prompts. Don't start a command with `cd`; use
  paths or `git -C` instead.
- Text to `hl say`, `hl task new` and `hl task handoff` goes in single quotes
  (a literal `'` as `'"'"'`): in double quotes the shell runs backticked words
  and they vanish from the message. Text with backticks or `$(`, or with a literal `'`
  in it, goes in a quoted heredoc on stdin as `-`, or in a file as `--file PATH`
  (`hl say`, `hl task new`, `hl task handoff`, `hl task comment add`).
- A message of more than a sentence goes in lines, in Markdown, through a
  quoted heredoc: `hl say <target> --file - <<'EOF'`, the body, then `EOF`.
  The receiver and the app see its lines.

## Pitfalls when building

- Git: if you squash, reset onto your merge base (`git reset --soft $(git merge-base HEAD origin/main)`), never
  `origin/main`, which reverts what landed since. Before every push, `git diff --stat origin/main...HEAD`
  lists only your files. Commit before any `git checkout -- <file>` (restoring a mutant, a before picture): it wipes
  uncommitted edits. Add files by name, so a leftover probe test is not swept in.
- Pass `origin/main` to `tools/check.sh` (it defaults to local `main`). Judge it by its own exit code with the
  log in a file (`> log 2>&1; rc=$?`), never through a pipe, `tail` or a trailing `echo`: zsh has no `PIPESTATUS`, and a
  wrapper exits 0. An exit 1 with every test passing is Prettier (`[warn]` in
  the log) or a typecheck (`Failed: ...#typecheck`).
- Go: `go test ./...` in go/ skips the nested modules (each `go/*/go.mod`); `tools/test-go.sh` tests each, and a
  nested module cannot import the daemon's module (go/). Never `go clean` the shared build cache; on `could not import ... go-build ...
  no such file`, rerun with a private `GOCACHE`. A Go test binary that links `go/kit/paths` drops an inherited
  `HANDLINE_HOME` that names the live home, so a test there gets a home only from its own `t.Setenv` or fixture. Timing knobs tests change live on Env, not package vars.
- Tests: a mutant that does not build or times out is not a kill. A change that shares a component tests each line
  that wires it. Before deleting a bound a mutant survived, send it extreme values (MaxInt64, negative). A stub script
  written per run is slow on its first exec under load (macOS checks it): check one in instead.
- The database is `~/.handline/db.sqlite`; read it with `sqlite3 -readonly`. The live daemon's log is
  `~/Library/Application Support/Handline/daemon.log`. Cast `strftime('%s', ...)` to int before
  comparing it to an expression, or the filter silently matches nothing. A higher `events.id` is not a later time (a
  late hook keeps its own time), and stored hook payloads are slimmed: a field missing there says nothing about what
  Claude Code sent; read its binary (python mmap and a regex; `strings` is too slow). To hide a request from a view,
  mark it; never change its requester, which routing reads.
- Local runs: a scratch daemon home needs a short path (a unix socket path over about 104 characters fails with
  `bind: invalid argument`). A port may be another agent's: check `lsof -iTCP:<port> -sTCP:LISTEN` first. Moving an
  account folder logs it out (its Keychain login is keyed by the folder's path).
- The live daemon is the app's: starting the app resets its checkout to main and rebuilds, so never swap a binary by
  hand. A change to a path or name baked into code rebuilds hl and handlined and greps each binary for the old name.
- Contract: edit `contract/spec/*.json`, then `python3 contract/generate.py`; never hand-edit a generated file.
- Web: with React Compiler on, read changing state only through a hook's return value, or it goes stale silently.
  One shortcut scheme app-wide: click opens here, Cmd-click, middle-click or Cmd-Enter a new tab, Option-click beside.
- A check of code belongs in CI, not in the daemon, which only steers agents. The Claude path comes first: no work whose
  only purpose is another agent CLI.

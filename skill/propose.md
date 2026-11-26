# Shaping a team (`hl propose`)

Read it before `hl propose add`, `hl propose team`, `hl propose retire`, `hl propose rename` or a role change (`hl propose context`).

- **Shape.** Each team has a lead and builders. The lead routes work and does not build. Builders own one layer or area each.
  Review is a subagent a builder runs inside its own task before landing, not another agent's step.
- **Landing** follows the repo's own rules (its AGENTS.md, CLAUDE.md or CONTRIBUTING).
- **Checks live in CI.** Tests, lint and the file rules run in CI. The daemon only steers agents; do not put a check in a role
  that CI can run.
- **UI work** comes with before and after pictures of each screen it changes.
- **Wait on gates.** Block a task on a pull request's CI or on another task (`hl task wait`, `--blocked-by`); do not hold it or poll.
- **Accounts.** Pace each account to its weekly limit instead of burning its 5-hour windows, and size a team to what the accounts'
  week can carry.
- **The founder** gets one decision at a time, in plain words, with your recommendation.
- **Retiring.** Before `hl propose retire`, hand each of the agent's open tasks to a new owner (`hl task handoff`); a retire is
  refused while a worktree of the agent holds work.
- **Several changes are one bundle.** `hl propose bundle --file changes.json --why "..."` (a JSON list of
  `{"kind", "name", ...}`) asks the founder once and applies all or nothing. Say the reason in words, not message numbers.

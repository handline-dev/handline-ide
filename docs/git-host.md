# GitHub and GitLab on the task

Pull requests, reviews and CI shown and acted on from the task that holds the work.

## What exists

- A task names its pull request (`task.edit pr`, `Task.pr`).
- `worktrees.list` gives each branch's pull request and what holds it (`go/internal/reads/prverdict.go`: merged, draft,
  checks failing, changes requested, waiting for review, ready), read with `gh pr list`, kept a minute per branch.
- A task can wait on a gate that watches a pull request's CI or its close, or an issue being finished
  (`hl task wait T<n> --pr <n>`, `go/taskboard/watch/github.go`).

All of it goes through the `gh` CLI's own login: Handline never sees a token.

## Rules for what follows

- **Identity:** whatever each team configures for its agents (their own `gh` login, a bot account or an app), given to
  the agent through its environment. The daemon uses the identity the agent's environment already has and never assumes
  the founder's login.
- **Poll, no webhooks:** conditional requests (`ETag`), so an unchanged pull request costs no rate limit, about once a
  minute.
- **One provider interface** (get a pull request, its comments, post a comment, its checks) with GitHub first and GitLab
  as a second implementation.
- **The host is the gate:** the daemon never merges on its own and never changes branch protection; it reads required
  checks and approvals.

## Still to build

1. **Read-only on the task.** The task page (PC and phone) shows its pull request's state, CI rolled up, and review
   threads waiting. A review comment that arrives is said to the agent holding the task, linked to it, as a new turn of
   the same agent.
2. **Reply and resolve** a review thread from the task, as writes through the provider.
3. **GitLab** through the same interface: merge request IID, pipeline status, note events, discussion reply and resolve.

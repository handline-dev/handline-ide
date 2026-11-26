# GitHub and GitLab on the task

Pull requests, reviews and CI shown and acted on from the task that holds the work.

## What exists

- A task names its pull request: the number and the repository it is in (`Task.pr`, `Task.pr_repo`). `task.wait` on a pull
  request fills both in the same write; `task.edit pr` sets it by hand, and `hl` sends the origin of the checkout it runs in
  as the repository. A number with no repository is read in the repository the task's holder works in.
- One read of a pull request (`go/taskboard/watch/pull.go`): state, draft, base branch, head and merge commits, the review
  decision and each check by its latest run. A gate, a branch's verdict and a task's card all come from it, so they cannot
  disagree, and it is asked about once a minute at most outside a gate's own passes.
- The task page shows its pull request in a card (`task.pr`): open, draft, merged or closed, the failed checks by name, how
  many still run and passed, the review's decision, whether its merge closes the task, and a link out.
- `worktrees.list` gives each branch's pull request and what holds it (merged, draft, checks failing, changes requested,
  waiting for review, ready), read with `gh pr list`, kept a minute per branch. The app shows it as a chip beside the
  branch, under an agent's chat and in the Changes tab.
- A task can wait on a gate that watches a pull request's CI, its close or its landing, or an issue being finished
  (`hl task wait T<n> --pr <n>`, `go/taskboard/watch/github.go`). `--until landed` gives the task back as soon as the pull
  request needs its holder (CI red, changes requested, closed), and with `--then done` its merge closes the task with the
  merge commit ([watch-vendors.md](watch-vendors.md)).
- When a pull request could not be read, the screen says why: gh not installed or not logged in on the daemon's machine, a
  pull request that login cannot see, or a number with no repository.

All of it goes through the `gh` CLI's own login: Handline never sees a token.

## Whose login

- **Reads use the login of the machine the daemon runs on.** Every read the daemon makes of GitHub (a pull request for a
  gate, a card or a branch's verdict; a repository's open issues) runs `gh` as the daemon's own process, so it sees what
  that machine's `gh auth login` sees: on a personal Mac, the founder's. No agent's identity is used for a read, and no
  token is read, stored or passed on.
- **Writes are the agent's.** An agent pushes its branch, opens its pull request and comments with whatever identity its
  team gave its environment (its own `gh` login, a bot account or an app). The daemon writes nothing to GitHub: it never
  merges, comments or changes branch protection.
- **Runners.** `gh` runs only on the master. An agent on a runner opens its pull request under the runner's login; if the
  master's login cannot see that repository, every read of the pull request comes back as gone: its card says so, its
  branch shows no chip, and a gate on it fails after three such reads and gives the task back with `gone: ...`. Give the
  master's login access to the repository; a per-agent identity for reads does not exist.

## Rules for what follows

- **Poll, no webhooks:** conditional requests (`ETag`), so an unchanged pull request costs no rate limit, about once a
  minute.
- **One provider interface** (get a pull request, its comments, post a comment, its checks) with GitHub first and GitLab
  as a second implementation.
- **The host is the gate:** the daemon never merges on its own and never changes branch protection; it reads required
  checks and approvals.

## Still to build

1. **Review threads on the task.** The task page shows the review threads waiting. A review comment that arrives is said to
   the agent holding the task, linked to it, as a new turn of the same agent. It reads the pull request through the one
   read above.
2. **Reply and resolve** a review thread from the task, as writes through the provider.
3. **GitLab** through the same interface: merge request IID, pipeline status, note events, discussion reply and resolve.

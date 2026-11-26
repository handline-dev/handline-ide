# Gate watches and their vendors

A gate is a task owned by `gate` that the daemon holds. It watches something outside handline, and the daemon marks it done
when that changes. A task blocked by the gate then comes back to its holder's queue. `hl task wait` opens one in a single
write (`task.wait`): it opens the gate, blocks the task on it and, if the task is doing, releases it. A wait on a pull
request also names that pull request as the task's (`Task.pr`, `Task.pr_repo`), in the same write. With `--then done` the
gate closes the task when it passes, in place of giving it back (see "A gate that closes its task").

## What a watch is

A watch is `{vendor, target, until}`:

- **vendor** is the outside tool. The one vendor is `github`.
- **target** is the thing, written the way that tool writes it. Only the vendor reads it.
- **until** is what the gate waits for, from one shared list: `ci` (the checks finished), `closed` (a PR merged or closed),
  `landed` (a PR merged, or it needs its holder), `done` (an issue finished). Each vendor takes some of them. If none is
  given, the vendor's first is used.

A vendor answers each read with `waiting`, `passed`, `failed` or `gone`, plus one line of evidence. The gate's result is
`passed`, `failed` or `timed_out`, and its done evidence is the vendor's line.

## The gate (`go/internal/gates`)

Every 60 s the daemon reads each open gate, one at a time, on a clock of its own (not the tick):

- **passed or failed:** the gate is done, with the vendor's evidence.
- **gone** (deleted, or this login cannot see it): after 3 gone reads in a row the gate is failed, with evidence
  `gone: <why>`. Any other answer starts the count again.
- **A read that fails** (network, rate limit, logged out): the gate keeps watching, and its status says
  `watching <label> - last read failed: <why>` until a read works again.
- **Past the deadline** (its time box): the gate is `timed out`, and its target is not read. With no time box given the
  deadline is 24 h, or for `landed` the setting `landed_deadline_s` (a week): a review can take days, and a gate that timed
  out daily would bring its task back to its holder each time, to no use. A time box an agent gives is at most a task's
  longest (4 h).

Every open gate is looked at on every pass, whatever the daemon remembers: what one pass could not finish, the next does,
and so does a daemon that starts later.

When a gate opens, the holder of the task it blocked is told its evidence in the unblock notice. The task keeps it too:
`task.get` lists the gates the task waited on that have opened, newest first (`TaskDetail.gates`: what each watched, its
result and its evidence), and `hl task show T<n>` prints them under `waited on`. That is how an agent learns which checks
failed or that changes were requested: it cannot read the pull request itself (`task.pr` is a person's read).

## A gate that closes its task (`--then done`)

`hl task wait T<n> --pr <n> --until landed --then done` opens a gate that names the task it closes (`TaskWatch.closes`).
Only a watch whose pass names a merge commit can: a pull request until `landed`. The gate closes the task in its holder's
name, so asking for it is asking for a done: `task.wait` refuses `then done` from anyone who may not mark the task done (its
assignee, its owner, its parent's owner, the founder). Anyone may still make a task wait without it. When it passes:

1. Outside any write, the daemon fetches origin's default branch in the folder the roster keeps for the agent acting on the
   task (`git fetch origin <default branch>`, nothing prompting, 60 s at most, over https, ssh or git only), and looks
   for the merge commit on that branch: origin's copy of it when the clone has one, else the local branch. It fetches
   nowhere a caller names: a fetch honours a repository's config. Every git question the done needs is asked here, so no
   git runs while the write holds the database.
2. In one write it marks the gate done (`merged <sha>`) and the task done, in that agent's name, with the merge commit as
   the evidence, as a review turn's pass closes a task. It is a done like any other: the workspace's `review_required`
   applies, and the task's result says the commit is on the default branch. The evidence is never words.

One write means no daemon death leaves a gate that passed beside a task it did not close.

- **The fetch fails** (network, credentials, a timeout): nothing is closed. The gate stays open, its status says
  `watching <label> - passed, not closing T<n> yet: <why>`, and the next pass tries again, until the gate's deadline.
- **The origin cannot be fetched at all:** an origin that is a local path, or any transport other than https, ssh and git,
  is refused by the daemon's fetch (`transport 'file' not allowed`), and no later fetch goes better. The gate does not wait
  out its deadline on it: it checks the clone as it stands. A clone that already holds the merge on origin's copy of the
  default branch (someone fetched it there) closes the task. Otherwise, after 3 passes (about 3 minutes), the gate opens as
  passed with `merged <sha>; T<n> not closed: its merge commit <sha> is not on origin/<branch> of <agent>'s repository, and
  the daemon cannot fetch that repository's origin to look again (git fetch: transport 'file' not allowed: it fetches over
  https, ssh, git only)`, and the task goes back to its holder, who closes it with `hl task done T<n> <sha>` after a fetch
  of its own.
- **The done is refused** (an open sub-task, another blocker, a review the workspace wants), **the merge is not on the
  default branch** after 3 passes that fetched (a merge into another branch, or in another repository), **the merge names
  no commit**, the agent has no folder on this machine, or the task is a person's turn: the task is not closed. The gate
  opens as passed with `merged <sha>; T<n> not closed: <why>`, the same is left on the task as a comment, and the task
  goes back to its holder as from any gate.
- **The task no longer waits on the gate** (unblocked by hand): the gate opens as passed and touches nothing.
- **Failed, gone or timed out:** the task goes back to its holder, as from any gate.

## What a vendor implements (`go/taskboard/watch`)

A vendor is one file, `go/taskboard/watch/<vendor>.go`, listed in `watch.Vendors`. It has four methods:

- `Untils()` lists the untils it takes, the default first.
- `Parse(target, until)` checks the target without reading the tool. It returns the target written in the vendor's one
  canonical form, and a label people read (`PR #418 CI`). An error refuses the watch as a bad request.
- `Ready(ctx)` says whether the vendor is set up, and why not. A watch whose vendor is not set up is refused. This is checked
  last, so every other refusal reads nothing outside.
- `Check(ctx, target, until, since)` reads the target once; `since` is when the gate started. An error means it could not be read;
  it is never a verdict.

A pass that lands a commit says so as `merged <sha>` (`watch.MergedSha` reads it back): that is what a gate closes a task
with.

No secrets go in the database, on the wire, or in an agent's settings. A vendor whose tool has a CLI with its own login uses that CLI.

A new vendor ships: its file, a test that feeds `Check` recorded tool output for each until and each failure case, and a
section below.

## GitHub (`github`)

One read of a pull request serves everything that shows or waits on one (`go/taskboard/watch/pull.go`): a gate, a branch's
verdict in `worktrees.list`, and a task's card (`task.pr`). It takes only these facts from what gh prints: state, draft,
base branch, head and merge commits, the review decision, when a reviewer last asked for changes (never who), and each
check by its latest run. A gate reads afresh on each pass; a card and a branch's verdict take what was read within the last
minute, a failed read included, so a gh that is down is asked once a minute.

| | |
|---|---|
| target | `owner/name#N`, e.g. `handline-dev/handline#418`. GitHub numbers PRs and issues together: `ci`, `closed` and `landed` read `#N` as a PR, and `done` reads it as an issue. A number of the other kind reads as gone. `hl task wait T<n> --pr 418` is `github:<this checkout's origin>#418`. |
| a PR's read | `gh pr view N -R owner/name --json number,url,state,isDraft,baseRefName,headRefOid,mergeCommit,reviewDecision,statusCheckRollup,commits,latestReviews`. Only the latest run of each check (the one that started last) counts: a run that was cancelled or failed and run again is superseded. A check run is waiting until its status is `COMPLETED`, and ok when it concludes `SUCCESS`, `NEUTRAL` or `SKIPPED`. A commit status is waiting while `PENDING` or `EXPECTED`, and ok when `SUCCESS`. A finished check that is not ok stands failed. |
| until `ci` (PR) | Once nothing waits: all ok is passed (`ci passed`); anything else is failed (`ci failed: <names>`). A PR that is merged or closed while a check still waits is failed (`closed before CI`). A PR with no checks is waiting until 10 minutes after its head commit or the gate's start, whichever is later, then passed (`no CI on this PR`). |
| until `closed` (PR) | `MERGED` is passed (`merged <sha>`). `CLOSED` is failed (`closed unmerged`). `OPEN` is waiting, whatever its checks say. |
| until `landed` (PR) | `MERGED` is passed (`merged <sha>`). It fails as soon as the PR needs its holder: `CLOSED` (`closed unmerged`); a check standing failed, even while others run (`ci failed: <names>`); a reviewer asking for changes after the gate started (`changes requested`). A request for changes made before the gate started waits: GitHub keeps that decision until the reviewer looks again, so a second wait after the fix would otherwise fail at once. Checks running, a review not given, an approval not merged yet and a draft all wait. |
| until `done` (issue) | `gh issue view N -R owner/name --json state,stateReason`. `CLOSED` as `COMPLETED` is passed (`closed as completed`). `CLOSED` for any other reason, such as `NOT_PLANNED`, is failed (`closed as not planned`). `OPEN` is waiting. |
| auth and setup | The gh CLI's own login on the machine the daemon runs on: `gh auth login`. Ready is `gh auth status` exiting 0. handline never sees a token. A screen that could not read a pull request says which it is: gh not installed, or not logged in. |
| cost | One gh call (one GraphQL query) per gate per read: 60 an hour per gate, of the 5000 an hour GitHub allows. A card or a branch's verdict of the same pull request adds none within the minute. |
| failures | `Could not resolve to a ...` or HTTP 404: gone (a deleted target, or a repo this login cannot see). Anything else (network, 5xx, rate limit, logged out) is a read that failed. |
| a PR this login cannot see | GitHub answers a pull request the login may not see the same as one that is not there, so it reads as gone: the gate fails after 3 such reads (about 3 minutes) with `gone: <what gh said>`, and the task goes back to its holder, who can tell from that line. This is the case of an agent on a runner that opened the pull request under the runner's own gh login, in a repo the master's login cannot see: gh runs only on the master. Give the master's login access to the repo; nothing else makes the wait work. |

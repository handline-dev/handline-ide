- Keep messages short. Reply when a message asks you something or you owe a result; one that only informs needs no reply, except the founder's.
- Send whoever opened your task no progress or FYI messages: it hears from you only when the task is done or you are blocked.
- Never schedule your own wakeup, /loop or poll to wait for a reply or a review: Handline types the answer into your session when it comes.
- A task or message to the founder starts with one line saying what they must do (answer, OK, pick), then the reason in one sentence; attach the screenshot when it is about something visible.

Working with peers:
- Anyone gives anyone work as a task; correct a task you gave with a memo, not a message behind it.
- Record a wait as `--blocked-by` on the task, not in prose.
- Check main before you open work, so nobody rebuilds what landed.
- Quote the founder's own words in the task you open for their request, and answer the founder through `hl`, never in chat.
- The contact passes work from another workspace to the peer it fits, and does its own work too.

<!-- protocol -->
<!-- Above: the house rules a new team's rules file starts as (team.SeedRules writes that part, as it is). Below: the team protocol,
which every agent follows whatever its team's file says. Each "## <key>" is one statement, read by key (team.Protocol) by the launch
prompt, the typed delivery texts and the say refusal, and copied into the generated blocks of AGENTS.md and skill/SKILL.md, which
TestProtocolDocsAreCurrent compares with it. -->

## terminal
Talk to people only through hl; what you write in your terminal reaches no one.

## ends
A task you were handed ends with `hl task done T<n> "<evidence>"` or `hl task handoff T<n> <agent> "<note>"` - that is your report, even a deferral: the next task arrives only then, and an answer typed in chat reaches no one.

## land
Land your work the way the repo's own rules say (its AGENTS.md, CLAUDE.md or CONTRIBUTING). When they say nothing: if you are on the repo's default branch, make a branch first; commit, then, if the repo has a GitHub remote, push your branch and open a pull request with `gh pr create --fill`; with none, commit and stop. Name the commit or the pull request in your done.

## progress
Work in steps is reported at each step with `hl progress --done N --of M "<what you are doing now>"`: the task shows the count.

## one-at-a-time
You do one at a time, in this order: each ends with `hl task done T<n>` or `hl task handoff T<n> <agent> "<note>"`.

## wait
Never end a turn holding a task you are not working on: it blocks the tasks queued behind it. Finish it, or block it on what it waits for (`hl task edit T<n> --blocked-by T<m>`); it comes back to your queue when that ends.

## correct
To change a task you gave that is not yet done, write a memo on it (`hl task comment T<n> add "..."`), never a message behind it.

## review
A done closes the task; nothing waits on an accept. Review is a subagent you run before the push, not a handoff.

## founder
When you cannot go on without the founder's decision, put them on the task you hold: `hl task comment T<n> add "<first line: the decision and what you recommend; then why>" --turn human`, with `--attach <screenshot>` when it is about something visible, then take your next task or stop; their comment gives it back to you. News for the founder that needs no decision is a comment on the task that starts with `@human`; it moves no turn. An `@human` or `--turn human` comment already shows in the founder's chat with you; do not repeat it with `hl say`. A question to the founder carries its content: put a file on with `--attach`, never a bare path, which they cannot open. Work you open from the founder's comment that another agent passed to you serves his request: `hl task new ... --request T<the task he commented on>`.

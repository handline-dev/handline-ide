- Keep messages short. Reply when a message asks you something or you owe a result; one that only informs needs no reply, except the founder's.
- Send the agent you report to no progress or FYI messages: it hears from you only when a task is done or you are blocked.
- Never schedule your own wakeup, /loop or poll to wait for a reply or a review: Handline types the answer into your session when it comes.
- A task or message to the founder starts with one line saying what they must do (answer, OK, pick), then the reason in one sentence; attach the screenshot when it is about something visible.

If you have reports:
- You do not implement: give the work to a report and read what it sends back.
- One task at a time per report. Correct a task with a memo, not a message behind it.
- Record a wait as `--blocked-by` on the task, not in prose.
- Check main before you assign, so nobody rebuilds what landed.
- Quote the founder's own words in the task you open for their request, and answer the founder through `hl`, never in chat.
- Finding the bottleneck is your job. At each periodic check, look at each report's queued count, its oldest wait and the work blocked behind it, and compare with the last check. When a wait passes 1h or a queue grows two checks in a row, ask that agent what slows it and open the work that fixes the cause. No dead or stalled agent does not mean healthy.

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
Land your work the way the repo's own rules say (its AGENTS.md, CLAUDE.md or CONTRIBUTING). When they say nothing: commit on your branch, then open a pull request with `gh pr create` if the repo has a GitHub remote; with none, commit and stop, and name the commit in your done.

## one-at-a-time
You do one at a time, in this order: each ends with `hl task done T<n>` or `hl task handoff T<n> <agent> "<note>"`.

## wait
Never end a turn holding a task you are not working on: it blocks the tasks queued behind it. Finish it, or block it on what it waits for (`hl task edit T<n> --blocked-by T<m>`); it comes back to your queue when that ends.

## correct
To change a task you gave that is not yet done, write a memo on it (`hl task comment T<n> add "..."`), never a message behind it.

## review
A done closes the task; nothing waits on an accept. Review is a subagent you run before the push, not a handoff.

## founder
When you cannot go on without the founder's decision, put them on the task you hold: `hl task turn T<n> human "<the decision, and what you recommend>"`, then take your next task or stop; their comment gives it back to you. A question that does not stop the work is an `@human` comment: `hl task comment T<n> add "@human ..."`.

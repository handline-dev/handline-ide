# Review flow: a big goal's split, run, result review, done

The founder's OK on T9325 (comments 8045, 8092). It replaces the story-only review of T9115 step 6 (T9131, T9199) with one flow for every
task that serves the founder, and a review ladder per agent and kind of work.

## Why

On 11-12 to 11-20 the founder was asked to look 287 times at 108 tasks; 36% were OK at first look, 2.7 looks a task. Every look went
through comments (`--turn human` and a picture); the story's OK, merge and Redo were never pressed and Plan OK once, so the review levels
of T9199 counted nothing. Replaying those looks with "step down after a run of first-look OKs" saves 2 looks of 106. The first gain is a
higher first-look OK (the check before the founder, a lesson from each Redo); the ladder pays later.

## The flow

A task serves the founder when it is in a founder request's tree (`requestTreeSQL`) or a person owns it. Such a task, at its level:

1. **Plan review, only for big work.** Work an agent cannot do as one task it splits into a story's drafts; the split (the design and the
   list of pieces in order) is the plan the founder reads before Run, as Story 4's was. Nobody else judges small from big: one task is
   small. No task gets a plan of its own unless the founder asks for one on its draft (Ask for a plan); drafts are no longer asked for
   plans by themselves (T9325 8086 to 8092).
2. **Run.**
3. **Result review.** Its finish is Ready for the founder (today's `task_ready`) instead of going to its land_by agent; OK lands it, Redo
   sends it back with the note, from which the assignee keeps a lesson.
4. **Done.**

The founder's comment on a task waiting for a review is a message to the assignee, as on a Ready task today; only OK or Redo ends the wait.
Agents stop asking "OK?" in comments. Off by default (setting `review_flow`) until the founder has tried it.

Work the founder does with an agent in chat makes no task (16 of his 224 chat messages since 11-13 became one), so it has no review flow:
the conversation is the review. A task an agent opens from that chat, to hand work on or keep it past the conversation, takes the flow
(T9325 8060, 8064).

**What each shows.** A plan: what changes for the founder, the core logic (a few lines of code or the rule) and his choices, not files
or components (T9338). A result: pictures or video first (before and after), then the code, the tests, and the check's findings.

## The ladder

- Kept per agent and kind of work, for results. Levels: every, 1 in 2, 1 in 3, 1 in 5, never.
- **Down** one level after 10 first-look OKs in a row at the level (a first look is a task's first verdict). The step is offered to
  the founder as one decision ("ui's screen results: look at 1 in 2?"); it never moves down by itself (T9115 7708).
- **Up** to every at once on any Redo, including a later Redo of a task that skipped the founder. Not asked.
- A new agent, or an agent whose model changed, starts at every.
- A skipped review is still checked: the result's check subagent and tests run as today, and a miss or a failed test goes to the founder.

10 in a row is the reduced-inspection switch of ANSI Z1.4; it shows only a miss rate under about 26%, so never is reached slowly.

## Kinds and the floor

The daemon infers the kind from the branch's changed files.

| Kind | Files | Floor |
|---|---|---|
| contract and data | `contract/`, migrations, a store's schema | every |
| screen | `apps/`, the app packages | none |
| daemon | `go/` otherwise | none |
| docs and cleanup | docs, `*.md`, only deletions | none |

A task that touches several kinds takes the strictest level among them.

## Decisions one at a time

The founder's turns (a split, a result, a step down) are the items of Story 9's my-turn list (T3995): one at a time, a recommendation and the
buttons. A big goal is still worked out on the story screen.

## Slices

Each is a draft under T9325.

1. **Every founder task reviews its result.** Result review for tasks that serve the founder, behind `review_flow`; drafts are no longer
   asked for plans by themselves, and the `plans` setting goes. Daemon.
2. **The ladder.** Per agent and kind; kinds from files; the floor; the step-down decision; Redo to every. Replaces T9199's levels.
   Daemon. After 1.
3. **Result on every task.** A task's page shows its result (pictures first, code, tests, check) with OK and Redo, and a plan when one
   was asked. App. After 1.
4. **The ladder on screen.** Settings, Agents, an agent: its level per kind and its record; the step-down item. App. After 2.

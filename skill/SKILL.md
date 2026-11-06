---
name: handline
description: Run as one agent in an handline team. Use to give another agent a task, send a message, check your inbox, or see your handoff after a resume.
---

# handline — you are one agent in a team, named in `$HANDLINE_AGENT`

```
hl team                       who you may reach, and what they are doing
hl task new --assign <target> -t <time> "<title>" "<body>"   work or a question that needs an answer
hl say <target> "<message>"   a message that needs no answer
hl inbox                      the one item waiting for you, and a count
hl task done T<n> "<evidence>"   finish your task (or `hl task handoff T<n> <agent> "<note>"`)
```

A message of more than a sentence goes in lines, in Markdown; the receiver and
the app see them:

```
hl say <target> --file - <<'EOF'
T<n> done: branch x @ abc1234.
- what changed
- what was checked
EOF
```

The rules for any agent tool are in AGENTS.md at the repo root, the source this
skill follows. You are handed one task at a time.
<!-- protocol: correct review wait -->
- To change a task you gave that is not yet done, write a memo on it (`hl task comment T<n> add "..."`), never a message behind it.
- A done closes the task; nothing waits on an accept. Review is a subagent you run before the push, not a handoff.
- Never end a turn holding a task you are not working on: it blocks the tasks queued behind it. Finish it, or block it on what it waits for (`hl task edit T<n> --blocked-by T<m>`); it comes back to your queue when that ends.
<!-- /protocol -->
Child tasks are for separate work, never for review.
To show the founder a screenshot, `hl say human "..." --attach shot.png` (or `hl task comment ... --attach`); a path pasted in the text shows as plain text.
Before you add, retire or reshape agents with `hl propose`, read `skill/propose.md` in the handline repo.

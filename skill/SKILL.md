---
name: handline
description: Run as one agent in a Handline team. Use to give another agent a task, send a message, check your inbox, or see your handoff after a resume.
---

# handline: you are one agent in a team, named in `$HANDLINE_AGENT`

- `hl team`: who you may reach, and what they are doing. `hl inbox`: what is waiting for you.
- `hl task new --assign <target> -t <time> "<title>" "<body>"`: work or a question that needs an answer.
- `hl say <target> "<message>"`: a message that needs no answer; more than a sentence goes in Markdown lines, `hl say <target> --file - <<'EOF'`.
- `hl task done T<n> "<evidence>"` or `hl task handoff T<n> <agent> "<note>"` ends the one task you hold.
- A screenshot goes on with `--attach shot.png` (`hl say`, `hl task comment`); a pasted path shows as plain text.
- Before `hl propose` adds, retires or reshapes agents, read `skill/propose.md`. The rules for any agent are in AGENTS.md.
<!-- protocol: correct review wait -->
- To change a task you gave that is not yet done, write a memo on it (`hl task comment T<n> add "..."`), never a message behind it.
- A done closes the task; nothing waits on an accept. Review is a subagent you run before the push, not a handoff. Ask the founder for a look by finishing the task, never by an "OK?" comment: the daemon puts a finish before them when it is theirs to see. A goal too big for one task is split into a story's drafts, and that split is what the founder reads before Run.
- Never end a turn holding a task you are not working on: it blocks the tasks queued behind it. Finish it, or block it on what it waits for (`hl task edit T<n> --blocked-by T<m>`); it comes back to your queue when that ends.
<!-- /protocol -->

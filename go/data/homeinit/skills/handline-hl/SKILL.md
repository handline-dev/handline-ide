---
name: handline-hl
description: The hl commands an agent runs in Handline: chat, tasks, progress, proposals to the founder, desk and usage. Use when you need to message someone, report or hand over a task, ask the founder to decide or run a command, or look something up in Handline.
---

# hl

`hl` is Handline's command line; your home's daemon answers it. A command given wrong arguments prints its form and an example.

- `hl inbox [--all] [--wait]` your messages; `hl read <msg>` one whole; `hl chat search "<text>"` older ones.
- `hl say <agent> "<text>"` a message; `hl withdraw <msg>` takes one back.
- `hl team` who you can reach and what each is doing.
- `hl task show|list|board|new|comment|done|handoff|pass|back|edit|discard` tasks. A task you hold ends with `hl task done T<n> "<evidence>"` or `hl task handoff T<n> <agent> "<note>"`.
- `hl task comment T<n> add "<text>" --turn human` puts a decision on the founder; `"@human ..."` first is news that moves no turn.
- `hl progress "done: ... left: ..."` keeps a long task's handoff current.
- `hl propose run "<command>" --why "<why>"` for a command you cannot run yourself; other `hl propose` forms change agents, teams, rules, settings and schedules.
- `hl lesson add "<text>" --task T<n>` records a correction.
- `hl use <run|port> -- <command>` takes a shared resource (a port, the simulator, a browser) for one command.
- `hl load`, `hl desk list`, `hl version`.

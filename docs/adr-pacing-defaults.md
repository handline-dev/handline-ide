# ADR: weekly pacing is on by default

Superseded 2026-11-04 (T8456): the pace hold is `launch_pace_hold`, off by default with every other hold on a start, until the founder turns it on (AGENTS.md: a new limit merges off).

Decided by the architect, 2026-11-02 (roadmap P1-7). Status: settled.

## Context

Each account has a weekly budget. Without pacing, the first accounts to be used are run to their
weekly wall early in the week while others sit unused, and then everything stalls at once.

## Decision

- `pacing` defaults to **on** and stays on. It spreads each account's weekly use across accounts
  and across the week.
- The pace line rises from 0 at the weekly reset to `pace_target_pct` (default 98) at the next one.
- An account at or behind the line takes work as usual. One ahead of it by up to `pace_burst_pct`
  points (default 10) gets a smaller cap. One further ahead takes no new turn until the line
  catches up.
- Turning `pacing` off shows the figures without acting on them. It is an escape hatch, not a
  mode to run in.

## Consequences

- No code change: the behavior already shipped. The decision is recorded so the default is not
  reopened as a bug ("why is this account idle at 60%?").
- The keys and their ranges are in [configuration.md](configuration.md#weekly-pacing).

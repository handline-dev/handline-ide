# What to look at now

The app tells the person what waits on them, in one place, with one primary action per item.

## What exists

- **Requests** groups what waits: Your turn, Notes, and Later (an item put off until a time or until a task is done,
  `packages/app-core/src/later.ts`). The daemon's `requests.pending`, and `tasks.list` with `my_turn`, say what waits on the person.
- **Your-turn cards** (`packages/app-core/src/my-turn.ts`): unread items that need an answer, agents that say
  `needs_you`, and open tasks given to the person, ordered by task priority, then deadline (the task clock's `end`),
  then oldest. A dialog the daemon does not know has only Open terminal.
- **Usage:** each account's 5-hour and week gauges with their reset times, and the Outlook page's guess of when a week
  runs out at the pace so far (`stats.outlook`, `runs_out_in_seconds`).

## Still to build

**A stop time per account, where the gauges are.**

```
main    62% of 5 h used · week 41%
        agents stop at 17:40 at this rate (in 1 h 52 m) · resets 19:00
second  12% · the window resets first: no stop time shown
```

- The daemon adds `Account.runs_out_at` (a timestamp or null) to `accounts.list` with `usage: true`, so the line needs
  one call and no text parsing. It is null when the reading is stale, there is no burn, or the window resets first; then
  no stop time is shown, never "safe".
- Warning (amber) from 90% of the 5-hour window or when the stop time is before the reset; red from 95% or under 15
  minutes. The client applies these thresholds.

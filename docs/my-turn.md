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
- **A stop time per account, under its 5-hour gauge** (Usage, and Settings > Accounts): "agents stop at 17:40 at this
  rate (in 1h 52m)", when the window fills before it resets at its use over the last day.

  - The time is the daemon's: `five_runs_out_at` of the account's row in `stats.outlook`'s `by_account`, a timestamp or
    null. It is null when the reading is stale, there is no burn, the window resets first, or the account has no 5-hour
    window; then no line is shown, never "safe". A time that has passed shows no line either: the gauge says the rest.
  - A stop time is a warning (amber), since it is before the reset; it is red from 95% of the window used or under 15
    minutes to go. The client applies these thresholds (`stopTime`, `packages/app-core/src/outlook.ts`). The gauges
    keep their own colours.

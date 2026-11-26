# How it works

The daemon decides what to do with agents and accounts using fixed rules. No model is involved.
The one model call is the daemon's fold, which summarizes recorded events into a digest.
Nothing that moves, parks, or resumes an agent depends on it.

## Usage numbers

Each account reports how much of its five-hour window and its week it has used. The daemon treats
those numbers carefully (`go/accounts/usage`).

- **A number it cannot trust blocks new work.** If there is no reading, or the reading is too old,
  the daemon does not assume the account is empty. Treating a missing reading as zero would make
  the "near the limit" trigger never fire.
- **An account nobody has used is different from an unknown one.** A fresh account reports no
  window at all, which means nothing has been spent. The daemon reads that as 0%, assigns work, and
  marks it `*` on screen.
- **The screen shows where each number came from.** `~` is an earlier reading from a window that
  is still open. `!` is a reading older than the daemon will assign on; it still counts as a floor
  for triggers, because usage inside a window only goes up, but no new work goes there.

## Choosing an account

An agent has a preferred account. A new session starts there unless the account is a bad place to
start: it is at its wall, it is past the line where agents move off early
(`usage_move_before_pct`, 90% of the five-hour window by default), or its five-hour window would not
last to its reset with one more agent on it.

Among the accounts that can take one more agent, the one furthest behind its weekly pace goes
first: its unused week, per hour left before the week ends, is spent soonest. Accounts close to
each other count as equal, and of those the one with the fewest agents mid-turn wins. The pick
fails closed: an agent whose settings or an account whose reading cannot be read picks nowhere.

How fast an account fills is measured, not fixed. The cost of one more agent is its model's
measured spend, turned into points of that account's window; with no measurements the daemon uses
0.75% of the window per minute.

With `account_split` set to `manual` the daemon never places or moves agents: each stays on its
account and waits at its wall. A person's move still goes.

## When an account runs out

A working agent keeps going until it hits the limit. A turn that ends on the usage limit, or a
reading at the wall, means the account is out. Nothing is lost, because the session transcript
keeps the work. An account the daemon cannot read is not treated as out.

When an account is out, the daemon compares costs (`go/scheduler/staymove`):

- An idle agent stays where it is. A move is for work that is waiting.
- An agent whose account resets soon waits for it, if its prompt cache will still be warm then.
  "Soon" is the measured cost of a cold start, as a share of the window (39 minutes until one is
  measured), and never longer than the cache lives.
- Otherwise the agent moves to an account with room, chosen as above.

A resumed session always goes back to the account its session lives on, because a session id
exists only in that account's config directory.

An account a turn found unusable (a refused login, an org that disallows it, a hold or a billing
problem) is stalled: it takes no work until a login or a wake from the Accounts page.

## Context and memory

These are two separate problems with two separate fixes.

- **Handoff frees context.** It records the agent's current request, the path to its transcript,
  and the state of its worktree. Then it clears the session. No model call is needed.
- **Closing frees memory.** An idle session holds hundreds of MB. Closing ends the session and
  keeps the terminal, the worktree, and the account.

Waking a closed session resumes the same session, and costs no more than keeping it open while the
prompt cache is warm: the cache lives on the server and expires by time, not by whether the process
stayed alive.

## Rules that keep it reliable

- **Rules run on stored state, every tick.** If a threshold was crossed while the daemon was down,
  the daemon still acts on it when it comes back.
- **An agent asks another workspace only through its lead.** The refusal says why, so an agent does
  not try to route around it.
- **A result goes back the way the work came.** `hl task done` records the evidence on the task, and
  the daemon tells the one who asked. Without that, agents send a second task to repeat what they
  already reported.

# Best cases

Setups that work. Names in the examples are made up. The commands are real.

## A task with a stated end

Give the builder a task whose body says what "done" is:

```bash
hl task new --assign shop.builder -t 3h "add coupon codes at checkout" \
  "Add a coupon field to checkout. A valid code takes the percentage off the cart total. Done: the suite is green and the sha is on main."
```

The builder writes the change, runs the tests, has a review subagent read the diff before it pushes, and closes the
task with the pushed sha, which `hl` checks is on `main`. A wrong result is reopened by whoever asked
(`hl task reopen T<n> "<why>"`). You read finished results in the app's Requests, not each step.

## Unattended overnight work

Several accounts and a daemon let work continue when one account runs out.

1. Add two or more accounts on the app's Accounts page.
2. Leave the Mac on. The daemon keeps running when the app's window is closed or the app quits.
3. Queue tasks before you leave, each with a clear "done".

During the night the daemon watches each account's usage. An agent that is working keeps going to
the limit, and the transcript keeps its work. If its account hits the limit, the daemon either
lets it wait until the window resets or moves it to an account with room, whichever costs less
(see [how it works](how-it-works.md)). A closed session is resumed when work arrives for it. In the
morning Requests holds what finished, what was refused, and the decisions waiting for you.

## A team

For a project with several parts, propose a whole team in one step:

```bash
hl propose team shop --member sea=$HOME/code/sea --member shore \
  --why "storefront split into search and checkout"
```

The lead is the team's contact: work from other teams comes to it, and it does its own work too.
The members are peers: any of them gives another work as a task. Another team is reached through
its lead.

Start with a lead and two members, and add more when the work calls for them.

## What these have in common

- Every piece of work is a task with a stated end.
- Every result goes back to whoever asked.
- The agent that writes a change does not judge it alone: a review reads it before it lands.
- You are asked only when a decision is yours.

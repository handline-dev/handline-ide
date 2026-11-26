# Handline

You asked AI to build something for you. It worked, so you asked for more, and now several AIs are
working on your project at once. Without noticing, you became the manager of a small team. Handline
is the desk that manager needs.

It is early. We use it every day to build Handline itself, but you cannot install it and use it yet.

## Does this sound familiar?

- You gave work to a few AIs. You cannot tell which one is working and which one has been waiting
  hours for your answer.
- A button in your app looks wrong. You spend ten minutes describing in words where it is and how
  it should look. Circling it on the screen would have taken five seconds.
- A job you expected to take an hour took all day. Nobody can tell you whether the AI was working
  or waiting for something.

## Why not the tools that already exist?

Plenty of tools help you give work to AI: assign a ticket to an AI in a project tracker, line up
cards for AIs on a board, or circle a bug on a screenshot and send it. All of them answer one
question: how do I hand this to an AI?

Handline starts from the next question: when many AIs work for one person, how does that person
keep up? Handline is built to save your attention. This is what it aims for:

- Only what needs you should reach you, as a decision with what you need to make it, for example a
  before and after picture with "I changed it like this. Is it ok?"
- Every piece of work should say what it is waiting for: you, another piece of work, or free memory
  and processor time. Nothing should sit still without a reason you can see.
- You should see where the time went, and when something ran long, the AI that did it should explain
  why.
- Work should start only when there is room for it. Your accounts, your computers and your own time
  are limited, and Handline should plan around them so you do not have to.

## What we hold to

1. You show what you want. A circle on the screen and one line is a complete order.
2. Work lives in tasks. Chat is where a task starts. Everything agreed is kept on the task, so it
   can be found later.
3. You decide and fixed rules run the rest. Which AI starts next and when is decided by rules you
   can read, not by another AI's judgment.
4. Some things only you may approve. An AI cannot approve its own work.

## What it should feel like

You open the app in the morning. It shows what finished overnight and the two things that wait for
you. One is a question with a before and after picture. You tap yes. The other is a review that
failed, with the reason in one line.

You notice a page looks wrong. You capture it, circle the field and type "line this up with the
others". That becomes a task. An AI picks it up when one is free, and an hour later the same task
asks you to look.

At the end of the week you look at one map of all the work: what is done, what is moving, and what
waits on you.

## What it does

- Orders by pointing at the screen, and the result reviewed inside the task.
- New work held while the computer is short of memory, with what is slowing it down shown.
- Command windows in the same app, next to the AIs.
- More than one computer: add another machine, such as an old laptop, a small desktop or a server
  you rent, and Handline spreads the work across them.

## Where it is going

1. Pull requests, reviews and CI shown on the task, from GitHub or GitLab.
2. The app tells you what to look at now.
3. A first-time user can install it and get from start to end alone.

## What to run

- `pnpm app`: run Handline on your real home. The everyday app; it follows main and offers the restart.
- `tools/check.sh`: before a push, what CI runs. A change to the app's main, owner or follow also runs `tools/app-smoke.mjs` (a Mac, about a minute): the real app on a scratch home through a follow restart, a quit and a kill -9.
- `pnpm format`: rewrite formatting.

## Read next

- [docs/how-it-works.md](docs/how-it-works.md): the rules Handline follows, and why.
- [docs/setup.md](docs/setup.md): installing it, for developers.
- [docs/development.md](docs/development.md): working on Handline itself.

## License

Handline is free software under the [GNU Affero General Public License v3.0](LICENSE)
(AGPL-3.0-only). If you cannot meet its terms, a commercial license is available.

Contributions are accepted under the [contributor license agreement](CLA.md); see
[CONTRIBUTING.md](CONTRIBUTING.md).

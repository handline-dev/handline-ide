# Account assignment: scenario table

The founder: "the account usage needs a proper test: scenarios and timing issues must work for certain." This is the table of what must hold. Code under test: `go/accounts/usage` (Admit, NoRoom, Urgency, BestByUrgency, FiveHourShare, Cap), `go/scheduler/lifecycle` (FindRoom, RoomNow, pinnedWhy, wake reservations, move), `go/scheduler/staymove`.

## Rules for every test

- Time is a fake clock passed in (`Launcher.Now`, `Env.now`, `Situation` values). No `sleep`, no `time.Now()` in the code under test or the test.
- A scenario is: accounts and agents at the start (usage %, reset times, cap, pins, state), time that passes (a clock step, a tick), and the expected placement plus the reason text. The test states all three in its name or table row.
- A test fails on the placement and on the reason string where the table gives one.
- Existing tests worth extending, not duplicating: `launch_test.go` (`TestRoomIs...`, `TestAResumeWaits...`, `TestAColdWakeMoves...`, `TestAMoveGoesWhere...`), `pin_test.go`, `staymove_test.go`, `urgency_test.go`, `load_test.go`.
- `W` is the wall percent (`WallPct`), `cap` is `Load.Cap`. Boundary rows use W-1, W and W+1.

## A. Window boundaries (usage)

| # | Start | Time passes | Expected |
|---|---|---|---|
| A1 | alpha 5h at W-1, one agent costs more than 1 point | none | alpha has no room (stretch under `MinStretchS`); beta takes it |
| A2 | alpha 5h at W | none | alpha is at the wall, never picked, even with cap free |
| A3 | alpha 5h at W, reset in 60 s | +59 s, then +2 s | not picked at +59 s; picked at +2 s (past reset: reading is stale, the new window is empty) |
| A4 | alpha week at W, 5h at 0 | none | not picked (weekly wall); reason names the week and its reset |
| A5 | alpha week at W, subscription ends before the week reset | none | `WeekEndsIn` uses the earlier end; Urgency not ranked when the end passed |
| A6 | alpha reading older than its trust window | none | unknown, not "empty": not ranked by Urgency, may start only as the "nobody has run on it lately" case |
| A7 | alpha week 100%, reset in 10 min, work waiting on alpha | none / +11 min | staymove: waits for the reset when cache is warm and reset <= T; else moves. After +11 min it runs on alpha |
| A8 | cache life edge: turn ended `CacheLifeS - ResetIn - 1` s ago, then `+1` | none | stay at one second less, move at the boundary |
| A9 | `ColdStartPct` nil, then measured 0 | none | nil: default T (39 min); measured 0: T is 0, so it moves |
| A10 | account logged out (beta) | none | never picked, never in `mayStartAccounts`; a pinned agent says "not logged in" |
| A11 | account paused / month cycle ended | none | `NotFor` reason text exact; ends one second after `EndDate` |

## B. Cap and counts (the 7 / 2 screen)

| # | Start | Time passes | Expected |
|---|---|---|---|
| B1 | cap 2, 1 mid-turn | none | room; at 2 mid-turn: "at its cap: 2 of 2 agents mid-turn" |
| B2 | cap 2, 3 mid-turn (over) | none | no new start on it; the three keep running; reason says 3 of 2; no counter goes negative |
| B3 | **The founder's screen.** alpha cap 2 with 7 agents: 3 mid-turn, 4 working in the background by hook; gamma, omega, sigma cap 4 and theta cap 2 empty, logged in; delta week at W; beta logged out; 5 of alpha's agents pinned to alpha | none | `agents_running` and the cap use the same count (T8945): the line reads the mid-turn number, background ones shown apart; a new unpinned start goes to the emptiest of gamma, omega, sigma, theta (urgency band, then fewest mid-turn), never alpha, delta or beta; the 5 pinned ones wait for alpha |
| B4 | same as B3, then an alpha agent ends its turn | tick | one waiting pinned agent starts; count stays at or under cap |
| B5 | same as B3, an idle unpinned agent on alpha wakes | wake | it moves to a free account (T8948); a pinned one stays |
| B6 | same as B3 for 10 min with alpha over cap | +10 min, +10 min 1 s | at +10 min 1 s, one not-yet-started agent is moved or held with a reason; no running turn is stopped (T8949) |
| B7 | a hook-working session (turn over, background task running) | none | counted as working for the display, not as mid-turn for the cap, in both places the same way |
| B8 | cap lowered by pace (`cap_effective` < set) | tick | the lower number is the one shown and enforced; both counters read it |
| B9 | account with cap 0 | none | takes nothing, and the screen says so rather than "0 of 0" |

## C. Placement order

| # | Start | Expected |
|---|---|---|
| C1 | two accounts, weeks behind pace: one with urgency 5, one with 1 | the 5 one first |
| C2 | urgency within the band (`UrgencyBandAbs` / `Rel`) | the one with fewest mid-turn; then more urgent; then name |
| C3 | urgency equal to band edge +/- 0.01 | one side of the edge only, no flapping between ticks |
| C4 | no urgency known on any account | older key (headroom) decides; named in the reason |
| C5 | an idle empty account behind its pace (T8947) | it gets the waiting work before a busier account with more headroom |
| C6 | cold wake of a closed agent | moves only when another account is better by `RepickGain` (0.5) |
| C7 | person starts an agent by hand | goes where it was last, else most headroom, queues behind nobody |
| C8 | other vendor's account is empty | never used for this vendor |

## D. Pins

| # | Start | Expected |
|---|---|---|
| D1 | pinned to alpha, alpha full | waits; never placed elsewhere; reason names alpha |
| D2 | pinned to alpha, alpha logged out / paused / week at W | refused with the exact `pinnedWhy` text for each |
| D3 | pinned to an unmetered account | starts with no room test |
| D4 | pin removed while it waits | next tick places it by the pick |
| D5 | pin to an account of another vendor | refused, text names both vendors |
| D6 | the pin list read (T8946): agents pinned on accounts at or over cap | each listed once with its account; no write |

## E. Races and timing

| # | Start | Time passes | Expected |
|---|---|---|---|
| E1 | alpha has one free slot; two agents ask in the same tick | concurrent | exactly one is placed; the other waits or goes elsewhere; run with `-race` and repeat 200 times |
| E2 | two starts, two accounts each with one slot, both same urgency | concurrent | one each, never both on one |
| E3 | a wake reservation not answered | +`WakeReserveS`, +1 s | held until the limit, released one second after; the slot goes to the next waiter in order |
| E4 | an older closed waiter holds the slot; an open agent's move asks | none | the move does not take it (existing test, keep) |
| E5 | a start races a usage read that lands between Admit and placement | concurrent | placement uses the reading it admitted on, or re-admits; never places on an account that read as full |
| E6 | tick lands exactly on a reset second | tick | one answer for that second, same before and after in two runs |
| E7 | clock steps back (DST or manual) | -1 h | no negative durations; no early release of a reservation |

## F. Moves, switch and restart

| # | Start | Expected |
|---|---|---|
| F1 | manual switch while the agent is mid-turn (T8905) | scheduled, shown; runs when the turn ends; cancel removes it |
| F2 | "switch now" | turn stopped, handoff note written first, then moved; a note that never finishes does not hang the move |
| F3 | switch target full | refused with the room text; the schedule keeps its place or fails visibly |
| F4 | wall hit mid-turn with work waiting | staymove result (A7/A8), then the usual move |
| F5 | daemon restart with agents mid-turn on alpha | counts rebuilt from sessions, not from memory; no agent counted twice; scheduled switches and wake reservations survive or are named as dropped |
| F6 | restart during a handoff move | the agent ends on exactly one account, with its note |
| F7 | an account logs out while agents run on it | they are not killed; new starts avoid it; its count still shows |

## Builder test tasks

Each writes its rows as table-driven tests with the fake clock; none changes product code except a clock or hook seam it needs.

1. **Fake-clock seam**: one `Now` for `usage.Env`, `Launcher`, wake reservations and staymove inputs, so no test reads wall time; a lint grep that these packages have no `time.Now()` outside the seam. Others wait on it.
2. **A (A1-A11)** and **C (C1-C8)**: `go/accounts/usage` boundary and ranking tests.
3. **B (B1-B9)**: the cap and counts, including the full B3 reproduction as one fixture shared with T8945, T8946, T8948, T8949.
4. **D and F (D1-D6, F1-F4)**: pins and the manual switch; F1-F3 wait for T8905's daemon part.
5. **E (E1-E7)**: concurrency and reservations under `-race` with repeats.
6. **F5-F7**: restart and logout, using the daemon test harness.

T8945-T8949 each include the table rows named here for their area: T8945 B3/B7/B8, T8946 D6, T8947 C5, T8948 B5, T8949 B6.

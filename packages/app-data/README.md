# app-data

Everything the daemon answers lives in one React Query cache, and this package is the only door to it. An app imports its hooks
and never calls the client for data itself.

## What lives where

| State                                                                                        | Owner                                 |
| -------------------------------------------------------------------------------------------- | ------------------------------------- |
| Daemon answers: agents, tasks, inbox, chat, accounts, team, settings, worktrees and the rest | the React Query cache                 |
| Connection, who I am, `hello` methods, the error toast                                       | the `app-core` store                  |
| The terminal stream (snapshot, frames, resize)                                               | the store's terminal view, not cached |
| Layout, drafts, filters, scroll                                                              | `app-core` and the app's own storage  |

Every key carries the daemon's id (`qk`, `src/reads.ts`), so two daemons never share an entry.

## Reads

`useRead` (`src/read.ts`) is the one place a read becomes a query; each named hook is a line over it. A read runs only while
the connection is open and `hello.methods` lists the method. A read fed by an event topic is never stale (the bridge keeps it
current); a snapshot read is stale after 30 s. A list keeps its rows while a filter changes.

Errors are sorted once (`src/errors.ts`): `transport` shows nothing in the view (the connection banner speaks), `not_found` is
the empty state, `refused` shows the daemon's sentence and hint, an unknown method or gate `unsupported` is "not served",
`daemon_busy` waits and reads again, `protocol` is the app's fatal sentence, anything else is an error block with Retry over the
last data.

## Events

The event bridge (`src/bridge.ts`) turns each pushed event into a cache edit, batched in a microtask. An event is the daemon's
truth and wins over an optimistic value. A `gap` or a reconnect reads every active query again.

## Writes

Every UI write goes through `useWrite` (`src/write.ts`) and has a row in the write table (`src/writes.ts`) with its kind:

- **authored**: the person's own data (a message, a task, a memo, a setting). The cache is edited at once and put back if the
  daemon refuses; the person's text is kept.
- **decided**: what only the daemon decides (a session starting, a prompt answered, an approval). The cache is not touched; the
  view shows a pending mark until the event arrives, and the reason if it is refused.
- **uncached**: a stream write such as terminal input; the stream is the answer.

Writes carry a `request_id`, so a call re-sent after a reconnect is answered once; React Query never retries a write.

## Guards

`test/guards.test.ts` fails the build when an app calls the client for data by hand, when a write method of the contract is in
none of the write table, the not-from-the-UI list and the pairing list (`src/write-lists.ts`), or when a read has no key. The fake daemon's
`latency(ms)` and `refuse(method, gate, reason, hint)` test loading and rollback with real timing.

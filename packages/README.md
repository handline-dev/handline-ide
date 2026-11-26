# Packages

The web page (`apps/web`) and the desktop shell (`apps/electron`) share these. Packages are source only: `exports` points at
`.ts`, and Vite or Node compiles them; there is no build step per package.

| Package       | Holds                                                                                    |
| ------------- | ---------------------------------------------------------------------------------------- |
| `client`      | the daemon client: JSON-RPC, reconnect, cursors, terminal views, the WebSocket transport |
| `client-node` | the Unix socket transport (Node only: tests, the shell's main process, tools)            |
| `fake-daemon` | a daemon that speaks the contract with scripted agents, for UI work and tests            |
| `app-core`    | the store and pure screen decisions; no React, no DOM                                    |
| `app-react`   | the store's provider and hooks                                                           |
| `app-data`    | server state: React Query reads, the event bridge, the write table (its README)          |
| `app-ui`      | words and state names every app draws from; no React, no DOM                             |
| `tokens`      | colours, spacing, type scale and breakpoints as constants                                |
| `config`      | base tsconfigs and the lint rules                                                        |

Dependencies run one way: `contract` → `client` → `app-core` → `app-react` → `app-data` → `apps/*`. `client` and `tokens`
import no React and no app package; `app-core` imports no React; only `app-data` imports React Query; no package imports an
app; an app imports `fake-daemon` only in tests, dev scripts and `vite.config.ts`. Imports name a package (`@handline/app-core`) or a path in the same package
(`#src/store.ts`), never a relative path. `packages/config/eslint/boundaries.js` enforces these import rules.

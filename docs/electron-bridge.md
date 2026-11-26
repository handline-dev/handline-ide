# Electron bridge: the one API between the page and the shell

The desktop shell (`apps/electron`) shows the built page (`apps/web`) in one window. What crosses the window's edge is one
object, `window.handline`. Its calls and their results are typed in one file, `packages/app-core/src/shell.ts` (`ShellApi`),
which the preload and the page both import.

- One object, made by the preload with `contextBridge`. No other global, no generic `invoke(channel, ...)`, no Node in the
  page. The window sets `contextIsolation: true`, `sandbox: true`, `nodeIntegration: false`.
- Each call is one `ipcRenderer.invoke` on its own channel (`CHANNELS` in `apps/electron/src/bridge.ts`). Before it does
  anything, the shell checks where the call came from (the window's top frame, on the origin the shell serves) and what it
  was given (exact keys and types, bounded strings); anything else is `bad_request`.
- Every call returns a result and never throws: `{ ok: true, ... }` or `{ ok: false, code, message }` (`Fail<C>`). The page
  decides by `code`, never by parsing `message`, which is a sentence for the person.
- `window.handline?.api === 1` says the shell is there. A call is present only if this shell has it, so the page checks the
  call it is about to use (`typeof handline.compose === 'function'`), never a version. New calls only add; a change that
  breaks one makes `api` 2.
- `apps/web/src/shell.ts` holds the page's only reads of `window.handline`; views go through it. In a plain browser each
  call is absent and the page does without it.
- The page tells the shell what only it knows: which agent is on screen (`screen`, so no notice for it) and whether a
  restart would lose something (`unsent`). The shell tells the page where to go (`onNavigate`, from a notice's click or a
  `handline://` link) and sends its state as it changes (`onUpdate`, `onDaemon`, `onClusters`, `onToken`, `onFullScreen`);
  each listener gets the last value at once.

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

## The Web tab's page

A Web tab shows its page in a `<webview>` the window locks down (`apps/electron/src/preview.ts`): no preload, a sandbox, its
own session that grants no permission, http(s) only, no new windows. Nothing is ever put into the page. Two calls read it,
each naming the page by its id, which must be a page of the window that calls:

- `capturePreview(id)`: the page as drawn now, a PNG. The person marks it up and sends it to the agent's chat.
- `pickElement(id)`: one element, picked by the person's next click in the page (`apps/electron/src/pick.ts`). For the
  length of that pick the shell attaches the page's debugger and turns on the browser's own inspect mode: the highlight
  follows the pointer, and the click names an element without reaching the page. The shell then sends a fixed run of
  DevTools commands (the page never gets a way to send one) and answers with the picture and the element: its tag, id,
  classes, a CSS path, its text, the computed styles listed in `PICK_STYLES`, its outer HTML and its box in the picture's
  pixels, each cut to `PICK_LIMITS` in the shell (`packages/app-core/src/element-pick.ts`). One pick per page at a time.
  It ends with `cancelled` on Esc (`cancelPick(id)`, since the window's keys do not reach the page) or when the page's
  developer tools take the debugger, `timeout` after two minutes, and `unavailable` when the page navigates or closes;
  the debugger is detached on every one of these. A click inside a frame from another site answers `other_site`: such a frame cannot be
  picked from yet.

The viewer opens on the picked picture with a box already drawn on the element, and Send to chat sends the picture with
the element's details. What the page wrote is data, not instructions: the details go to the agent inside one fenced block
under a line that says so (`elementBlock`), and the box's own line names the element by plain names only, never by its text.

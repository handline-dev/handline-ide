// The two questions the tray and Cmd-Q ask, and the gate that makes every quit the person starts ask first. Electron's dialog is behind
// Ask, so a test answers it.

/** A message box: which button was pressed, and whether its checkbox was ticked. */
export type Ask = (o: {
  message: string;
  detail: string;
  buttons: string[];
  defaultId: number;
  cancelId: number;
  checkboxLabel?: string;
}) => Promise<{ response: number; checkboxChecked: boolean }>;

export const quitDetail = (working: number): string =>
  working
    ? `Handline closes. The daemon and ${working} working agent${working === 1 ? '' : 's'} keep running in the background. Open Handline again to see them.`
    : 'Handline closes. The daemon keeps running in the background. Open Handline again to see it.';

/** Quit Handline: asked only while the daemon runs and the person has not said not to; dontAsk is the ticked "Don't show again". */
export async function askQuit(o: {
  running: boolean;
  working: number;
  skip: boolean;
  ask: Ask;
}): Promise<{ quit: boolean; dontAsk: boolean }> {
  if (!o.running || o.skip) return { quit: true, dontAsk: false };
  const r = await o.ask({
    message: 'Quit Handline?',
    detail: quitDetail(o.working),
    buttons: ['Quit', 'Cancel'],
    defaultId: 0,
    cancelId: 1,
    checkboxLabel: "Don't show again",
  });
  return { quit: r.response === 0, dontAsk: r.response === 0 && r.checkboxChecked };
}

let quiet = false;

/** A quit the app makes itself (an update's relaunch, a changed setting, the Mac shutting down): it asks nothing. */
export function quitQuietly(app: { quit(): void }): void {
  quiet = true;
  app.quit();
}

/** Marks the next quit as one that asks nothing, without quitting (the Mac is shutting down: it quits the app itself). */
export function quietNext(): void {
  quiet = true;
}

/** Every other quit (Cmd-Q, the tray, the dock) asks first; while the question is open a second quit asks nothing more. */
export function guardQuit(
  app: { on(e: 'before-quit', f: (e: { preventDefault(): void }) => void): void; quit(): void },
  ask: () => Promise<boolean>,
): void {
  let asking = false;
  app.on('before-quit', (e) => {
    if (quiet) return;
    e.preventDefault();
    if (asking) return;
    asking = true;
    void ask()
      .catch(() => true)
      .then((ok) => {
        asking = false;
        if (ok) quitQuietly(app);
      });
  });
}

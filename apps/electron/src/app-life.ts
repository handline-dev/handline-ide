// When the app opens, how it quits, and what the tray counts, in Electron terms: the rules are in login.ts and ask.ts, which a test drives.
import { app, powerMonitor } from 'electron';
import type { DaemonClient } from '@handline/client';
import { askQuit, guardQuit, quietNext, quitQuietly } from '#src/ask.ts';
import { applyOpenAtLogin, openedAtLogin, workingOf } from '#src/login.ts';
import type { Follower } from '#src/follow.ts';
import type { SettingsFile } from '#src/mode.ts';
import type { TrayFollow, TrayUpdate } from '#src/tray.ts';
import type { UpdateFlow } from '#src/update-flow.ts';
import { electronAsk, type Own } from '#src/own.ts';

/** Hands Open at login to the system, and says whether this run was opened at login: then it keeps to the tray (no window, no dock icon)
 * until `shown` is called, when the person opens the window. */
export function loginLaunch(openAtLogin: boolean, background: boolean): { hidden: () => boolean; shown: () => void } {
  applyOpenAtLogin(app, app.isPackaged, openAtLogin);
  let hidden = openedAtLogin(app, app.isPackaged);
  if (hidden) app.setActivationPolicy('accessory');
  return {
    hidden: () => hidden,
    shown: () => {
      if (!hidden || background) return;
      hidden = false;
      app.setActivationPolicy('regular'); // the dock icon comes with the window
    },
  };
}

/** Quitting closes the app only: it asks first while the daemon runs (unless told not to, which a ticked Don't show again keeps), and a
 * quit the app makes itself, the Mac's shutdown and a SIGTERM among them, asks nothing. */
export function askBeforeQuit(own: Own, settings: SettingsFile): void {
  guardQuit(app, async () => {
    const r = await askQuit({
      running: own.running(),
      working: own.working() ?? 0,
      skip: settings.read().quietQuit === true,
      ask: electronAsk,
    });
    if (r.dontAsk) settings.write({ ...settings.read(), quietQuit: true });
    return r.quit;
  });
  powerMonitor.on('shutdown', quietNext);
  // SIGTERM is a program's or the system's quit, not the person's: Electron quits on it, and nobody is there to answer a question.
  process.on('SIGTERM', () => quitQuietly(app));
}

/** How many agents are mid-turn, every ten seconds, for the tray's first line and the two questions. */
export function countWorking(client: DaemonClient, own: Own): void {
  const look = () =>
    void client
      .call('agents.list', {})
      .then((r) => own.setWorking(workingOf(r as { agents?: { state?: string }[] })))
      .catch(() => own.setWorking(null));
  const every = setInterval(look, 10_000);
  app.on('quit', () => clearInterval(every));
}

/** The tray's Open at login: kept in the settings and handed to the system. */
export const saveOpenAtLogin =
  (settings: SettingsFile) =>
  (on: boolean): void => {
    settings.write({ ...settings.read(), openAtLogin: on });
    applyOpenAtLogin(app, app.isPackaged, on);
  };

/** What the tray reads of the update flow. */
export const trayUpdate = (flow: UpdateFlow): TrayUpdate => ({ phase: flow.view.phase });

/** Follow-main mode as the tray shows it. */
export const trayFollow = (f: Follower): TrayFollow => ({
  sha: f.sha,
  state: f.state,
  next: f.next?.sha ?? null,
  reason: f.reason,
  check: () => void f.look(),
  restart: () => void f.restartNow(),
});

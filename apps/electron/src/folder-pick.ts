// pickFolder (docs/electron-bridge.md §4.8): the system's folder chooser, for the first-run step that picks the folder to work in.
import type { BrowserWindow, Dialog } from 'electron';
import type { Fail } from '@handline/app-core';
import { CHANNELS, fromWindow, type Sender } from '#src/bridge.ts';

export interface FolderPickDeps {
  /** Shows the chooser for one folder (Electron's dialog.showOpenDialog with openDirectory). */
  choose(): Promise<{ canceled: boolean; filePaths: string[] }>;
}

/** The chosen folder's path, or null when the person cancels. */
export class FolderPick {
  private origin: string;
  private d: FolderPickDeps;
  constructor(origin: string, d: FolderPickDeps) {
    this.origin = origin;
    this.d = d;
  }
  async pick(e: Sender): Promise<{ ok: true; path: string | null } | Fail<'bad_request'>> {
    if (!fromWindow(e, this.origin)) return { ok: false, code: 'bad_request', message: 'not from the app window' };
    const r = await this.d.choose();
    return { ok: true, path: r.canceled ? null : (r.filePaths[0] ?? null) };
  }
}

/** Serves the channel on Electron's ipcMain. */
export function serveFolderPick(
  ipc: { handle(channel: string, f: (e: Sender, arg: unknown) => unknown): void },
  origin: string,
  deps: FolderPickDeps,
): void {
  const p = new FolderPick(origin, deps);
  ipc.handle(CHANNELS.pickFolder, (e) => p.pick(e));
}

/** The chooser as Electron shows it: one folder, a new one can be made there, over the window when it is open. */
export const chooseFolder = (dialog: Dialog, win: () => BrowserWindow | null): FolderPickDeps => ({
  choose: () => {
    const opts = { properties: ['openDirectory', 'createDirectory'] as ('openDirectory' | 'createDirectory')[] };
    const w = win();
    return w && !w.isDestroyed() ? dialog.showOpenDialog(w, opts) : dialog.showOpenDialog(opts);
  },
});

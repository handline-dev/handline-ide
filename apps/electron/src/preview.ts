// The web tab's pages: <webview> elements the app window draws among its own, so the app's menus and dialogs show over them.
// The window decides what each one gets: no preload, a sandbox, its own session that grants no permission, http(s) only, no new windows.
import { session, webContents, type BrowserWindow, type IpcMainInvokeEvent, type WebPreferences } from 'electron';
import type { Fail } from '@handline/app-core';
import { fromWindow } from '#src/bridge.ts';

const PARTITION = 'preview';
const bad = (message: string): Fail<'bad_request'> => ({ ok: false, code: 'bad_request', message });

/** What a page in a web tab may load: an http(s) address. */
export const showable = (src: string): boolean => {
  try {
    return /^https?:$/.test(new URL(src).protocol);
  } catch {
    return false;
  }
};

/** The preferences a web tab's page runs with, whatever the app window asked for. */
export function lockDown(prefs: WebPreferences): void {
  delete prefs.preload;
  prefs.nodeIntegration = false;
  prefs.nodeIntegrationInSubFrames = false;
  prefs.contextIsolation = true;
  prefs.sandbox = true;
  prefs.webSecurity = true;
  prefs.partition = PARTITION;
}

/** Sets the window's <webview> pages to the rules above. */
export function guardWebviews(w: BrowserWindow): void {
  // Its own session grants the page no permission (camera, microphone, location...): Electron grants every one by default.
  const own = session.fromPartition(PARTITION);
  own.setPermissionRequestHandler((_wc, _permission, grant) => grant(false));
  own.setPermissionCheckHandler(() => false);
  w.webContents.on('will-attach-webview', (e, prefs, params) => {
    if (!showable(params.src)) return e.preventDefault();
    lockDown(prefs);
    params.partition = PARTITION;
  });
  w.webContents.on('did-attach-webview', (_e, guest) => {
    // A link that wants a new window stays out: a tab is one page. And it stays on http(s) wherever it goes.
    guest.setWindowOpenHandler(() => ({ action: 'deny' }));
    const keep = (e: { url: string; preventDefault(): void }) => void (!showable(e.url) && e.preventDefault());
    guest.on('will-navigate', keep);
    guest.on('will-frame-navigate', keep);
    guest.on('will-redirect', keep);
  });
}

/** A web tab's page as drawn now, for the person to mark: the page is named by its id, and must be one this window holds. */
export async function capturePage(
  e: IpcMainInvokeEvent,
  origin: string,
  id: unknown,
): Promise<{ ok: true; png: string } | Fail<'bad_request' | 'unavailable'>> {
  if (!fromWindow(e, origin)) return bad('not from the app window');
  if (typeof id !== 'number') return bad('id is a number');
  const page = webContents.fromId(id);
  if (!page || page.hostWebContents !== e.sender) return bad('not a page of this window');
  const image = await page.capturePage();
  return image.isEmpty()
    ? { ok: false, code: 'unavailable', message: 'The page is blank.' }
    : { ok: true, png: image.toPNG().toString('base64') };
}

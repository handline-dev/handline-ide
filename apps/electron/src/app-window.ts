// The app window, and what keeps it on the app's own origin.
import os from 'node:os';
import path from 'node:path';
import { BrowserWindow, shell, type WebContents } from 'electron';
import { CHANNELS } from '#src/bridge.ts';
import { isDebugWindow } from '#src/debug-window.ts';
import { guardWebviews } from '#src/preview.ts';
import type { Frame } from '#src/window-place.ts';

/** The app window: the page gets window.handline and nothing else, and the window never leaves the loopback origin. */
export function createWindow(
  origin: string,
  onReady: () => void,
  frame: Frame | null,
  o: { icon: string; background: boolean },
): BrowserWindow {
  const w = new BrowserWindow({
    ...(frame ? { x: frame.x, y: frame.y } : {}),
    width: frame?.width ?? 1280,
    height: frame?.height ?? 860,
    title: 'Handline',
    icon: o.icon,
    show: false,
    // No OS title bar; the traffic lights sit over the sidebar (styles/base.css makes room).
    ...(os.platform() === 'darwin' ? { titleBarStyle: 'hiddenInset' as const, trafficLightPosition: { x: 14, y: 15 } } : {}),
    // The page gets window.handline from the preload and nothing else (docs/electron-bridge.md §2).
    webPreferences: {
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
      preload: path.join(import.meta.dirname, 'preload.cjs'),
      webviewTag: true, // the web tab's pages, each held to preview.ts's rules
      // A hidden window would be throttled; a background run's page must keep drawing for its checks and captures.
      backgroundThrottling: !o.background,
    },
  });
  guardWebviews(w);
  // The traffic lights are gone in full screen: the page drops the room it keeps for them (shell.ts).
  const tellFull = () => w.webContents.send(CHANNELS.fullScreen, w.isFullScreen());
  w.on('enter-full-screen', tellFull);
  w.on('leave-full-screen', tellFull);
  w.webContents.on('did-finish-load', tellFull);
  w.once('ready-to-show', onReady);
  if (frame?.maximized) w.maximize();
  if (frame?.fullScreen) w.setFullScreen(true);
  confine(w.webContents, origin);
  return w;
}

/**
 * The window shows the app and nothing else: a link goes to the browser (http/https only), and the window never leaves the loopback
 * origin. The one window the page may open is the debug window, on the same origin and with the same preferences, and it is
 * confined the same way.
 */
function confine(contents: WebContents, origin: string): void {
  contents.setWindowOpenHandler(({ url }) => {
    if (isDebugWindow(url, origin))
      return { action: 'allow', overrideBrowserWindowOptions: { width: 1280, height: 820, title: 'Terminals (debug)' } };
    if (/^https?:\/\//.test(url)) void shell.openExternal(url);
    return { action: 'deny' };
  });
  contents.on('did-create-window', (child) => confine(child.webContents, origin));
  contents.on('will-navigate', (e, url) => {
    if (new URL(url).origin !== origin) e.preventDefault();
  });
}

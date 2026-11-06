// The window's place in Electron terms: where window-state.ts keeps the frame and the route (in userData) and when it writes them.
import path from 'node:path';
import { app, screen, type BrowserWindow } from 'electron';
import { readFrame, readRoute, restorable, writeFrame, writeRoute, type Frame } from '#src/window-state.ts';

export type { Frame };
const file = (name: string) => path.join(app.getPath('userData'), name);

/** The frame the last window had, while it is still on a connected display. */
export const savedFrame = (): Frame | null =>
  restorable(
    readFrame(file('window.json')),
    screen.getAllDisplays().map((d) => d.bounds),
  );
/** The hash of the screen the last window showed, or ''. */
export const savedRoute = (): string => readRoute(file('route.txt'));

/** Keeps the window's frame (on move, resize, maximize and full screen, debounced; and once on close) and the page's route (on every in-page navigation). */
export function keepPlace(w: BrowserWindow): void {
  const save = () => {
    if (w.isDestroyed()) return;
    writeFrame(file('window.json'), { ...w.getNormalBounds(), maximized: w.isMaximized(), fullScreen: w.isFullScreen() });
  };
  let timer: NodeJS.Timeout | undefined;
  const later = () => {
    clearTimeout(timer);
    timer = setTimeout(save, 500);
  };
  for (const e of ['move', 'resize', 'maximize', 'unmaximize', 'enter-full-screen', 'leave-full-screen'] as const)
    (w as NodeJS.EventEmitter).on(e, later);
  w.on('close', () => (clearTimeout(timer), save()));
  w.webContents.on('did-navigate-in-page', (_e, url) => writeRoute(file('route.txt'), new URL(url).hash));
}

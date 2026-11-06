// What the window keeps across any restart (a quit, a crash, the follow-main relaunch): the screen it showed (route.txt, the page's
// hash) and its frame (window.json). The files are in userData; a missing or unreadable one is the default.
import fs from 'node:fs';
import path from 'node:path';

export type Frame = { x: number; y: number; width: number; height: number; maximized: boolean; fullScreen: boolean };
export type Rect = { x: number; y: number; width: number; height: number };

/** The saved hash, or '' when there is none or it is not a hash route. */
export function readRoute(file: string): string {
  try {
    const h = fs.readFileSync(file, 'utf8');
    return /^#\/\S*$/.test(h) && h.length < 2048 ? h : '';
  } catch {
    return '';
  }
}

export function writeRoute(file: string, hash: string): void {
  try {
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, hash);
  } catch {
    /* a window that cannot keep its place still works */
  }
}

const num = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);

export function readFrame(file: string): Frame | null {
  try {
    const f = JSON.parse(fs.readFileSync(file, 'utf8')) as Partial<Frame>;
    if (!num(f.x) || !num(f.y) || !num(f.width) || !num(f.height) || f.width < 200 || f.height < 200) return null;
    return { x: f.x, y: f.y, width: f.width, height: f.height, maximized: f.maximized === true, fullScreen: f.fullScreen === true };
  } catch {
    return null;
  }
}

export function writeFrame(file: string, f: Frame): void {
  try {
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, JSON.stringify(f));
  } catch {
    /* as writeRoute */
  }
}

/** The saved frame when enough of it is still on a connected display to grab and drag, else null (the default spot). */
export function restorable(f: Frame | null, displays: Rect[], minVisible = 100): Frame | null {
  if (!f) return null;
  const seen = displays.some((d) => {
    const w = Math.min(f.x + f.width, d.x + d.width) - Math.max(f.x, d.x);
    const h = Math.min(f.y + f.height, d.y + d.height) - Math.max(f.y, d.y);
    return w >= minVisible && h >= minVisible;
  });
  return seen ? f : null;
}

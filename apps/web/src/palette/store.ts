// The palette's two small stores: whether it is open, and what was on screen lately (kept per browser, a convenience of this viewer).
import { withRecent } from '@handline/app-core';
import { opened, shellHandle } from '#src/layout/kinds.tsx';
import { read, write } from '#src/persist/kv.ts';
import { createStore } from '#src/persist/store.ts';

const open = createStore(false);
export const openPalette = () => open.set(true);
export const closePalette = () => open.set(false);
export const togglePalette = () => open.set(!open.get());
export const usePaletteOpen = open.useValue;

const KEY = 'palette:recent';
const stored = read<unknown>(KEY, []);
const recent = createStore<string[]>(Array.isArray(stored) ? stored.filter((o): o is string => typeof o === 'string') : []);

/** Whether a tab is worth finding again: not an Empty tab, a plain shell (gone once closed), a text kept only in this browser's tab,
 * nor another cluster's (the palette reads this Mac's). */
const kept = (opens: string) =>
  !opens.startsWith('@') && !shellHandle(opens) && !opens.startsWith('newshell') && !opened(opens, 'text') && !opened(opens, 'msg');

/** What the focused tab shows now: the one recent thing there is no going back to. */
const current = createStore<string | undefined>(undefined);

/** Notes what the focused tab shows, newest first. */
export function noteRecent(opens: string | undefined) {
  current.set(opens);
  if (!opens || !kept(opens) || recent.get()[0] === opens) return;
  const next = withRecent(recent.get(), opens);
  write(KEY, next);
  recent.set(next);
}
/** What was on screen lately, newest first, without what the focused tab shows now. */
export function useRecent(): string[] {
  const list = recent.useValue();
  const now = current.useValue();
  return list.filter((o) => o !== now);
}

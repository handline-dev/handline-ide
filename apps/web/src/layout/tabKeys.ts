import { MAC, primaryMod } from '#src/ui/keys.ts';
// The keys that move between a pane's tabs, as browsers and editors have them. Pure: the key and the pane give the tab to show.
import { useEffect } from 'react';

type Key = Pick<KeyboardEvent, 'key' | 'code' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>;

/** The tab a key goes to, or null: next/previous round the ends, Cmd/Ctrl+1-8 by place, 9 the last. */
export function tabKeyTarget(e: Key, pane: { tabs: { id: string }[]; active: string }, mac = MAC): string | null {
  const i = tabIndex(
    e,
    pane.tabs.length,
    pane.tabs.findIndex((t) => t.id === pane.active),
    mac,
  );
  return i === null ? null : (pane.tabs[i]?.id ?? null);
}

function tabIndex(e: Key, count: number, at: number, mac: boolean): number | null {
  if (count < 1 || e.altKey) return null;
  const step = (by: number) => (at + by + count) % count;
  if (e.key === 'Tab' && e.ctrlKey && !e.metaKey) return step(e.shiftKey ? -1 : 1);
  if ((e.key === 'PageDown' || e.key === 'PageUp') && e.ctrlKey && !e.metaKey && !e.shiftKey) return step(e.key === 'PageDown' ? 1 : -1);
  if (mac && e.metaKey && e.shiftKey && !e.ctrlKey && (e.code === 'BracketRight' || e.code === 'BracketLeft'))
    return step(e.code === 'BracketRight' ? 1 : -1);
  const mod = primaryMod(e, mac);
  if (mod && !e.shiftKey && /^[1-9]$/.test(e.key)) {
    const n = Number(e.key);
    return n === 9 ? count - 1 : n <= count ? n - 1 : null;
  }
  return null;
}

/** Moves the pane's tab on a tab key, caught before a box or a terminal sees it; not behind a dialog. */
export function useTabKeys(
  getPane: () => { id: string; tabs: { id: string }[]; active: string },
  select: (pane: string, tab: string) => void,
) {
  useEffect(() => {
    const step = (e: KeyboardEvent) => {
      const p = getPane();
      const tab = tabKeyTarget(e, p);
      if (!tab || document.querySelector('[role="dialog"], [role="alertdialog"]')) return;
      e.preventDefault();
      e.stopPropagation();
      select(p.id, tab);
    };
    addEventListener('keydown', step, true);
    return () => removeEventListener('keydown', step, true);
  });
}

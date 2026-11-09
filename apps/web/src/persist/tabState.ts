// A tab's way of showing (a filter, a fold, what it compares against) is held in memory under `tab/<tab id>/<name>`: a pane draws only
// its current tab, so plain state is lost on leaving it; a reload starts it fresh. Dropped when no layout has the tab any more.
import { createContext, useContext, useState } from 'react';
import { names, write } from '#src/persist/kv.ts';
import { panes, type Node } from '#src/layout/tree.ts';

/** The id of the tab a view is drawn in (Workspace); none outside a tab, as in a popover or Settings. */
export const TabId = createContext<string | null>(null);

const held = new Map<string, unknown>();
/** How many tab values are held: for a leak check. */
export const heldCount = () => held.size;

/** useState held with the tab it is drawn in, read when the view is drawn again; outside a tab, plain useState. */
export function useTabState<T>(name: string, fallback: T) {
  const tab = useContext(TabId);
  const key = tab && `tab/${tab}/${name}`;
  const [value, setValue] = useState<T>(() => (key && held.has(key) ? (held.get(key) as T) : fallback));
  const set = (next: T | ((v: T) => T)) =>
    setValue((v) => {
      const n = typeof next === 'function' ? (next as (v: T) => T)(v) : next;
      if (key) held.set(key, n);
      return n;
    });
  return [value, set] as const;
}

/** The names that belong to a tab not among `tabs`. */
export function strayTabNames(kept: string[], tabs: ReadonlySet<string>): string[] {
  return kept.filter((n) => {
    const id = /^tab\/([^/]+)\//.exec(n)?.[1];
    return id !== undefined && !tabs.has(id);
  });
}

/** Every tab id in `layouts`, sorted, as one string: what a sweep depends on, so one runs only when it changes. */
export const tabIds = (layouts: Node[]) =>
  layouts
    .flatMap((l) => panes(l).flatMap((p) => p.tabs.map((t) => t.id)))
    .sort()
    .join(' ');

/** What a Markdown or Web tab keeps in storage (persist/kv.ts), left there as on main: Back can bring a closed tab back with it. Any
 * other tab/ name in storage is a view value an earlier build of this kept there. */
const KEPT = /^tab\/[^/]+\/(path|mode|url)$/;

/** Drops what is held for tabs not among `tabs`, and the view values an earlier build left in storage. */
export function sweepTabState(tabs: ReadonlySet<string>) {
  for (const k of strayTabNames([...held.keys()], tabs)) held.delete(k);
  for (const n of names('tab/').filter((n) => !KEPT.test(n))) write(n, undefined);
}

// A tab's way of showing (a filter, a fold, what it compares against) is kept under `tab/<tab id>/<name>`: a pane draws only its current
// tab, so plain state is lost on leaving it. Swept when no layout has the tab any more (sweepTabState).
import { createContext, useContext } from 'react';
import { names, usePersisted, write } from '#src/persist/kv.ts';
import { panes, type Node } from '#src/layout/tree.ts';

/** The id of the tab a view is drawn in (Workspace); none outside a tab, as in a popover or Settings. */
export const TabId = createContext<string | null>(null);

/** useState kept with the tab it is drawn in; outside a tab, plain useState. Values must be JSON. */
export function useTabState<T>(name: string, fallback: T) {
  const tab = useContext(TabId);
  return usePersisted<T>(tab ? `tab/${tab}/${name}` : null, fallback);
}

/** The kept names that belong to a tab not among `tabs`. */
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

/** Drops what is kept for tabs not among `tabs`. */
export function sweepTabState(tabs: ReadonlySet<string>) {
  for (const n of strayTabNames(names('tab/'), tabs)) write(n, undefined);
}

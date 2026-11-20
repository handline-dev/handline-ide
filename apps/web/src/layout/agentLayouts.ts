import { useEffect, useRef, useState } from 'react';
import { read, write } from '#src/persist/kv.ts';
import { sweepTabState, tabIds } from '#src/persist/tabState.ts';
import { canonical, carry, labelOf } from '#src/layout/kinds.tsx';
import {
  CLOSED_KEPT,
  closedBy,
  initial,
  putTabs,
  reduce,
  reopen,
  takeTabs,
  type Action,
  type Closed,
  type Node,
  type Target,
} from '#src/layout/tree.ts';

/** Each agent, and each page (Tasks, Requests, Usage) under `page/<kind>`, has its own workspace layout:
 * picking one in the sidebar brings back its panes and tabs. Kept per browser (persist/kv.ts), so a reload keeps them. */
const KEY = 'layouts';
const PAGE = 'page/';
/** A page's workspace key. */
export const pageSpace = (kind: string) => PAGE + kind;
/** `closed`: each workspace's closes, newest last, for Cmd+Shift+T. */
type Saved = { agent: string | null; layouts: Record<string, Node>; closed?: Record<string, Closed[]> };

/** `s` with a layout for its picked workspace, made when there is none yet: a page's starts on its own tab, fixed. */
const ensured = (s: Saved): Saved => {
  const k = s.agent ?? '';
  if (s.layouts[k]) return s;
  const page = k.startsWith(PAGE) ? k.slice(PAGE.length) : null;
  return { ...s, layouts: { ...s.layouts, [k]: page ? initial(page, labelOf(page)) : initial() } };
};

/** The Notifications page (inbox) became Requests: a saved tab or page of it opens Requests. An agent's Settings tab, which drew
 * nothing, is now Settings › Agents › that agent: a saved one becomes an empty tab. */
const renamed = (n: Node): Node =>
  n.kind === 'split'
    ? { ...n, a: renamed(n.a), b: renamed(n.b) }
    : {
        ...n,
        tabs: n.tabs.map((t) =>
          t.opens === 'inbox' ? { ...t, opens: 'requests', title: 'Requests' } : t.opens === 'settings' ? { id: t.id, title: '' } : t,
        ),
      };
const upgraded = (s: Saved): Saved => {
  const { [`${PAGE}inbox`]: _gone, ...layouts } = s.layouts;
  return {
    ...s,
    agent: s.agent === `${PAGE}inbox` ? `${PAGE}requests` : s.agent,
    layouts: Object.fromEntries(Object.entries(layouts).map(([k, n]) => [k, renamed(n)])),
  };
};

const load = (): Saved => ensured(upgraded(read<Saved>(KEY, { agent: null, layouts: {} })));

/** Layout changes that move the person somewhere: each is a step that Back and Forward walk. Closing, splitting, moving and resizing are not. */
const MOVES = new Set<Action['type']>(['select', 'open', 'show', 'choose']);

export function useAgentLayouts() {
  const [saved, setSaved] = useState(load);
  // Navigation is the browser's history: a move pushes the whole layout as an entry; any other change rewrites the entry it is on,
  // so Back returns to where the person was with what was open then. Restoring an entry neither pushes nor rewrites.
  const step = useRef<'push' | 'replace' | 'restore'>('replace');
  const last = useRef<string | null>(null);
  const swept = useRef<string | null>(null);
  useEffect(() => {
    write(KEY, saved);
    // A tab no layout has any more takes what it kept with it (persist/tabState); the first run sweeps what earlier ones left.
    const tabs = tabIds(Object.values(saved.layouts));
    if (tabs !== swept.current) sweepTabState(new Set(tabs.split(' ')));
    swept.current = tabs;
    // The closed list is not a place: Back and Forward leave it as it is now.
    const nav = { agent: saved.agent, layouts: saved.layouts };
    const json = JSON.stringify(nav);
    const how = step.current;
    step.current = 'replace';
    if (how === 'restore' || json === last.current) return void (last.current = json);
    last.current = json;
    if (how === 'push') history.pushState({ nav }, '');
    else history.replaceState({ ...(history.state as object | null), nav }, ''); // keeps an open modal's mark (ui/backCloses)
  }, [saved]);
  useEffect(() => {
    const back = (e: PopStateEvent) => {
      const nav = (e.state as { nav?: Saved } | null)?.nav;
      if (!nav) return;
      step.current = 'restore';
      setSaved((s) => ({ ...nav, closed: s.closed }));
    };
    addEventListener('popstate', back);
    return () => removeEventListener('popstate', back);
  }, []);

  // Before any agent is picked the workspace still works, under no agent ('').
  const key = saved.agent ?? '';
  const layout = saved.layouts[key];
  // Picking another workspace brings it back as it was left; picking the one shown again brings up its main tab.
  const open = (space: string, opens: string, title: string) => {
    step.current = 'push';
    setSaved((s) => {
      const e = ensured({ ...s, agent: space });
      if ((s.agent ?? '') !== space) return e;
      return { ...e, layouts: { ...e.layouts, [space]: reduce(e.layouts[space], { type: 'show', opens, title }) } };
    });
  };
  const page = key.startsWith(PAGE) ? key.slice(PAGE.length) : null;
  return {
    agent: page === null ? saved.agent : null,
    page,
    layout,
    select: (agent: string) => open(agent, 'chat', 'Chat'),
    selectPage: (kind: string, title: string) => open(PAGE + kind, kind, title),
    dispatch: (action: Action) => {
      if (MOVES.has(action.type)) step.current = 'push';
      setSaved((s) => {
        const k = s.agent ?? '';
        const after = reduce(s.layouts[k], action);
        const gone = closedBy(s.layouts[k], after, action);
        const closed = gone ? { ...s.closed, [k]: [...(s.closed?.[k] ?? []), gone].slice(-CLOSED_KEPT) } : s.closed;
        return { ...s, closed, layouts: { ...s.layouts, [k]: after } };
      });
    },
    /** The workspace on screen: an agent's key, or `page/<kind>`. */
    space: key,
    /** Moves tabs of pane `pane` in workspace `from` to workspace `to` (at `at`, else the end of its first pane) and goes there.
     *  Pinned tabs stay; one already open there comes up instead. */
    moveTabs: (from: string, pane: string, tabs: string[], shown: string, to: string, at?: Target) => {
      if (from === to) return;
      step.current = 'push';
      setSaved((s) => {
        const src = s.layouts[from];
        if (!src) return s;
        const taken = takeTabs(src, pane, tabs);
        if (!taken.tabs.length) return s;
        const e = ensured({ ...s, agent: to });
        // Another agent's chat is that agent's own Chat in its workspace.
        const agent = to.startsWith(PAGE) ? null : to;
        // A tab of the agent's own (its terminal, its changes) keeps naming it in another workspace.
        const owner = from.startsWith(PAGE) ? null : from;
        const moved = taken.tabs.map((t) => ({ ...t, opens: canonical(carry(t.opens!, owner), agent) }));
        return { ...e, layouts: { ...e.layouts, [from]: taken.root, [to]: putTabs(e.layouts[to], moved, shown, at) } };
      });
    },
    /** Opens the workspace's last close again; `pane` takes the tabs whose own pane is gone. */
    reopenClosed: (pane: string) =>
      setSaved((s) => {
        const k = s.agent ?? '';
        const list = s.closed?.[k] ?? [];
        const last = list.at(-1);
        if (!last) return s;
        return { ...s, closed: { ...s.closed, [k]: list.slice(0, -1) }, layouts: { ...s.layouts, [k]: reopen(s.layouts[k], last, pane) } };
      }),
  };
}

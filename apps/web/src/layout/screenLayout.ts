import { useEffect, useRef, useState } from 'react';
import { read, write } from '#src/persist/kv.ts';
import { sweepTabState, tabIds } from '#src/persist/tabState.ts';
import { carry, chatOf, ownedBy, pages } from '#src/layout/kinds.tsx';
import { spaceOf, splitSpace } from '#src/connection/clusters.tsx';
import {
  CLOSED_KEPT,
  closedBy,
  findPane,
  dropTabs,
  initial,
  panes,
  reduce,
  reopen,
  type Action,
  type Closed,
  type Node,
  type Pane,
} from '#src/layout/tree.ts';

/** The one layout on screen, as in a browser window: a sidebar pick goes to the focused pane's shown tab, the panes stay.
 *  Kept per browser (persist/kv.ts), so a reload keeps it. `focus`: the pane last clicked or typed in. */
const KEY = 'screen';
/** `shut`: tabs closed lately, which a Back step leaves out. */
type Saved = { layout: Node; focus?: string; closed?: Closed[]; shut?: string[] };
const SHUT_KEPT = 200;
const tabsOf = (n: Node) => panes(n).flatMap((p) => p.tabs.map((t) => t.id));

/** Before T9453 each agent and page had a layout of its own (`layouts`, picked by `agent`). The one on screen then is kept, its
 *  tabs naming their agent (its Chat is `chat:<agent>`), and the others are let go. Read only when there is no `screen` yet, so the
 *  old key stays as it was. */
type Old = { agent: string | null; layouts: Record<string, Node> };
const named = (n: Node, agent: string | null): Node =>
  n.kind === 'split'
    ? { ...n, a: named(n.a, agent), b: named(n.b, agent) }
    : {
        ...n,
        tabs: n.tabs.map(({ closable: _pin, ...t }) => {
          if (!t.opens || !agent) return t;
          const { cluster, agent: name } = splitSpace(agent);
          if (t.opens === 'chat') return { ...t, opens: spaceOf(cluster, chatOf(name)) };
          return { ...t, opens: carry(t.opens, agent) };
        }),
      };
function fromOld(): Saved {
  const old = read<Old | null>('layouts', null);
  const key = old?.agent ?? '';
  const n = old?.layouts[key];
  if (!n) return { layout: initial() };
  return { layout: named(n, key.startsWith('page/') ? null : key) };
}
const load = (): Saved => read<Saved | null>(KEY, null) ?? fromOld();

/** The focused pane, else the first. */
const focused = (s: Saved): Pane => (s.focus && findPane(s.layout, s.focus)) || panes(s.layout)[0];

/** The agent a tab shows (its chat, its terminal, its changes), with its cluster, or null. */
export function agentOf(opens: string | undefined): string | null {
  const { cluster, agent: inner } = splitSpace(opens ?? '');
  const name = inner.startsWith('chat:') ? inner.slice(5) : (ownedBy(inner)?.agent ?? null);
  return name ? spaceOf(cluster, name) : null;
}
const PAGES = new Set(pages.map((p) => p.kind));

/** Layout changes that move the person somewhere: each is a step that Back and Forward walk. Closing, splitting, moving and resizing are not. */
const MOVES = new Set<Action['type']>(['select', 'open', 'show', 'choose', 'navigate']);

export function useScreenLayout() {
  const [saved, setSaved] = useState(load);
  // Navigation is the browser's history: a move pushes the layout as an entry; any other change rewrites the entry it is on, so
  // Back returns to where the person was with what was open then. Restoring an entry neither pushes nor rewrites.
  const step = useRef<'push' | 'replace' | 'restore'>('replace');
  const last = useRef<string | null>(null);
  const swept = useRef<string | null>(null);
  useEffect(() => {
    write(KEY, saved);
    // A tab the layout has no more takes what it kept with it (persist/tabState); the first run sweeps what earlier ones left.
    const tabs = tabIds([saved.layout]);
    if (tabs !== swept.current) sweepTabState(new Set(tabs.split(' ')));
    swept.current = tabs;
    // The closed list and the focus are not a place: Back and Forward leave them as they are now.
    const json = JSON.stringify(saved.layout);
    const how = step.current;
    step.current = 'replace';
    if (how === 'restore' || json === last.current) return void (last.current = json);
    last.current = json;
    const nav = { layout: saved.layout };
    if (how === 'push') history.pushState({ nav }, '');
    else history.replaceState({ ...(history.state as object | null), nav }, ''); // keeps an open modal's mark (ui/backCloses)
  }, [saved]);
  useEffect(() => {
    const back = (e: PopStateEvent) => {
      const nav = (e.state as { nav?: { layout?: Node } } | null)?.nav;
      if (!nav?.layout) return;
      step.current = 'restore';
      const layout = nav.layout;
      setSaved((s) => ({ ...s, layout: dropTabs(layout, new Set(s.shut)) }));
    };
    addEventListener('popstate', back);
    return () => removeEventListener('popstate', back);
  }, []);

  const apply = (action: Action) => {
    if (MOVES.has(action.type)) step.current = 'push';
    setSaved((s) => {
      const after = reduce(s.layout, action);
      const gone = closedBy(s.layout, after, action);
      const closed = gone ? [...(s.closed ?? []), gone].slice(-CLOSED_KEPT) : s.closed;
      // A closed tab's state and terminal are gone: Back must not bring it back as an empty shell.
      const left =
        action.type === 'closeTab' || action.type === 'closePane' ? tabsOf(s.layout).filter((id) => !tabsOf(after).includes(id)) : [];
      return { ...s, closed, layout: after, shut: [...(s.shut ?? []), ...left].slice(-SHUT_KEPT) };
    });
  };
  /** What a pane already shows takes the focus there (its tab comes up); else the focused pane's shown tab goes to `opens`, as a
   *  browser tab follows a link. */
  const go = (opens: string, title: string) => {
    step.current = 'push';
    setSaved((s) => {
      const all = panes(s.layout);
      const at =
        all.find((p) => p.tabs.find((t) => t.id === p.active)?.opens === opens) ?? all.find((p) => p.tabs.some((t) => t.opens === opens));
      if (!at) return { ...s, layout: reduce(s.layout, { type: 'navigate', pane: focused(s).id, opens, title }) };
      const tab = at.tabs.find((t) => t.opens === opens)!;
      return { ...s, focus: at.id, layout: reduce(s.layout, { type: 'select', pane: at.id, tab: tab.id }) };
    });
  };

  const pane = focused(saved);
  const shown = pane.tabs.find((t) => t.id === pane.active)?.opens;
  const agent = agentOf(shown);
  return {
    /** The agent the focused tab shows, or null. */
    agent,
    /** The page (Tasks, Requests, Usage) the focused tab is, or null. */
    page: shown && PAGES.has(shown) ? shown : null,
    /** What every pane's shown tab is (an agent's key or a page's kind): the sidebar marks each. */
    onScreen: panes(saved.layout)
      .map((p) => {
        const o = p.tabs.find((t) => t.id === p.active)?.opens;
        return agentOf(o) ?? (o && PAGES.has(o) ? o : null);
      })
      .filter((x): x is string => !!x),
    layout: saved.layout,
    focus: pane.id,
    setFocus: (id: string) => setSaved((s) => (s.focus === id ? s : { ...s, focus: id })),
    select: (key: string) => {
      const { cluster, agent: name } = splitSpace(key);
      go(spaceOf(cluster, chatOf(name)), 'Chat');
    },
    selectPage: (kind: string, title: string) => go(kind, title),
    dispatch: apply,
    /** Opens the last close again; `pane` takes the tabs whose own pane is gone. */
    reopenClosed: (into: string) =>
      setSaved((s) => {
        const list = s.closed ?? [];
        const c = list.at(-1);
        if (!c) return s;
        return { ...s, closed: list.slice(0, -1), layout: reopen(s.layout, c, into) };
      }),
  };
}

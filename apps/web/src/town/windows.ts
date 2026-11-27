// The town's windows as data: tab bars docked down one side of the town at a third, a half or two thirds of its width, and tab bars
// floating over it. Each is a layout tree of the app's own (layout/tree.ts), so a tab bar here takes tabs, links and splits as one on the
// main screen does; this file only moves tabs between the trees and drops a tree that has nothing open left. Pure, for the tests.
import {
  STOPS,
  dropTabs,
  findPane,
  holds,
  initial,
  newId,
  panes,
  placeTabs,
  reduce,
  type Action,
  type Node,
  type Pane,
  type Tab,
  type Target,
} from '#src/layout/tree.ts';

export type Side = 'left' | 'right';
/** A floating tab bar: its top left as a share of the town's width and height. */
export type Float = { id: string; root: Node; x: number; y: number };
export type Windows = { side: Side; stop: number; dock: Node | null; floats: Float[] };

/** What goes somewhere: tabs taken from a pane, or a thing to open (a link dragged in, a person clicked). */
export type Moving = { from: string; tabs: string[]; shown: string } | { opens: string; title: string };

export type WindowAction =
  /** A tab bar's own change, made in the tree that holds the pane it names; a move into another tree goes across. */
  | { type: 'tree'; action: Action }
  /** Brings up the tab that opens this wherever it is, else opens it in `pane` while a tab bar holds it, else in a new tab bar floating
   *  at `at`, else in the dock. */
  | { type: 'show'; opens: string; title: string; pane?: string; at?: { x: number; y: number } }
  | { type: 'dock'; what: Moving; side: Side }
  | { type: 'float'; what: Moving; x: number; y: number }
  | { type: 'stop'; stop: number }
  | { type: 'closeDock' }
  /** Puts a floating tab bar on top of the others. */
  | { type: 'raise'; float: string }
  | { type: 'moveFloat'; float: string; x: number; y: number };

export const NONE: Windows = { side: 'right', stop: STOPS[0], dock: null, floats: [] };

type Trees = { dock: Node | null; floats: Float[] };
const trees = (w: Trees): Node[] => [...(w.dock ? [w.dock] : []), ...w.floats.map((f) => f.root)];
const opened = (n: Node) => panes(n).some((p) => p.tabs.some((t) => t.opens));

/** `w` with the tree that holds node `id` replaced by f(tree). */
function onTree<W extends Trees>(w: W, id: string, f: (n: Node) => Node): W {
  if (w.dock && holds(w.dock, id)) return { ...w, dock: f(w.dock) };
  return { ...w, floats: w.floats.map((x) => (holds(x.root, id) ? { ...x, root: f(x.root) } : x)) };
}

/** A tree with nothing open left goes: a dock or a float is there only for what it shows. */
function prune(w: Windows): Windows {
  const dock = w.dock && opened(w.dock) ? w.dock : null;
  const floats = w.floats.filter((f) => opened(f.root));
  return dock === w.dock && floats.length === w.floats.length ? w : { ...w, dock, floats };
}

/** The node an action names, to find the tree it is in. */
function named(a: Action): string | null {
  if (a.type === 'move') return a.from;
  if (a.type === 'resize') return a.split;
  if (a.type === 'open') return a.to.pane;
  if (a.type === 'show') return null;
  return a.pane;
}

/** `w` without the tabs `what` takes, and those tabs as a pane: the source pane itself (its id kept) when they are all of it. */
function take(w: Windows, what: Moving): [Windows, Pane] | null {
  if ('opens' in what) {
    // Open already: that tab goes, so nothing is open twice.
    const at = trees(w)
      .flatMap(panes)
      .find((p) => p.tabs.some((t) => t.opens === what.opens));
    if (!at) return [w, initial(what.opens, what.title) as Pane];
    const t = at.tabs.find((x) => x.opens === what.opens)!;
    return take(w, { from: at.id, tabs: [t.id], shown: t.id });
  }
  const src = trees(w)
    .map((n) => findPane(n, what.from))
    .find(Boolean);
  const group: Tab[] = src?.tabs.filter((t) => what.tabs.includes(t.id)) ?? [];
  if (!src || !group.length) return null;
  const active = group.some((t) => t.id === what.shown) ? what.shown : group[0].id;
  const ids = new Set(group.map((t) => t.id));
  const rest = onTree(w, src.id, (n) => dropTabs(n, ids));
  const whole = group.length === src.tabs.length;
  return [rest, { kind: 'pane', id: whole ? src.id : newId(), tabs: group, active }];
}

/** The last pane of a tree: a pane docked in beside the others goes under it. */
const last = (n: Node) => panes(n)[panes(n).length - 1];

export function reduceWindows(w: Windows, a: WindowAction): Windows {
  return prune(change(w, a));
}

function change(w: Windows, a: WindowAction): Windows {
  switch (a.type) {
    case 'tree': {
      const act = a.action;
      // Nothing is open twice in the town: what a tab bar asks to show or open comes up wherever it is open already.
      if (act.type === 'show' || (act.type === 'open' && isOpenIn(w, act.opens)))
        return change(w, { type: 'show', opens: act.opens, title: act.title });
      if (act.type === 'move') {
        const from = trees(w).find((n) => holds(n, act.from));
        if (from && !holds(from, act.to.pane)) return across(w, act.from, act.tabs ?? [act.tab], act.tab, act.to);
      }
      const id = named(act);
      return id ? onTree(w, id, (n) => reduce(n, act)) : w;
    }
    case 'show': {
      const at = trees(w).find((n) => panes(n).some((p) => p.tabs.some((t) => t.opens === a.opens)));
      if (at) return onTree(w, at.id, (n) => reduce(n, { type: 'show', opens: a.opens, title: a.title }));
      if (a.pane && trees(w).some((n) => findPane(n, a.pane!)))
        return onTree(w, a.pane, (n) => reduce(n, { type: 'open', opens: a.opens, title: a.title, to: { pane: a.pane! } }));
      if (a.at) return { ...w, floats: [...w.floats, { id: newId(), root: initial(a.opens, a.title), ...a.at }] };
      if (!w.dock) return { ...w, dock: initial(a.opens, a.title) };
      return { ...w, dock: reduce(w.dock, { type: 'open', opens: a.opens, title: a.title, to: { pane: panes(w.dock)[0].id } }) };
    }
    case 'dock': {
      const got = take(w, a.what);
      if (!got) return w;
      const [rest, pane] = got;
      const dock =
        rest.dock && opened(rest.dock) ? placeTabs(rest.dock, pane.tabs, pane.active, { pane: last(rest.dock).id, edge: 'down' }) : pane;
      return { ...prune(rest), side: a.side, dock };
    }
    case 'float': {
      const got = take(w, a.what);
      if (!got) return w;
      const rest = prune(got[0]);
      return { ...rest, floats: [...rest.floats, { id: newId(), root: got[1], x: a.x, y: a.y }] };
    }
    case 'stop':
      return { ...w, stop: a.stop };
    case 'closeDock':
      return { ...w, dock: null };
    case 'moveFloat':
      return { ...w, floats: w.floats.map((f) => (f.id === a.float ? { ...f, x: a.x, y: a.y } : f)) };
    case 'raise': {
      const f = w.floats.find((x) => x.id === a.float);
      return !f || w.floats[w.floats.length - 1] === f ? w : { ...w, floats: [...w.floats.filter((x) => x !== f), f] };
    }
  }
}

/** Tabs moved from a pane of one tree to a pane of another. */
function across(w: Windows, from: string, tabs: string[], shown: string, to: Target): Windows {
  // A target that is gone (a tab bar still fading out) takes nothing, so nothing is lost on the way.
  if (!trees(w).some((n) => holds(n, to.pane))) return w;
  const got = take(w, { from, tabs, shown });
  if (!got) return w;
  const [rest, pane] = got;
  return onTree(rest, to.pane, (n) => placeTabs(n, pane.tabs, pane.active, to));
}

/** The windows on a phone's width: every floating tab bar's tabs join the dock's first tab bar (the dock is then a sheet under the
 *  picture), so nothing covers the picture and the sheet is never split into slivers. */
export function sheetOf(w: Windows): Windows {
  return w.floats
    .flatMap((f) => panes(f.root))
    .reduce((v, p) => {
      const what = { from: p.id, tabs: p.tabs.map((t) => t.id), shown: p.active };
      if (!v.dock) return reduceWindows(v, { type: 'dock', what, side: v.side });
      const got = take(v, what);
      if (!got?.[0].dock) return v;
      const [rest, pane] = got;
      return prune({ ...rest, dock: placeTabs(rest.dock!, pane.tabs, pane.active, { pane: panes(rest.dock!)[0].id }) });
    }, w);
}

/** Where a new floating tab bar opens, as shares of the town: at the top of the side away from `clickedX` (the share of the width last
 *  pressed at), so it does not cover who was clicked; each one more (`nth`) a little down and toward the middle. `wide`: the tab bar's
 *  width as a share of the town's. */
export function spotAway(clickedX: number, wide: number, nth: number): { x: number; y: number } {
  const step = 0.03 * (nth % 6);
  const room = Math.max(0, 1 - wide);
  const x = clickedX > 0.5 ? Math.min(room, 0.02 + step) : Math.max(0, room - 0.02 - step);
  return { x, y: 0.06 + step };
}

/** The keys of the agents whose chat a tab bar shows now. */
export function chatsShown(w: Windows): Set<string> {
  const keys = trees(w)
    .flatMap(panes)
    .map((p) => p.tabs.find((t) => t.id === p.active)?.opens ?? '')
    .filter((o) => o.startsWith('chat:'))
    .map((o) => o.slice(5));
  return new Set(keys);
}

const isOpenIn = (w: Windows, opens: string) =>
  trees(w)
    .flatMap(panes)
    .some((p) => p.tabs.some((t) => t.opens === opens));

/** The pane that holds the keys: `focus` while it is there, else the dock's first, else the top float's. */
export function focusIn(w: Windows, focus: string): string {
  const all = trees(w).flatMap(panes);
  if (all.some((p) => p.id === focus)) return focus;
  const top = w.floats[w.floats.length - 1];
  return (w.dock ? panes(w.dock)[0] : top ? panes(top.root)[0] : undefined)?.id ?? '';
}

/** The pane showing `opens`, if any. */
export const paneShowing = (w: Windows, opens: string) =>
  trees(w)
    .flatMap(panes)
    .find((p) => p.tabs.some((t) => t.id === p.active && t.opens === opens));

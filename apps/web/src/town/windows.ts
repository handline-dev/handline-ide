// The town's windows as data: tab bars docked down the town's left side, its right side or both, each at a third, a half or two thirds
// of its width, and tab bars floating over it, each the town's full height. Each is a layout tree of the app's own (layout/tree.ts), so a tab bar here takes tabs, links and splits as one on the
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
export const SIDES: Side[] = ['left', 'right'];
/** A floating tab bar, the town's full height: its left edge as a share of the town's width, and its width in px. */
export type Float = { id: string; root: Node; x: number; w: number };
/** `docks`: the tab bars docked on each side; `stops`: each side's share of the town's width. */
export type Windows = { docks: Record<Side, Node | null>; stops: Record<Side, number>; floats: Float[] };

/** A floating tab bar's width when it opens, and the least it is dragged to. */
export const FLOAT_W = 400;
export const FLOAT_MIN = 280;
/** The most of the town's width the docks take together, so the picture always keeps some. */
export const DOCKS_MOST = 0.8;

/** Each side's share as drawn (0 for a side with no dock): both scaled down together when they would take more than DOCKS_MOST. */
export function sharesOf(left: number, right: number): Record<Side, number> {
  const k = left + right > DOCKS_MOST ? DOCKS_MOST / (left + right) : 1;
  return { left: left * k, right: right * k };
}

/** What goes somewhere: tabs taken from a pane, or a thing to open (a link dragged in, a person clicked). */
export type Moving = { from: string; tabs: string[]; shown: string } | { opens: string; title: string };

export type WindowAction =
  /** A tab bar's own change, made in the tree that holds the pane it names; a move into another tree goes across. */
  | { type: 'tree'; action: Action }
  /** Brings up the tab that opens this wherever it is, else opens it in `pane` while a tab bar holds it, else in a new tab bar floating
   *  at `at` (its left edge), else in a dock. */
  | { type: 'show'; opens: string; title: string; pane?: string; at?: number }
  | { type: 'dock'; what: Moving; side: Side }
  | { type: 'float'; what: Moving; x: number }
  | { type: 'stop'; side: Side; stop: number }
  | { type: 'closeDock'; side: Side }
  /** Puts a floating tab bar on top of the others. */
  | { type: 'raise'; float: string }
  /** A floating tab bar moved or resized: its left edge and its width. */
  | { type: 'placeFloat'; float: string; x: number; w?: number };

export const NONE: Windows = { docks: { left: null, right: null }, stops: { left: STOPS[0], right: STOPS[0] }, floats: [] };

const docked = (w: Windows): Node[] => SIDES.flatMap((s) => w.docks[s] ?? []);
const trees = (w: Windows): Node[] => [...docked(w), ...w.floats.map((f) => f.root)];
const opened = (n: Node) => panes(n).some((p) => p.tabs.some((t) => t.opens));

/** `w` with the tree that holds node `id` replaced by f(tree). */
function onTree(w: Windows, id: string, f: (n: Node) => Node): Windows {
  const side = SIDES.find((s) => w.docks[s] && holds(w.docks[s], id));
  if (side) return { ...w, docks: { ...w.docks, [side]: f(w.docks[side]!) } };
  return { ...w, floats: w.floats.map((x) => (holds(x.root, id) ? { ...x, root: f(x.root) } : x)) };
}

/** A tree with nothing open left goes: a dock or a float is there only for what it shows. */
function prune(w: Windows): Windows {
  const keep = (n: Node | null) => (n && opened(n) ? n : null);
  const docks = { left: keep(w.docks.left), right: keep(w.docks.right) };
  const floats = w.floats.filter((f) => opened(f.root));
  const same = docks.left === w.docks.left && docks.right === w.docks.right && floats.length === w.floats.length;
  return same ? w : { ...w, docks, floats };
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
      if (a.at !== undefined) return { ...w, floats: [...w.floats, { id: newId(), root: initial(a.opens, a.title), x: a.at, w: FLOAT_W }] };
      const dock = w.docks.right ?? w.docks.left;
      if (!dock) return { ...w, docks: { ...w.docks, right: initial(a.opens, a.title) } };
      return onTree(w, dock.id, (n) => reduce(n, { type: 'open', opens: a.opens, title: a.title, to: { pane: panes(n)[0].id } }));
    }
    case 'dock': {
      const got = take(w, a.what);
      if (!got) return w;
      const [rest, pane] = got;
      const left = prune(rest);
      const there = left.docks[a.side];
      const dock = there ? placeTabs(there, pane.tabs, pane.active, { pane: last(there).id, edge: 'down' }) : pane;
      return { ...left, docks: { ...left.docks, [a.side]: dock } };
    }
    case 'float': {
      const got = take(w, a.what);
      if (!got) return w;
      const rest = prune(got[0]);
      return { ...rest, floats: [...rest.floats, { id: newId(), root: got[1], x: a.x, w: FLOAT_W }] };
    }
    case 'stop':
      return { ...w, stops: { ...w.stops, [a.side]: a.stop } };
    case 'closeDock':
      return { ...w, docks: { ...w.docks, [a.side]: null } };
    case 'placeFloat':
      return { ...w, floats: w.floats.map((f) => (f.id === a.float ? { ...f, x: a.x, w: a.w ?? f.w } : f)) };
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

/** The windows on a phone's width: every floating tab bar's and the left dock's tabs join the right dock's first tab bar (the dock is then
 *  a sheet under the picture), so nothing covers the picture and the sheet is never split into slivers. */
export function sheetOf(w: Windows): Windows {
  return [...(w.docks.left ? [w.docks.left] : []), ...w.floats.map((f) => f.root)].flatMap(panes).reduce((v, p) => {
    const what = { from: p.id, tabs: p.tabs.map((t) => t.id), shown: p.active };
    if (!v.docks.right) return reduceWindows(v, { type: 'dock', what, side: 'right' });
    const got = take(v, what);
    if (!got?.[0].docks.right) return v;
    const [rest, pane] = got;
    const sheet = rest.docks.right!;
    return prune({ ...rest, docks: { ...rest.docks, right: placeTabs(sheet, pane.tabs, pane.active, { pane: panes(sheet)[0].id }) } });
  }, w);
}

/** Where a new floating tab bar's left edge goes, as a share of the town's width: on the side away from `clickedX` (the share of the
 *  width last pressed at), so it does not cover who was clicked; each one more (`nth`) a little toward the middle. `wide`: the tab bar's
 *  width as a share of the town's. */
export function spotAway(clickedX: number, wide: number, nth: number): number {
  const step = 0.03 * (nth % 6);
  const room = Math.max(0, 1 - wide);
  return clickedX > 0.5 ? Math.min(room, 0.02 + step) : Math.max(0, room - 0.02 - step);
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

/** The pane that holds the keys: `focus` while it is there, else a dock's first, else the top float's. */
export function focusIn(w: Windows, focus: string): string {
  const all = trees(w).flatMap(panes);
  if (all.some((p) => p.id === focus)) return focus;
  const top = w.floats[w.floats.length - 1];
  const dock = docked(w)[0];
  return (dock ? panes(dock)[0] : top ? panes(top.root)[0] : undefined)?.id ?? '';
}

/** The side whose dock holds node `id`, if one does. */
export const sideOf = (w: Windows, id: string): Side | undefined => SIDES.find((s) => w.docks[s] && holds(w.docks[s], id));

/** The pane showing `opens`, if any. */
export const paneShowing = (w: Windows, opens: string) =>
  trees(w)
    .flatMap(panes)
    .find((p) => p.tabs.some((t) => t.id === p.active && t.opens === opens));

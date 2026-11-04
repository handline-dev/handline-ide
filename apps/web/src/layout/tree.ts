/** The workspace layout as data: a binary tree whose leaves are panes of tabs. Every change is a pure
 * function here, so the view only dispatches and renders, and the rules are tested without a DOM. */

export type Tab = {
  id: string;
  title: string;
  /** What the tab shows; none means Empty, which offers the choices. */
  opens?: string;
  /** false pins the tab: it cannot be closed or dragged. */
  closable?: boolean;
};
export type Pane = { kind: 'pane'; id: string; tabs: Tab[]; active: string };
export type Split = { kind: 'split'; id: string; axis: 'x' | 'y'; ratio: number; a: Node; b: Node };
export type Node = Pane | Split;

/** A side of a pane: a new pane goes there. */
export type Edge = 'left' | 'right' | 'up' | 'down';
/** Where a moved tab lands: among a pane's tabs at `index` (end when omitted), or at a pane's edge. */
export type Target = { pane: string; index?: number; edge?: never } | { pane: string; edge: Edge; index?: never };

export type Action =
  | { type: 'select'; pane: string; tab: string }
  | { type: 'open'; opens: string; title: string; to: Target }
  /** Brings up the tab that opens this, wherever it is; else opens it in the first pane. */
  | { type: 'show'; opens: string; title: string }
  | { type: 'choose'; pane: string; tab: string; opens: string; title: string }
  | { type: 'closeTab'; pane: string; tab: string }
  | { type: 'split'; pane: string; edge: Edge | 'four' }
  | { type: 'closePane'; pane: string }
  | { type: 'move'; from: string; tab: string; to: Target }
  | { type: 'resize'; split: string; ratio: number };

/** Unique across reloads too: layouts are saved. */
export const newId = () => crypto.randomUUID();

export const pane = (tabs: Tab[], active = tabs[0].id): Pane => ({ kind: 'pane', id: newId(), tabs, active });
export const hasFixed = (p: Pane) => p.tabs.some((t) => t.closable === false);
export const emptyTab = (): Tab => ({ id: newId(), title: 'Empty' });
export const emptyPane = () => pane([emptyTab()]);
/** A workspace's first pane: its fixed tab (Chat for an agent, the page itself for a page), which cannot close. */
export const initial = (opens = 'chat', title = 'Chat'): Node => pane([{ id: newId(), title, opens, closable: false }]);

const split = (axis: 'x' | 'y', a: Node, b: Node): Split => ({ kind: 'split', id: newId(), axis, ratio: 0.5, a, b });
export const clampRatio = (r: number) => Math.min(0.85, Math.max(0.15, r));

export function panes(node: Node): Pane[] {
  return node.kind === 'pane' ? [node] : [...panes(node.a), ...panes(node.b)];
}
export const findPane = (root: Node, id: string) => panes(root).find((p) => p.id === id);
/** Whether a tab already shows `opens` (a workspace holds one tab per thing it opens). */
export const isOpen = (root: Node, opens: string) => panes(root).some((p) => p.tabs.some((t) => t.opens === opens));

/** `root` with node `id` replaced by f(node); null removes it and its sibling takes the parent split's place. */
function edit(root: Node, id: string, f: (n: Node) => Node | null): Node | null {
  if (root.id === id) return f(root);
  if (root.kind === 'pane') return root;
  const a = edit(root.a, id, f);
  const b = edit(root.b, id, f);
  if (!a) return b;
  if (!b) return a;
  return a === root.a && b === root.b ? root : { ...root, a, b };
}

function attach(at: Pane, edge: Edge | 'four', added: Pane): Node {
  switch (edge) {
    case 'right':
      return split('x', at, added);
    case 'left':
      return split('x', added, at);
    case 'down':
      return split('y', at, added);
    case 'up':
      return split('y', added, at);
    case 'four':
      return split('x', split('y', at, added), split('y', emptyPane(), emptyPane()));
  }
}

/** Removes a tab; the neighbour becomes active, and a pane left with no tabs is removed. */
function withoutTab(p: Pane, tab: string): Pane | null {
  const i = p.tabs.findIndex((t) => t.id === tab);
  const rest = p.tabs.filter((t) => t.id !== tab);
  if (!rest.length) return null;
  return { ...p, tabs: rest, active: p.active === tab ? (rest[i] ?? rest[i - 1]).id : p.active };
}

/** An Empty tab only waits for a choice: anything opened or moved into its pane replaces it. */
function insert(p: Pane, tab: Tab, index = p.tabs.length): Pane {
  const before = p.tabs.slice(0, index).filter((t) => t.opens);
  const after = p.tabs.slice(index).filter((t) => t.opens);
  return { ...p, tabs: [...before, tab, ...after], active: tab.id };
}

function onPane(root: Node, id: string, f: (p: Pane) => Node | null): Node {
  return edit(root, id, (n) => (n.kind === 'pane' ? f(n) : n)) ?? initial();
}

export function reduce(root: Node, action: Action): Node {
  switch (action.type) {
    case 'select':
      return onPane(root, action.pane, (p) => ({ ...p, active: action.tab }));
    case 'open': {
      const tab = { id: newId(), title: action.title, opens: action.opens };
      const to = action.to;
      return onPane(root, to.pane, (p) => (to.edge ? attach(p, to.edge, pane([tab])) : insert(p, tab, to.index)));
    }
    case 'show': {
      const at = panes(root).find((p) => p.tabs.some((t) => t.opens === action.opens));
      if (at) return onPane(root, at.id, (p) => ({ ...p, active: p.tabs.find((t) => t.opens === action.opens)!.id }));
      return reduce(root, { type: 'open', opens: action.opens, title: action.title, to: { pane: panes(root)[0].id } });
    }
    case 'choose':
      return onPane(root, action.pane, (p) => ({
        ...p,
        tabs: p.tabs.map((t) => (t.id === action.tab ? { ...t, opens: action.opens, title: action.title } : t)),
      }));
    case 'closeTab': {
      const p = findPane(root, action.pane);
      const t = p?.tabs.find((x) => x.id === action.tab);
      if (!p || !t || t.closable === false) return root;
      // The last pane keeps a tab bar: closing its last tab leaves an Empty tab.
      if (p.tabs.length === 1 && root.kind === 'pane') {
        const e = emptyTab();
        return { ...p, tabs: [e], active: e.id };
      }
      return onPane(root, p.id, (x) => withoutTab(x, t.id));
    }
    case 'split':
      return onPane(root, action.pane, (p) => attach(p, action.edge, emptyPane()));
    case 'closePane': {
      // The pane holding the fixed tab stays, as the fixed tab does.
      const p = findPane(root, action.pane);
      return root.kind === 'pane' || !p || hasFixed(p) ? root : onPane(root, action.pane, () => null);
    }
    case 'resize':
      return edit(root, action.split, (n) => (n.kind === 'split' ? { ...n, ratio: clampRatio(action.ratio) } : n)) ?? root;
    case 'move':
      return move(root, action.from, action.tab, action.to);
  }
}

function move(root: Node, from: string, tabId: string, to: Target): Node {
  const src = findPane(root, from);
  const tab = src?.tabs.find((t) => t.id === tabId);
  if (!src || !tab || tab.closable === false || !findPane(root, to.pane)) return root;
  const edge = to.edge ?? null;
  // Splitting a pane off itself needs another tab to stay behind.
  if (edge && from === to.pane && src.tabs.length === 1) return root;

  if (!edge && from === to.pane) {
    // Reorder within the bar; the index counts the tabs as they are, the dragged one included.
    const i = src.tabs.findIndex((t) => t.id === tabId);
    const at = to.index ?? src.tabs.length;
    const rest = src.tabs.filter((t) => t.id !== tabId);
    const j = at > i ? at - 1 : at;
    return onPane(root, from, (p) => ({ ...p, tabs: [...rest.slice(0, j), tab, ...rest.slice(j)], active: tab.id }));
  }

  // Into the target first (its id survives), then out of the source, which may then disappear.
  const placed = onPane(root, to.pane, (p) => (edge ? attach(p, edge, pane([tab])) : insert(p, tab, to.index)));
  return onPane(placed, from, (p) => withoutTab(p, tabId));
}

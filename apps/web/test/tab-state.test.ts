import { afterEach, describe, expect, it } from 'vitest';
import { names, read, write } from '#src/persist/kv.ts';
import { strayTabNames, sweepTabState as sweepIds, tabIds } from '#src/persist/tabState.ts';
import { initial, panes, reduce, type Action, type Node, type Pane } from '#src/layout/tree.ts';

const run = (root: Node, ...actions: Action[]) => actions.reduce(reduce, root);
const tabOf = (root: Node, opens: string) =>
  panes(root)
    .flatMap((p) => p.tabs.map((t) => ({ p, t })))
    .find((x) => x.t.opens === opens)!;
const keep = (root: Node, opens: string) => write(`tab/${tabOf(root, opens).t.id}/show`, 'closed');
const kept = () => names('tab/').sort();

/** A workspace: Chat, then Tasks and Changes beside it in a pane of their own. */
function workspace() {
  const first = initial() as Pane;
  let root = run(first, { type: 'split', pane: first.id, edge: 'right' });
  const right = panes(root)[1]!;
  root = run(
    root,
    { type: 'choose', pane: right.id, tab: right.tabs[0]!.id, opens: 'tasks', title: 'Tasks' },
    { type: 'open', opens: 'diff', title: 'Changes', to: { pane: right.id } },
  );
  return root;
}

afterEach(() => localStorage.clear());
const sweepTabState = (layouts: Node[]) => sweepIds(new Set(tabIds(layouts).split(' ')));

describe('tab state', () => {
  it('names only what belongs to a tab no layout has', () => {
    expect(strayTabNames(['tab/a/show', 'tab/b/only', 'tab/b/open/x/y', 'layouts', 'folded'], new Set(['a']))).toEqual([
      'tab/b/only',
      'tab/b/open/x/y',
    ]);
  });

  it('a closed tab takes what it kept with it; the others keep theirs', () => {
    let root = workspace();
    keep(root, 'tasks');
    keep(root, 'diff');
    const changes = tabOf(root, 'diff');
    root = run(root, { type: 'closeTab', pane: changes.p.id, tab: changes.t.id });
    sweepTabState([root]);
    expect(kept()).toEqual([`tab/${tabOf(root, 'tasks').t.id}/show`]);
  });

  it('a closed pane takes the state of every tab in it', () => {
    let root = workspace();
    keep(root, 'tasks');
    keep(root, 'diff');
    root = run(root, { type: 'closePane', pane: tabOf(root, 'tasks').p.id });
    sweepTabState([root]);
    expect(kept()).toEqual([]);
  });

  it('a tab moved to another pane keeps its state', () => {
    let root = workspace();
    keep(root, 'tasks');
    const tasks = tabOf(root, 'tasks');
    root = run(root, { type: 'move', from: tasks.p.id, tab: tasks.t.id, to: { pane: panes(root)[0]!.id } });
    sweepTabState([root]);
    expect(read(`tab/${tasks.t.id}/show`, 'open')).toBe('closed');
  });

  it('a tab in another workspace keeps its state, and what earlier runs left is swept', () => {
    const a = workspace();
    const b = workspace();
    keep(b, 'tasks');
    write('tab/gone-long-ago/url', 'https://example.com');
    write('folded', ['x']);
    sweepTabState([a, b]);
    expect(kept()).toEqual([`tab/${tabOf(b, 'tasks').t.id}/show`]);
    expect(read('folded', [])).toEqual(['x']);
  });
});

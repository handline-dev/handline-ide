import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { names, read, write } from '#src/persist/kv.ts';
import { heldCount, strayTabNames, sweepTabState as sweepIds, TabId, tabIds, useTabState } from '#src/persist/tabState.ts';
import { initial, panes, reduce, type Action, type Node, type Pane } from '#src/layout/tree.ts';

const run = (root: Node, ...actions: Action[]) => actions.reduce(reduce, root);
const tabOf = (root: Node, opens: string) =>
  panes(root)
    .flatMap((p) => p.tabs.map((t) => ({ p, t })))
    .find((x) => x.t.opens === opens)!;
const sweep = (...layouts: Node[]) => sweepIds(new Set(tabIds(layouts).split(' ')));

/** A tab view with one held value: its text shows the value, a click sets it. */
function View() {
  const [show, setShow] = useTabState('show', 'open');
  return (
    <button type="button" onClick={() => setShow('closed')}>
      {show}
    </button>
  );
}
const roots: Root[] = [];
/** Draws the view in tab `id`, optionally clicks it, and returns its text and a way to take it off screen. */
async function draw(id: string, click = false) {
  const el = document.createElement('div');
  const root = createRoot(el);
  roots.push(root);
  await act(async () =>
    root.render(
      <TabId.Provider value={id}>
        <View />
      </TabId.Provider>,
    ),
  );
  if (click) await act(async () => el.querySelector('button')!.click());
  return { text: el.textContent, gone: () => act(async () => root.unmount()) };
}

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

afterEach(async () => {
  for (const r of roots.splice(0)) await act(async () => r.unmount());
  sweepIds(new Set());
  localStorage.clear();
});

describe('tab state', () => {
  it('a view drawn again in its tab finds the value it left; another tab starts fresh', async () => {
    const a = await draw('a', true);
    await a.gone();
    expect((await draw('a')).text).toBe('closed');
    expect((await draw('b')).text).toBe('open');
  });

  it('names only what belongs to a tab no layout has', () => {
    expect(strayTabNames(['tab/a/show', 'tab/b/only', 'tab/b/open/x/y', 'layouts'], new Set(['a']))).toEqual([
      'tab/b/only',
      'tab/b/open/x/y',
    ]);
  });

  it('a closed tab or pane takes what it held with it; a moved tab and one in another workspace keep theirs', async () => {
    let root = workspace();
    const other = workspace();
    for (const r of [root, other]) for (const o of ['tasks', 'diff']) await (await draw(tabOf(r, o).t.id, true)).gone();
    expect(heldCount()).toBe(4);
    const tasks = tabOf(root, 'tasks');
    root = run(root, { type: 'move', from: tasks.p.id, tab: tasks.t.id, to: { pane: panes(root)[0]!.id } });
    const changes = tabOf(root, 'diff');
    root = run(root, { type: 'closeTab', pane: changes.p.id, tab: changes.t.id });
    sweep(root, other);
    expect(heldCount()).toBe(3);
    expect((await draw(tasks.t.id)).text).toBe('closed');
    sweep(run(other, { type: 'closePane', pane: tabOf(other, 'tasks').p.id }));
    expect(heldCount()).toBe(0);
  });

  it('a hundred tabs opened, set and closed leave nothing held', async () => {
    const before = heldCount();
    for (let i = 0; i < 100; i++) await (await draw(`t${i}`, true)).gone();
    expect(heldCount()).toBe(before + 100);
    sweep(workspace());
    expect(heldCount()).toBe(before);
  });

  it('what a tab shows stays across a reload; a dead tab, and a view value an earlier build kept, are swept from storage', () => {
    const root = workspace();
    const live = tabOf(root, 'tasks').t.id;
    write(`tab/${live}/url`, 'https://example.com');
    write(`tab/${live}/show`, 'closed');
    write('tab/gone-long-ago/path', 'notes.md');
    write('folded', ['x']);
    sweep(root);
    expect(names('tab/')).toEqual([`tab/${live}/url`]);
    expect(read('folded', [])).toEqual(['x']);
  });
});

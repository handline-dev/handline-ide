import { describe, expect, it } from 'vitest';
import { panes, snapRatio, stepStop, type Node } from '#src/layout/tree.ts';
import { NONE, chatsShown, focusIn, reduceWindows, type WindowAction, type Windows } from '#src/town/windows.ts';

const run = (w: Windows, ...actions: WindowAction[]) => actions.reduce(reduceWindows, w);
const show = (opens: string): WindowAction => ({ type: 'show', opens, title: opens });
const opens = (n: Node | null | undefined) => (n ? panes(n).map((p) => p.tabs.map((t) => t.opens)) : null);
const first = (n: Node) => panes(n)[0];

describe('divider stops', () => {
  it('snaps to the nearest third or half, and steps from stop to stop', () => {
    expect(snapRatio(0.3)).toBeCloseTo(1 / 3);
    expect(snapRatio(0.45)).toBe(0.5);
    expect(snapRatio(0.95)).toBeCloseTo(2 / 3);
    expect(stepStop(0.5, 1)).toBeCloseTo(2 / 3);
    expect(stepStop(0.4, -1)).toBeCloseTo(1 / 3);
    expect(stepStop(2 / 3, 1)).toBeCloseTo(2 / 3);
  });
});

describe('town windows', () => {
  it('opens a click in the dock, brings up what is open already, and drops the dock when its last tab closes', () => {
    const w = run(NONE, show('chat:api'), show('chat:web'), show('chat:api'));
    expect(opens(w.dock)).toEqual([['chat:api', 'chat:web']]);
    expect(chatsShown(w)).toEqual(new Set(['api']));
    const p = first(w.dock!);
    const shut = run(w, ...p.tabs.map((t): WindowAction => ({ type: 'tree', action: { type: 'closeTab', pane: p.id, tab: t.id } })));
    expect(shut.dock).toBeNull();
  });

  it('floats a tab bar and docks it back on the side it is dropped on, keeping its pane', () => {
    const w = run(NONE, show('chat:api'), show('chat:web'));
    const p = first(w.dock!);
    const floated = run(w, { type: 'float', what: { from: p.id, tabs: p.tabs.map((t) => t.id), shown: p.active }, x: 0.2, y: 0.1 });
    expect(floated.dock).toBeNull();
    expect(floated.floats).toHaveLength(1);
    expect(first(floated.floats[0].root).id).toBe(p.id);
    const back = run(floated, { type: 'dock', what: { from: p.id, tabs: [p.tabs[1].id], shown: p.tabs[1].id }, side: 'left' });
    expect(back.side).toBe('left');
    expect(opens(back.dock)).toEqual([['chat:web']]);
    expect(opens(back.floats[0].root)).toEqual([['chat:api']]);
  });

  it('a second tab bar docked joins the dock under the first', () => {
    const w = run(NONE, show('chat:api'), { type: 'float', what: { opens: 'task:7', title: 'T7' }, x: 0, y: 0 });
    const f = first(w.floats[0].root);
    const both = run(w, { type: 'dock', what: { from: f.id, tabs: [f.tabs[0].id], shown: f.active }, side: 'right' });
    expect(both.floats).toHaveLength(0);
    expect(both.dock?.kind === 'split' && both.dock.axis).toBe('y');
    expect(opens(both.dock)).toEqual([['chat:api'], ['task:7']]);
  });

  it('moves a tab across trees, and a link dragged in that is open already moves rather than opening twice', () => {
    const w = run(NONE, show('chat:api'), { type: 'float', what: { opens: 'task:7', title: 'T7' }, x: 0, y: 0 });
    const d = first(w.dock!);
    const f = first(w.floats[0].root);
    const moved = run(w, { type: 'tree', action: { type: 'move', from: f.id, tab: f.tabs[0].id, to: { pane: d.id } } });
    expect(opens(moved.dock)).toEqual([['chat:api', 'task:7']]);
    expect(moved.floats).toHaveLength(0);
    const again = run(moved, { type: 'float', what: { opens: 'chat:api', title: 'api' }, x: 0.5, y: 0.5 });
    expect(opens(again.dock)).toEqual([['task:7']]);
    expect(opens(again.floats[0].root)).toEqual([['chat:api']]);
  });

  it('raises a float to the top', () => {
    const w = run(
      NONE,
      { type: 'float', what: { opens: 'a', title: 'a' }, x: 0, y: 0 },
      { type: 'float', what: { opens: 'b', title: 'b' }, x: 0, y: 0 },
    );
    const raised = run(w, { type: 'raise', float: w.floats[0].id });
    expect(raised.floats.map((f) => opens(f.root))).toEqual([[['b']], [['a']]]);
  });

  it('a tab bar asking to show or open what another one holds brings that one up, never a second copy', () => {
    const w = run(NONE, show('chat:api'), show('task:7'), { type: 'float', what: { opens: 'chat:web', title: 'web' }, x: 0, y: 0 });
    const f = first(w.floats[0].root);
    const opened = run(w, { type: 'tree', action: { type: 'open', opens: 'chat:api', title: 'api', to: { pane: f.id } } });
    expect(opens(opened.floats[0].root)).toEqual([['chat:web']]);
    expect(chatsShown(opened)).toEqual(new Set(['api', 'web']));
    const shown = run(w, { type: 'tree', action: { type: 'show', opens: 'chat:api', title: 'api' } });
    expect(chatsShown(shown)).toEqual(new Set(['api', 'web']));
  });

  it('opens a click in a new floating tab bar, then in the tab bar it is given, and a tab dropped on open space makes another', () => {
    const at = { x: 0.3, y: 0.1 };
    const w = run(NONE, { type: 'show', opens: 'chat:api', title: 'api', at });
    expect(w.dock).toBeNull();
    expect(w.floats).toHaveLength(1);
    expect(w.floats[0]).toMatchObject(at);
    const f = first(w.floats[0].root);
    const two = run(w, { type: 'show', opens: 'task:7', title: 'T7', pane: f.id, at });
    expect(two.floats.map((x) => opens(x.root))).toEqual([[['chat:api', 'task:7']]]);
    const t = first(two.floats[0].root).tabs[1];
    const apart = run(two, { type: 'float', what: { from: f.id, tabs: [t.id], shown: t.id }, x: 0.6, y: 0.4 });
    expect(apart.floats.map((x) => opens(x.root))).toEqual([[['chat:api']], [['task:7']]]);
    const gone = run(w, { type: 'show', opens: 'task:7', title: 'T7', pane: 'gone', at });
    expect(gone.floats).toHaveLength(2);
  });

  it('a move onto a tab bar that is gone keeps the tabs where they were', () => {
    const w = run(NONE, { type: 'float', what: { opens: 'task:1', title: 'T1' }, x: 0, y: 0 });
    const f = first(w.floats[0].root);
    const after = run(w, { type: 'tree', action: { type: 'move', from: f.id, tab: f.tabs[0].id, to: { pane: 'gone' } } });
    expect(after).toBe(w);
  });

  it('gives the keys to a pane that is there', () => {
    const w = run(NONE, show('chat:api'), { type: 'float', what: { opens: 'task:7', title: 'T7' }, x: 0, y: 0 });
    const f = first(w.floats[0].root);
    expect(focusIn(w, f.id)).toBe(f.id);
    expect(focusIn(w, 'gone')).toBe(first(w.dock!).id);
    expect(focusIn(run(w, { type: 'closeDock' }), 'gone')).toBe(f.id);
    expect(focusIn(NONE, 'gone')).toBe('');
  });
});

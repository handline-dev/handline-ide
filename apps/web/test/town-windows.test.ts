import { describe, expect, it } from 'vitest';
import { panes, stepRatio, type Node } from '#src/layout/tree.ts';
import {
  DOCKS_MOST,
  FLOAT_W,
  NONE,
  chatsShown,
  sharesOf,
  focusIn,
  holdsOpen,
  paneShowing,
  reduceWindows,
  sheetOf,
  spotAway,
  type WindowAction,
  type Windows,
} from '#src/town/windows.ts';

const run = (w: Windows, ...actions: WindowAction[]) => actions.reduce(reduceWindows, w);
const show = (opens: string): WindowAction => ({ type: 'show', opens, title: opens });
const opens = (n: Node | null | undefined) => (n ? panes(n).map((p) => p.tabs.map((t) => t.opens)) : null);
const first = (n: Node) => panes(n)[0];

describe('divider keys', () => {
  it('step a twentieth of the whole, within the bounds', () => {
    expect(stepRatio(0.5, 1)).toBeCloseTo(0.55);
    expect(stepRatio(0.42, -1)).toBeCloseTo(0.37);
    expect(stepRatio(0.84, 1)).toBeCloseTo(0.85);
  });
});

describe('town windows', () => {
  it('opens a click in the dock, brings up what is open already, and drops the dock when its last tab closes', () => {
    const w = run(NONE, show('chat:api'), show('chat:web'), show('chat:api'));
    expect(opens(w.docks.right)).toEqual([['chat:api', 'chat:web']]);
    expect(chatsShown(w)).toEqual(new Set(['api']));
    const p = first(w.docks.right!);
    const shut = run(w, ...p.tabs.map((t): WindowAction => ({ type: 'tree', action: { type: 'closeTab', pane: p.id, tab: t.id } })));
    expect(shut.docks.right).toBeNull();
  });

  it('floats a tab bar and docks it back on the side it is dropped on, keeping its pane', () => {
    const w = run(NONE, show('chat:api'), show('chat:web'));
    const p = first(w.docks.right!);
    const floated = run(w, { type: 'float', what: { from: p.id, tabs: p.tabs.map((t) => t.id), shown: p.active }, x: 0.2 });
    expect(floated.docks.right).toBeNull();
    expect(floated.floats).toHaveLength(1);
    expect(first(floated.floats[0].root).id).toBe(p.id);
    const back = run(floated, { type: 'dock', what: { from: p.id, tabs: [p.tabs[1].id], shown: p.tabs[1].id }, side: 'left' });
    expect(back.docks.right).toBeNull();
    expect(opens(back.docks.left)).toEqual([['chat:web']]);
    expect(opens(back.floats[0].root)).toEqual([['chat:api']]);
  });

  it('a second tab bar docked joins the dock under the first', () => {
    const w = run(NONE, show('chat:api'), { type: 'float', what: { opens: 'task:7', title: 'T7' }, x: 0 });
    const f = first(w.floats[0].root);
    const both = run(w, { type: 'dock', what: { from: f.id, tabs: [f.tabs[0].id], shown: f.active }, side: 'right' });
    expect(both.floats).toHaveLength(0);
    expect(both.docks.right?.kind === 'split' && both.docks.right.axis).toBe('y');
    expect(opens(both.docks.right)).toEqual([['chat:api'], ['task:7']]);
  });

  it('moves a tab across trees, and a link dragged in that is open already moves rather than opening twice', () => {
    const w = run(NONE, show('chat:api'), { type: 'float', what: { opens: 'task:7', title: 'T7' }, x: 0 });
    const d = first(w.docks.right!);
    const f = first(w.floats[0].root);
    const moved = run(w, { type: 'tree', action: { type: 'move', from: f.id, tab: f.tabs[0].id, to: { pane: d.id } } });
    expect(opens(moved.docks.right)).toEqual([['chat:api', 'task:7']]);
    expect(moved.floats).toHaveLength(0);
    const again = run(moved, { type: 'float', what: { opens: 'chat:api', title: 'api' }, x: 0.5 });
    expect(opens(again.docks.right)).toEqual([['task:7']]);
    expect(opens(again.floats[0].root)).toEqual([['chat:api']]);
  });

  it('raises a float to the top', () => {
    const w = run(
      NONE,
      { type: 'float', what: { opens: 'a', title: 'a' }, x: 0 },
      { type: 'float', what: { opens: 'b', title: 'b' }, x: 0 },
    );
    const raised = run(w, { type: 'raise', float: w.floats[0].id });
    expect(raised.floats.map((f) => opens(f.root))).toEqual([[['b']], [['a']]]);
  });

  it('a tab bar asking to show or open what another one holds brings that one up, never a second copy', () => {
    const w = run(NONE, show('chat:api'), show('task:7'), { type: 'float', what: { opens: 'chat:web', title: 'web' }, x: 0 });
    const f = first(w.floats[0].root);
    const opened = run(w, { type: 'tree', action: { type: 'open', opens: 'chat:api', title: 'api', to: { pane: f.id } } });
    expect(opens(opened.floats[0].root)).toEqual([['chat:web']]);
    expect(chatsShown(opened)).toEqual(new Set(['api', 'web']));
    const shown = run(w, { type: 'tree', action: { type: 'show', opens: 'chat:api', title: 'api' } });
    expect(chatsShown(shown)).toEqual(new Set(['api', 'web']));
  });

  it('opens a click in a new floating tab bar, then in the tab bar it is given, and a tab dropped on open space makes another', () => {
    const at = 0.3;
    const w = run(NONE, { type: 'show', opens: 'chat:api', title: 'api', at });
    expect(w.docks).toEqual({ left: null, right: null });
    expect(w.floats).toHaveLength(1);
    expect(w.floats[0]).toMatchObject({ x: at, w: FLOAT_W });
    const f = first(w.floats[0].root);
    const two = run(w, { type: 'show', opens: 'task:7', title: 'T7', pane: f.id, at });
    expect(two.floats.map((x) => opens(x.root))).toEqual([[['chat:api', 'task:7']]]);
    const t = first(two.floats[0].root).tabs[1];
    const apart = run(two, { type: 'float', what: { from: f.id, tabs: [t.id], shown: t.id }, x: 0.6 });
    expect(apart.floats.map((x) => opens(x.root))).toEqual([[['chat:api']], [['task:7']]]);
    const gone = run(w, { type: 'show', opens: 'task:7', title: 'T7', pane: 'gone', at });
    expect(gone.floats).toHaveLength(2);
  });

  it('a move onto a tab bar that is gone keeps the tabs where they were', () => {
    const w = run(NONE, { type: 'float', what: { opens: 'task:1', title: 'T1' }, x: 0 });
    const f = first(w.floats[0].root);
    const after = run(w, { type: 'tree', action: { type: 'move', from: f.id, tab: f.tabs[0].id, to: { pane: 'gone' } } });
    expect(after).toBe(w);
  });

  it('docks on both sides at once, each with its own width, and moves a tab bar from one side to the other', () => {
    const w = run(NONE, show('chat:api'), { type: 'float', what: { opens: 'task:7', title: 'T7' }, x: 0 });
    const f = first(w.floats[0].root);
    const both = run(w, { type: 'dock', what: { from: f.id, tabs: [f.tabs[0].id], shown: f.active }, side: 'left' });
    expect(opens(both.docks.left)).toEqual([['task:7']]);
    expect(opens(both.docks.right)).toEqual([['chat:api']]);
    const wider = run(both, { type: 'stop', side: 'left', stop: 0.5 });
    expect(wider.stops).toEqual({ left: 0.5, right: NONE.stops.right });
    const r = first(both.docks.right!);
    const over = run(both, { type: 'dock', what: { from: r.id, tabs: [r.tabs[0].id], shown: r.active }, side: 'left' });
    expect(over.docks.right).toBeNull();
    expect(opens(over.docks.left)).toEqual([['task:7'], ['chat:api']]);
    expect(run(both, { type: 'closeDock', side: 'left' }).docks.right).toBe(both.docks.right);
  });

  it('draws two docks that would take too much of the width smaller together, so the picture keeps some', () => {
    expect(sharesOf(1 / 3, 1 / 3)).toEqual({ left: 1 / 3, right: 1 / 3 });
    expect(sharesOf(0.5, 0)).toEqual({ left: 0.5, right: 0 });
    const s = sharesOf(2 / 3, 2 / 3);
    expect(s.left + s.right).toBeCloseTo(DOCKS_MOST);
    expect(s.left).toBeCloseTo(s.right);
  });

  it('keeps where a float is and how wide', () => {
    const w = run(NONE, { type: 'float', what: { opens: 'a', title: 'a' }, x: 0.1 });
    const id = w.floats[0].id;
    expect(run(w, { type: 'placeFloat', float: id, x: 0.4, w: 520 }).floats[0]).toMatchObject({ x: 0.4, w: 520 });
    expect(run(w, { type: 'placeFloat', float: id, x: 0.2 }).floats[0]).toMatchObject({ x: 0.2, w: FLOAT_W });
  });

  it('gives the keys to a pane that is there', () => {
    const w = run(NONE, show('chat:api'), { type: 'float', what: { opens: 'task:7', title: 'T7' }, x: 0 });
    const f = first(w.floats[0].root);
    expect(focusIn(w, f.id)).toBe(f.id);
    expect(focusIn(w, 'gone')).toBe(first(w.docks.right!).id);
    expect(focusIn(run(w, { type: 'closeDock', side: 'right' }), 'gone')).toBe(f.id);
    expect(focusIn(NONE, 'gone')).toBe('');
  });
});

describe('on a phone width', () => {
  const at = 0.3;
  it('puts every floating tab bar into one tab bar of the sheet, keeping what the dock showed first', () => {
    const w = run(
      NONE,
      { type: 'show', opens: 'chat:api', title: 'api', at },
      { type: 'float', what: { opens: 'chat:web', title: 'web' }, x: at },
    );
    expect(w.floats).toHaveLength(2);
    const sheet = sheetOf(w);
    expect(sheet.floats).toEqual([]);
    expect(opens(sheet.docks.right)).toEqual([['chat:api', 'chat:web']]);
    const docked = run(NONE, show('requests'), { type: 'float', what: { opens: 'chat:web', title: 'web' }, x: at });
    expect(opens(sheetOf(docked).docks.right)).toEqual([['requests', 'chat:web']]);
  });
  it('puts the left dock into the sheet too', () => {
    const w = run(NONE, show('chat:api'), { type: 'float', what: { opens: 'chat:web', title: 'web' }, x: at });
    const f = first(w.floats[0].root);
    const both = run(w, { type: 'dock', what: { from: f.id, tabs: [f.tabs[0].id], shown: f.active }, side: 'left' });
    const sheet = sheetOf(both);
    expect(sheet.docks.left).toBeNull();
    expect(opens(sheet.docks.right)).toEqual([['chat:api', 'chat:web']]);
  });
  it('leaves windows with nothing floating or docked left as they are', () => {
    const w = run(NONE, show('chat:api'));
    expect(sheetOf(w)).toBe(w);
  });
});

describe('where a window opens', () => {
  it('opens on the side away from the click, inside the town, each one more a little further in', () => {
    expect(spotAway(0.2, 0.3, 0)).toBeCloseTo(0.68);
    expect(spotAway(0.8, 0.3, 0)).toBeCloseTo(0.02);
    expect(spotAway(0.8, 0.3, 2)).toBeCloseTo(0.08);
    expect(spotAway(0.2, 0.3, 2)).toBeCloseTo(0.62);
    // A town narrower than the window: it starts at the left edge.
    expect(spotAway(0.2, 1.4, 0)).toBe(0);
    expect(spotAway(0.8, 1.4, 3)).toBe(0);
  });
});

describe('a tab bar holding something', () => {
  it('holds it behind another tab too, where it no longer shows', () => {
    const one = run(NONE, { type: 'dock', what: { opens: 'chat:a', title: 'a' }, side: 'left' });
    const two = run(one, { type: 'show', opens: 'chat:b', title: 'b', pane: first(one.docks.left!).id });
    expect([holdsOpen(two, 'chat:a'), !!paneShowing(two, 'chat:a'), holdsOpen(two, 'chat:c')]).toEqual([true, false, false]);
  });
});

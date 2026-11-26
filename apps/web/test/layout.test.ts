import { describe, expect, it } from 'vitest';
import { closedBy, findPane, initial, panes, reduce, reopen, type Action, type Node, type Pane } from '#src/layout/tree.ts';

/** A layout on a Chat tab, as a sidebar pick leaves it. */
const initialChat = () => initial('chat', 'Chat');
const run = (root: Node, ...actions: Action[]) => actions.reduce(reduce, root);
const only = (root: Node) => root as Pane;
const titles = (p: Pane | undefined) => p?.tabs.map((t) => t.title);
const open = (pane: string, title: string, index?: number): Action => ({
  type: 'open',
  opens: title.toLowerCase(),
  title,
  to: { pane, index },
});

describe('layout tree', () => {
  it('starts on an Empty tab; every tab closes, the last one leaving an Empty tab', () => {
    expect(titles(only(initial()))).toEqual(['Empty']);
    const root = only(initialChat());
    expect(titles(only(reduce(root, { type: 'closeTab', pane: root.id, tab: root.tabs[0].id })))).toEqual(['Empty']);
  });

  it('navigates the shown tab in place, as a browser tab follows a link', () => {
    const p = only(initialChat());
    const root = only(run(p, open(p.id, 'Tasks'), { type: 'navigate', pane: p.id, opens: 'chat:a5', title: 'Chat' }));
    expect(root.tabs.map((t) => t.opens)).toEqual(['chat', 'chat:a5']);
    expect(root.active).toBe(root.tabs[1].id);
    expect(root.tabs[1].id).not.toBe(only(run(p, open(p.id, 'Tasks'))).tabs[1].id);
  });

  it('splits a pane with an Empty one, which + or a drop replaces', () => {
    const p = only(initialChat());
    let root = run(p, { type: 'split', pane: p.id, edge: 'right' });
    const [, right] = panes(root);
    expect(titles(right)).toEqual(['Empty']);
    root = run(root, open(right.id, 'Tasks'));
    expect(titles(findPane(root, right.id))).toEqual(['Tasks']);
  });

  it('closing the last tab of a split-off pane closes the pane', () => {
    const p = only(initialChat());
    let root = run(p, { type: 'split', pane: p.id, edge: 'down' });
    const right = panes(root)[1];
    root = run(root, { type: 'closeTab', pane: right.id, tab: right.tabs[0].id });
    expect(root.kind).toBe('pane');
  });

  it('a tab moved out gives its pane back to the tab shown before it, not its neighbour', () => {
    const p = only(initialChat());
    const chat = p.tabs[0].id;
    let root = run(p, open(p.id, 'Task1'), { type: 'select', pane: p.id, tab: chat }, open(p.id, 'Task2'));
    const task2 = only(root).tabs[2].id;
    root = run(root, { type: 'move', from: p.id, tab: task2, to: { pane: p.id, edge: 'right' } });
    expect(findPane(root, p.id)?.active).toBe(chat);
    root = run(root, open(p.id, 'Task3'));
    root = run(root, { type: 'closeTab', pane: p.id, tab: findPane(root, p.id)!.tabs[2].id });
    expect(findPane(root, p.id)?.active).toBe(chat);
  });

  it('opens at an index, and reorders within a bar', () => {
    const p = only(initialChat());
    let root = run(p, open(p.id, 'A'), open(p.id, 'B'), open(p.id, 'C', 1));
    expect(titles(only(root))).toEqual(['Chat', 'C', 'A', 'B']);
    const b = only(root).tabs[3];
    root = run(root, { type: 'move', from: p.id, tab: b.id, to: { pane: p.id, index: 1 } });
    expect(titles(only(root))).toEqual(['Chat', 'B', 'C', 'A']);
  });

  it('merges a tab into another pane, closing the emptied source', () => {
    const p = only(initialChat());
    let root = run(p, { type: 'split', pane: p.id, edge: 'right' });
    const right = panes(root)[1];
    root = run(root, open(right.id, 'Tasks'));
    const tasks = findPane(root, right.id)!.tabs[0];
    root = run(root, { type: 'move', from: right.id, tab: tasks.id, to: { pane: p.id, index: 1 } });
    expect(root.kind).toBe('pane');
    expect(titles(only(root))).toEqual(['Chat', 'Tasks']);
    expect(only(root).active).toBe(tasks.id);
  });

  it('drops a tab at an edge: a new pane on that side', () => {
    const p = only(initialChat());
    let root = run(p, open(p.id, 'Tasks'));
    const tasks = only(root).tabs[1];
    root = run(root, { type: 'move', from: p.id, tab: tasks.id, to: { pane: p.id, edge: 'left' } });
    expect(root.kind === 'split' && root.axis).toBe('x');
    expect(panes(root).map(titles)).toEqual([['Tasks'], ['Chat']]);
  });

  it('moves several tabs together, in their bar order, showing the one named', () => {
    const p = only(initialChat());
    let root = run(p, open(p.id, 'A'), open(p.id, 'B'), open(p.id, 'C'), open(p.id, 'D'));
    const [, a, , c, d] = only(root).tabs;
    root = run(root, { type: 'move', from: p.id, tab: a.id, tabs: [d.id, a.id], to: { pane: p.id, index: 1 } });
    expect(titles(only(root))).toEqual(['Chat', 'A', 'D', 'B', 'C']);
    root = run(root, { type: 'move', from: p.id, tab: c.id, tabs: [a.id, c.id, d.id], to: { pane: p.id, edge: 'right' } });
    expect(panes(root).map(titles)).toEqual([
      ['Chat', 'B'],
      ['A', 'D', 'C'],
    ]);
    const right = panes(root)[1];
    expect(right.active).toBe(c.id);
    root = run(root, { type: 'move', from: right.id, tab: a.id, tabs: right.tabs.map((t) => t.id), to: { pane: p.id, index: 1 } });
    expect(root.kind).toBe('pane');
    expect(titles(only(root))).toEqual(['Chat', 'A', 'D', 'C', 'B']);
    expect(only(root).active).toBe(a.id);
  });

  it('moves a group into a bar with an Empty tab in one piece', () => {
    const p = only(initialChat());
    let root = run(p, open(p.id, 'A'), open(p.id, 'B'), { type: 'split', pane: p.id, edge: 'right' });
    const right = panes(root)[1];
    root = run(root, open(right.id, 'C'));
    root = run(root, { type: 'split', pane: right.id, edge: 'down' });
    const low = panes(root)[2];
    const [, a, b] = findPane(root, p.id)!.tabs;
    root = run(root, { type: 'move', from: p.id, tab: b.id, tabs: [a.id, b.id], to: { pane: low.id, index: 1 } });
    expect(titles(findPane(root, low.id))).toEqual(['A', 'B']);
    expect(findPane(root, low.id)!.active).toBe(b.id);
  });

  it('will not split a pane off itself with its only tab', () => {
    const p = only(initialChat());
    let root = run(p, { type: 'split', pane: p.id, edge: 'right' });
    const right = panes(root)[1];
    root = run(root, open(right.id, 'Tasks'));
    const before = root;
    const tasks = findPane(root, right.id)!.tabs[0];
    expect(run(root, { type: 'move', from: right.id, tab: tasks.id, to: { pane: right.id, edge: 'down' } })).toBe(before);
    const moved = run(root, { type: 'move', from: p.id, tab: p.tabs[0].id, to: { pane: right.id } });
    expect(panes(moved).map(titles)).toEqual([['Tasks', 'Chat']]);
  });

  it('opens a dragged-in thing (a sidebar agent) at an edge', () => {
    const p = only(initialChat());
    const root = run(p, { type: 'open', opens: 'agent', title: 'handline', to: { pane: p.id, edge: 'down' } });
    expect(root.kind === 'split' && root.axis).toBe('y');
    expect(panes(root).map(titles)).toEqual([['Chat'], ['handline']]);
  });

  it('clamps a resize', () => {
    const p = only(initialChat());
    let root = run(p, { type: 'split', pane: p.id, edge: 'right' });
    root = run(root, { type: 'resize', split: root.id, ratio: 0.99 });
    expect(root.kind === 'split' && root.ratio).toBe(0.85);
  });
});

describe('agent list', () => {
  it('lists the saved order first, then the rest by name', async () => {
    const { ordered } = await import('#src/layout/agentList.ts');
    const list = ordered(
      [
        { key: 'ui', name: 'ui', state: 'running' },
        { key: 'lead', name: 'lead', state: 'idle' },
        { key: 'api', name: 'api', state: 'idle' },
        { key: 'zed', name: 'zed', state: 'idle' },
      ],
      ['zed', 'ui'],
    );
    expect(list.map((a) => a.key)).toEqual(['zed', 'ui', 'api', 'lead']);
  });
});

describe('show', () => {
  it('selects an open tab of that kind anywhere, else opens it in the first pane', () => {
    const p = only(initialChat());
    let root = run(p, { type: 'split', pane: p.id, edge: 'right' });
    const right = panes(root)[1];
    root = run(root, open(right.id, 'Tasks'), { type: 'select', pane: right.id, tab: right.tabs[0].id });
    root = run(root, { type: 'show', opens: 'tasks', title: 'Tasks' });
    expect(findPane(root, right.id)!.tabs.find((t) => t.id === findPane(root, right.id)!.active)?.opens).toBe('tasks');
    root = run(root, { type: 'show', opens: 'usage', title: 'Usage' });
    expect(titles(panes(root)[0])).toEqual(['Chat', 'Usage']);
  });
});

describe('activityOf', () => {
  const of = (state: string) => ({ state, has_dialog: false });
  it('sorts the state words, unknown as resting', async () => {
    const { activityOf } = await import('#src/layout/agentList.ts');
    const words = (l: string[]) => l.map((w) => activityOf(of(w)));
    expect(words(['running', 'starting', 'queued'])).toEqual(['working', 'waking', 'waking']);
    expect(words(['parked', 'no_terminal'])).toEqual(['asleep', 'asleep']);
    expect(words(['stalled', 'blocked', 'waiting'])).toEqual(['error', 'error', 'you']);
    expect(words(['idle', 'something-new'])).toEqual(['resting', 'resting']);
  });

  it('reads AgentView.status: idle state with background work running is working', async () => {
    const { activityOf } = await import('#src/layout/agentList.ts');
    const status = { state: 'running', reason: 'background', text: '', until: null, since: null, tasks: [] };
    expect(activityOf({ ...of('idle'), status })).toBe('working');
  });
});

describe('restarting', () => {
  it('is a settings change or an asked restart under way, not a move or a full context', async () => {
    const { restarting } = await import('#src/layout/agentList.ts');
    const h = (cause: string) => ({ handoff: { cause, phase: 'waiting' as const, since: '' } });
    expect([restarting(h('settings_applied')), restarting(h('restart')), restarting(h('move')), restarting({})]).toEqual([
      true,
      true,
      false,
      false,
    ]);
  });
});

describe('holdLine', () => {
  const status = (state: string, reason: string, text: string, tasks: number[] = []) => ({
    state,
    reason,
    text,
    until: null,
    since: null,
    tasks,
  });
  it('names the tasks a held agent waits on, calm, not as a fault', async () => {
    const { activityOf, holdLine } = await import('#src/layout/agentList.ts');
    const a = { state: 'idle', has_dialog: false, status: status('waiting', 'task', 'blocked by T8771 (human)', [8771, 8772]) };
    expect(activityOf(a)).toBe('held');
    expect(holdLine(a)).toBe('Waiting on T8771, T8772');
  });
  it('a stop is a fault and says its own line; a working agent has none', async () => {
    const { activityOf, holdLine } = await import('#src/layout/agentList.ts');
    const a = { state: 'stalled', has_dialog: false, status: status('stopped', 'stalled', 'stalled until someone unstalls it') };
    expect(activityOf(a)).toBe('error');
    expect(holdLine(a)).toBe('Stalled until someone unstalls it');
    expect(holdLine({ state: 'running', has_dialog: false })).toBeNull();
  });
  it('a restart for a changed model says so while the agent still works, then while it starts', async () => {
    const { activityOf, holdLine } = await import('#src/layout/agentList.ts');
    const h = { cause: 'settings_applied', settings: ['model'], since: '2026-11-08T12:00:00Z' };
    const waiting = { state: 'running', has_dialog: false, handoff: { ...h, phase: 'waiting' as const } };
    expect(activityOf(waiting)).toBe('working');
    expect(holdLine(waiting)).toBe('Waiting for its turn to end, then restarts: model changed');
    const starting = { state: 'idle', has_dialog: false, handoff: { ...h, phase: 'starting' as const } };
    expect(holdLine(starting)).toBe('Starting a new session to restart: model changed');
    expect(holdLine({ ...starting, handoff: { ...starting.handoff, cause: 'context_full' } })).toBeNull();
  });
  it("a starting agent shows the daemon's own words with the waking mark", async () => {
    const { activityOf, holdLine } = await import('#src/layout/agentList.ts');
    const a = { state: 'idle', has_dialog: false, status: status('preparing', 'work', 'Starting in a moment') };
    expect(activityOf(a)).toBe('waking');
    expect(holdLine(a)).toBe('Starting in a moment');
  });
});

describe('pins', () => {
  it('takes the pinned agents out of the list, in pin order', async () => {
    const { splitPinned } = await import('#src/layout/agentList.ts');
    const agents = ['a', 'b', 'c'].map((key) => ({ key, name: key, state: 'idle' }));
    const { pinned, rest } = splitPinned(agents, ['c', 'a', 'gone']);
    expect(pinned.map((a) => a.key)).toEqual(['c', 'a']);
    expect(rest.map((a) => a.key)).toEqual(['b']);
  });
});

describe('close pane', () => {
  it('closes any pane of a split, never the last one', () => {
    const start = only(initialChat());
    const split = reduce(start, { type: 'split', pane: start.id, edge: 'right' });
    expect(panes(reduce(split, { type: 'closePane', pane: start.id })).length).toBe(1);
    expect(reduce(start, { type: 'closePane', pane: start.id })).toBe(start);
  });
});

describe('reopen closed', () => {
  const opened = () => {
    let r = initialChat();
    const p = panes(r)[0].id;
    r = reduce(r, { type: 'open', opens: 'task:1', title: 'T1', to: { pane: p } });
    r = reduce(r, { type: 'open', opens: 'task:2', title: 'T2', to: { pane: p } });
    return r;
  };
  const opens = (r: Node) => panes(r).map((p) => p.tabs.map((t) => t.opens));

  it('a closed tab comes back where it stood', () => {
    const r = opened();
    const p = panes(r)[0];
    const close: Action = { type: 'closeTab', pane: p.id, tab: p.tabs[1].id };
    const c = closedBy(r, reduce(r, close), close)!;
    expect(opens(reopen(reduce(r, close), c, p.id))).toEqual([['chat', 'task:1', 'task:2']]);
  });

  it('a terminal or a refused close is not kept', () => {
    let r = opened();
    const p = panes(r)[0];
    r = reduce(r, { type: 'open', opens: 'shell:abc', title: 'Terminal', to: { pane: p.id } });
    expect(
      closedBy(r, reduce(r, { type: 'closeTab', pane: p.id, tab: panes(r)[0].tabs[3].id }), {
        type: 'closeTab',
        pane: p.id,
        tab: panes(r)[0].tabs[3].id,
      }),
    ).toBeNull();
    expect(closedBy(r, reduce(r, { type: 'closePane', pane: p.id }), { type: 'closePane', pane: p.id })).toBeNull();
  });

  it('a closed pane brings all its tabs back into the fallback pane; one already open is only shown', () => {
    let r = initialChat();
    const first = panes(r)[0].id;
    r = reduce(r, { type: 'open', opens: 'task:1', title: 'T1', to: { pane: first, edge: 'right' } });
    const side = panes(r)[1];
    r = reduce(r, { type: 'open', opens: 'task:2', title: 'T2', to: { pane: side.id } });
    const close: Action = { type: 'closePane', pane: side.id };
    const c = closedBy(r, reduce(r, close), close)!;
    expect(c.tabs.map((t) => t.tab.opens)).toEqual(['task:1', 'task:2']);
    let after = reduce(r, close);
    after = reduce(after, { type: 'open', opens: 'task:2', title: 'T2', to: { pane: first } });
    const back = reopen(after, c, first);
    expect(opens(back)).toEqual([['chat', 'task:2', 'task:1']]);
  });
});

describe("an agent's own tab", () => {
  it('carries the agent', async () => {
    const { carry, ownedBy, labelOf } = await import('#src/layout/kinds.tsx');
    expect(carry('shell', 'a5')).toBe('shell|a5');
    expect(carry('tasks', 'a5')).toBe('tasks');
    expect(ownedBy('shell:t3|a5')).toEqual({ kind: 'shell:t3', agent: 'a5' });
    expect(ownedBy('web:https://x@y.z')).toBeNull();
    expect(ownedBy('file:node_modules/@types/x.md')).toBeNull();
    expect(carry('file:node_modules/@types/x.md', 'a5')).toBe('file:node_modules/@types/x.md|a5');
    expect(labelOf('shell|a5', () => 'town')).toBe('Agent terminal · town');
    // Tasks never carries one, but a desk's papers in the town open one agent's.
    expect(ownedBy('tasks|a5')).toEqual({ kind: 'tasks', agent: 'a5' });
    expect(labelOf('tasks|a5', () => 'town')).toBe('Tasks · town');
  });

  it('is opened naming the agent it was asked from, so focusing it never leaves it with no agent to draw', async () => {
    const { openFrom } = await import('#src/Workspace.tsx');
    const root = initialChat();
    const pane = only(root).id;
    const sent: Action[] = [];
    const ctx = { root, agent: 'a5', dispatch: (a: Action) => void sent.push(a), nameOf: (k: string) => k, focus: () => {} };
    openFrom(ctx, pane, 'shell', 'side');
    openFrom(ctx, pane, 'tasks', 'side');
    openFrom({ ...ctx, agent: null }, pane, 'shell', 'side');
    // A joined cluster's agent keeps its cluster, so a focus there never turns the tab to this Mac's agent of that name.
    openFrom({ ...ctx, agent: '@c1/bob' }, pane, 'shell', 'side');
    openFrom({ ...ctx, agent: '@c1/bob' }, pane, '@c1/diff', 'side');
    openFrom({ ...ctx, agent: '@c1/bob' }, pane, '@c2/shell', 'side');
    expect(sent.map((a) => (a.type === 'open' ? a.opens : a.type))).toEqual([
      'shell|a5',
      'tasks',
      'shell',
      '@c1/shell|bob',
      '@c1/diff|bob',
      '@c2/shell',
    ]);
    // Asked again from the same agent, the tab already open comes up instead of a second one.
    const opened = run(root, sent[0]);
    sent.length = 0;
    openFrom({ ...ctx, root: opened }, pane, 'shell');
    expect(sent.map((a) => a.type)).toEqual(['show']);
  });
});

describe('the agent a tab shows', () => {
  it('reads it from a chat or an agent kind, with its cluster', async () => {
    const { agentOf } = await import('#src/layout/screenLayout.ts');
    expect(agentOf('chat:a5')).toBe('a5');
    expect(agentOf('shell|a5')).toBe('a5');
    expect(agentOf('@c1/chat:a5')).toBe('@c1/a5');
    expect(agentOf('tasks')).toBeNull();
    expect(agentOf(undefined)).toBeNull();
  });
});

it('dropTabs leaves out closed tabs, so Back does not bring them back; a pane left empty goes', async () => {
  const { dropTabs, pane, panes } = await import('#src/layout/tree.ts');
  const a = { id: 'a', title: 'A', opens: 'chat:x' };
  const b = { id: 'b', title: 'B', opens: 'shell:1|x' };
  const c = { id: 'c', title: 'C', opens: 'tasks' };
  const root = { kind: 'split' as const, id: 's', axis: 'x' as const, ratio: 0.5, a: pane([a, b]), b: pane([c]) };
  const after = dropTabs(root, new Set(['b', 'c']));
  expect(panes(after).flatMap((p) => p.tabs.map((t) => t.id))).toEqual(['a']);
});

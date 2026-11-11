import { describe, expect, it } from 'vitest';
import { closedBy, findPane, initial, panes, reduce, reopen, type Action, type Node, type Pane } from '#src/layout/tree.ts';

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
  it('starts on one pane with a pinned Chat', () => {
    const root = only(initial());
    expect(titles(root)).toEqual(['Chat']);
    expect(reduce(root, { type: 'closeTab', pane: root.id, tab: root.tabs[0].id })).toBe(root);
  });

  it('splits a pane with an Empty one, which + or a drop replaces', () => {
    const p = only(initial());
    let root = run(p, { type: 'split', pane: p.id, edge: 'right' });
    const [, right] = panes(root);
    expect(titles(right)).toEqual(['Empty']);
    root = run(root, open(right.id, 'Tasks'));
    expect(titles(findPane(root, right.id))).toEqual(['Tasks']);
  });

  it('closing the last tab of a split-off pane closes the pane', () => {
    const p = only(initial());
    let root = run(p, { type: 'split', pane: p.id, edge: 'down' });
    const right = panes(root)[1];
    root = run(root, { type: 'closeTab', pane: right.id, tab: right.tabs[0].id });
    expect(root.kind).toBe('pane');
  });

  it('opens at an index, and reorders within a bar', () => {
    const p = only(initial());
    let root = run(p, open(p.id, 'A'), open(p.id, 'B'), open(p.id, 'C', 1));
    expect(titles(only(root))).toEqual(['Chat', 'C', 'A', 'B']);
    const b = only(root).tabs[3];
    root = run(root, { type: 'move', from: p.id, tab: b.id, to: { pane: p.id, index: 1 } });
    expect(titles(only(root))).toEqual(['Chat', 'B', 'C', 'A']);
  });

  it('merges a tab into another pane, closing the emptied source', () => {
    const p = only(initial());
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
    const p = only(initial());
    let root = run(p, open(p.id, 'Tasks'));
    const tasks = only(root).tabs[1];
    root = run(root, { type: 'move', from: p.id, tab: tasks.id, to: { pane: p.id, edge: 'left' } });
    expect(root.kind === 'split' && root.axis).toBe('x');
    expect(panes(root).map(titles)).toEqual([['Tasks'], ['Chat']]);
  });

  it('will not split a pane off itself with its only tab, nor move a pinned tab', () => {
    const p = only(initial());
    let root = run(p, { type: 'split', pane: p.id, edge: 'right' });
    const right = panes(root)[1];
    root = run(root, open(right.id, 'Tasks'));
    const before = root;
    const tasks = findPane(root, right.id)!.tabs[0];
    expect(run(root, { type: 'move', from: right.id, tab: tasks.id, to: { pane: right.id, edge: 'down' } })).toBe(before);
    expect(run(root, { type: 'move', from: p.id, tab: p.tabs[0].id, to: { pane: right.id } })).toBe(before);
  });

  it('opens a dragged-in thing (a sidebar agent) at an edge', () => {
    const p = only(initial());
    const root = run(p, { type: 'open', opens: 'agent', title: 'handline', to: { pane: p.id, edge: 'down' } });
    expect(root.kind === 'split' && root.axis).toBe('y');
    expect(panes(root).map(titles)).toEqual([['Chat'], ['handline']]);
  });

  it('clamps a resize', () => {
    const p = only(initial());
    let root = run(p, { type: 'split', pane: p.id, edge: 'right' });
    root = run(root, { type: 'resize', split: root.id, ratio: 0.99 });
    expect(root.kind === 'split' && root.ratio).toBe(0.85);
  });
});

describe('agent tree', () => {
  it('nests agents under their parent, sorted, with unknown or self parents at the top', async () => {
    const { agentTree } = await import('#src/layout/agentTree.ts');
    const tree = agentTree([
      { key: 'handline.ui', name: 'handline.ui', parent: 'handline', state: 'running' },
      { key: 'handline', name: 'handline', parent: null, state: 'idle' },
      { key: 'handline.api', name: 'handline.api', parent: 'handline', state: 'idle' },
      { key: 'orphan', name: 'orphan', parent: 'gone', state: 'idle' },
      { key: 'loop', name: 'loop', parent: 'loop', state: 'idle' },
    ]);
    const flat = (ns: typeof tree, d = 0): string[] => ns.flatMap((n) => [`${d}:${n.agent.key}`, ...flat(n.children, d + 1)]);
    expect(flat(tree)).toEqual(['0:handline', '1:handline.api', '1:handline.ui', '0:loop', '0:orphan']);
  });
});

describe('show', () => {
  it('selects an open tab of that kind anywhere, else opens it in the first pane', () => {
    const p = only(initial());
    let root = run(p, { type: 'split', pane: p.id, edge: 'right' });
    const right = panes(root)[1];
    root = run(root, open(right.id, 'Tasks'), { type: 'select', pane: right.id, tab: right.tabs[0].id });
    root = run(root, { type: 'show', opens: 'tasks', title: 'Tasks' });
    expect(findPane(root, right.id)!.tabs.find((t) => t.id === findPane(root, right.id)!.active)?.opens).toBe('tasks');
    root = run(root, { type: 'show', opens: 'usage', title: 'Usage' });
    expect(titles(panes(root)[0])).toEqual(['Chat', 'Usage']);
  });
});

describe('shortName', () => {
  it('drops the parent prefix', async () => {
    const { shortName } = await import('#src/layout/agentTree.ts');
    expect(shortName('a.b', 'a')).toBe('b');
    expect(shortName('a.b.c', 'a')).toBe('b.c');
    expect(shortName('ab.c', 'a')).toBe('ab.c');
    expect(shortName('a.b', null)).toBe('a.b');
  });
});

describe('activityOf', () => {
  const of = (state: string) => ({ state, has_dialog: false });
  it('sorts the state words, unknown as resting', async () => {
    const { activityOf } = await import('#src/layout/agentTree.ts');
    const words = (l: string[]) => l.map((w) => activityOf(of(w)));
    expect(words(['running', 'starting', 'queued'])).toEqual(['working', 'waking', 'waking']);
    expect(words(['parked', 'no_terminal'])).toEqual(['asleep', 'asleep']);
    expect(words(['stalled', 'blocked', 'waiting'])).toEqual(['error', 'error', 'you']);
    expect(words(['idle', 'something-new'])).toEqual(['resting', 'resting']);
  });

  it('reads AgentView.status: idle state with background work running is working', async () => {
    const { activityOf } = await import('#src/layout/agentTree.ts');
    const status = { state: 'running', reason: 'background', text: '', until: null, since: null, tasks: [] };
    expect(activityOf({ ...of('idle'), status })).toBe('working');
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
    const { activityOf, holdLine } = await import('#src/layout/agentTree.ts');
    const a = { state: 'idle', has_dialog: false, status: status('waiting', 'task', 'blocked by T8771 (human)', [8771, 8772]) };
    expect(activityOf(a)).toBe('held');
    expect(holdLine(a)).toBe('Waiting on T8771, T8772');
  });
  it('a stop is a fault and says its own line; a working agent has none', async () => {
    const { activityOf, holdLine } = await import('#src/layout/agentTree.ts');
    const a = { state: 'stalled', has_dialog: false, status: status('stopped', 'stalled', 'stalled until someone unstalls it') };
    expect(activityOf(a)).toBe('error');
    expect(holdLine(a)).toBe('Stalled until someone unstalls it');
    expect(holdLine({ state: 'running', has_dialog: false })).toBeNull();
  });
  it('a restart for a changed model says so while the agent still works, then while it starts', async () => {
    const { activityOf, holdLine } = await import('#src/layout/agentTree.ts');
    const h = { cause: 'settings_applied', settings: ['model'], since: '2026-11-08T12:00:00Z' };
    const waiting = { state: 'running', has_dialog: false, handoff: { ...h, phase: 'waiting' as const } };
    expect(activityOf(waiting)).toBe('working');
    expect(holdLine(waiting)).toBe('Restarting: model changed. Waiting for its turn to end…');
    const starting = { state: 'idle', has_dialog: false, handoff: { ...h, phase: 'starting' as const } };
    expect(holdLine(starting)).toBe('Restarting: model changed. Starting a new session…');
    expect(holdLine({ ...starting, handoff: { ...starting.handoff, cause: 'context_full' } })).toBeNull();
  });
});

describe('pathTo', () => {
  it('keeps only the branch down to the picked agent', async () => {
    const { agentTree, pathTo } = await import('#src/layout/agentTree.ts');
    const tree = agentTree([
      { key: 'a', name: 'a', parent: null, state: 'idle' },
      { key: 'a.b', name: 'a.b', parent: 'a', state: 'idle' },
      { key: 'a.b.c', name: 'a.b.c', parent: 'a.b', state: 'idle' },
      { key: 'a.d', name: 'a.d', parent: 'a', state: 'idle' },
    ]);
    const names = (ns: typeof tree): string[] => ns.flatMap((n) => [n.agent.key, ...names(n.children)]);
    expect(names(pathTo(tree[0].children, 'a.b.c'))).toEqual(['a.b', 'a.b.c']);
    expect(pathTo(tree[0].children, 'x')).toEqual([]);
  });
});

describe('pins', () => {
  const agents = [
    { key: 'a', name: 'a', parent: null, state: 'idle' },
    { key: 'a.x', name: 'a.x', parent: 'a', state: 'idle' },
    { key: 'b', name: 'b', parent: null, state: 'idle' },
    { key: 'c', name: 'c', parent: null, state: 'idle' },
  ];
  it('moves pinned agents with their subtrees out of the tree, in pin order', async () => {
    const { agentTree, splitPinned } = await import('#src/layout/agentTree.ts');
    const { pinned, rest } = splitPinned(agentTree(agents), ['c', 'a']);
    expect(pinned.map((n) => n.agent.key)).toEqual(['c', 'a']);
    expect(pinned[1].children.map((n) => n.agent.key)).toEqual(['a.x']);
    expect(rest.map((n) => n.agent.key)).toEqual(['b']);
  });

  it('pins a sub-agent out of its team: it shows once, under Pinned', async () => {
    const { agentTree, splitPinned } = await import('#src/layout/agentTree.ts');
    const { pinned, rest } = splitPinned(agentTree(agents), ['a.x']);
    expect(pinned.map((n) => [n.agent.key, n.children.length])).toEqual([['a.x', 0]]);
    expect(rest.find((n) => n.agent.key === 'a')!.children.map((n) => n.agent.key)).toEqual([]);
    const both = splitPinned(agentTree(agents), ['a', 'a.x']);
    expect(both.pinned.map((n) => [n.agent.key, n.children.length])).toEqual([
      ['a', 0],
      ['a.x', 0],
    ]);
  });
});

describe('fixed tab', () => {
  it('keeps the pane that holds it open', () => {
    const start = only(initial());
    const split = reduce(start, { type: 'split', pane: start.id, edge: 'right' });
    expect(reduce(split, { type: 'closePane', pane: start.id })).toBe(split);
  });
});

describe('reopen closed', () => {
  const opened = () => {
    let r = initial();
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

  it('a fixed tab, a terminal or a refused close is not kept', () => {
    let r = opened();
    const p = panes(r)[0];
    expect(
      closedBy(r, reduce(r, { type: 'closeTab', pane: p.id, tab: p.tabs[0].id }), { type: 'closeTab', pane: p.id, tab: p.tabs[0].id }),
    ).toBeNull();
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
    let r = initial();
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

describe('workingUnder', () => {
  it('counts the working and the starting at any depth', async () => {
    const { workingUnder } = await import('#src/layout/agentTree.ts');
    const node = (state: string, children: unknown[] = []) => ({ agent: { name: state, state, has_dialog: false }, children });
    const tree = [node('running', [node('starting'), node('idle')]), node('parked')];
    expect(workingUnder(tree as never)).toBe(2);
  });
});

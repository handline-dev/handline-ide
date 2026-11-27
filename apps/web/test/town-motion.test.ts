import { describe, expect, it } from 'vitest';
import type { AgentView, Workspace } from '@handline/client';
import type { Activity } from '#src/layout/agentList.ts';
import { townOf } from '#src/town/model.ts';
import { motionOf } from '#src/town/motion.ts';
import { painter, screenKind } from '#src/town/monitors.tsx';

describe('motionOf', () => {
  it('reads while its newest call looks at things, and while it has called nothing yet', () => {
    for (const doing of ['Read go/main.go', 'Grep callers of stepped', 'Glob', 'WebFetch the docs', 'WebSearch', null, '  '])
      expect(motionOf('working', doing), String(doing)).toBe('reading');
  });

  it('writes while its newest call changes a file', () => {
    for (const doing of ['Edit fix the bound', 'Write a new test', 'MultiEdit two files', 'NotebookEdit'])
      expect(motionOf('working', doing), doing).toBe('writing');
  });

  it('runs something for a command, a subagent or any other tool', () => {
    for (const doing of ['Bash Run the tests', 'Agent Review the change', 'Task', 'Skill', 'TodoWrite plan'])
      expect(motionOf('working', doing), doing).toBe('running');
  });

  it('reads an MCP tool by its own name, after the server', () => {
    expect(motionOf('working', 'mcp__docs__search_docs pairing')).toBe('reading');
    expect(motionOf('working', 'mcp__drive__update_file the plan')).toBe('writing');
    expect(motionOf('working', 'mcp__sim__tap the button')).toBe('running');
  });

  it('runs while background work goes on at its prompt with no call of its own', () => {
    expect(motionOf('working', null, true)).toBe('running');
    expect(motionOf('working', 'Edit the file', true)).toBe('writing');
  });

  it('waits for the founder, and sits still in every other state', () => {
    expect(motionOf('you', 'Read the file')).toBe('waiting');
    for (const a of ['waking', 'asleep', 'held', 'error', 'resting'] satisfies Activity[]) expect(motionOf(a, 'Bash x'), a).toBeNull();
  });
});

describe('townOf motion', () => {
  it("moves each person by the agents list's doing line and state, not by its task's title", () => {
    const ws = { key: 'w', name: 'core', folder: '', folders: [], added_folders: [], lead: null } as unknown as Workspace;
    const agent = (key: string, more: object) =>
      ({ key, name: key, workspace: 'w', state: 'idle', status: null, task: null, doing: null, ...more }) as unknown as AgentView;
    const t = townOf(
      [ws],
      [
        agent('a', { state: 'running', task: { id: 7, title: 'Read the notes' }, doing: { text: 'Edit notes.md', at: '' } }),
        agent('b', { background: [{ kind: 'shell' }] }),
        agent('c', {}),
      ],
      [],
    );
    expect(t.houses[0].people.map((p) => [p.key, p.motion])).toEqual([
      ['a', 'writing'],
      ['b', 'running'],
      ['c', null],
    ]);
  });
});

describe('screens', () => {
  it('show what their agent does: scrolling text, a line typing out, a terminal', () => {
    expect([screenKind('reading'), screenKind('writing'), screenKind('running'), screenKind(null)]).toEqual([
      'scroll',
      'type',
      'terminal',
      'scroll',
    ]);
  });

  it('type one letter a frame with the cursor after it, then move the lines up once the screen is full', () => {
    const { frames, at } = painter('type', 16, 3);
    const row = (f: number, y: number) => Array.from({ length: 16 }, (_, x) => at(f, x, y));
    const cursorAt = (f: number, y: number) => row(f, y).indexOf('cursor');
    expect([cursorAt(0, 0), cursorAt(1, 0), cursorAt(2, 0)]).toEqual([1, 2, 3]);
    // Nothing after the cursor, and no ink on the blank row between lines.
    expect(
      row(2, 0)
        .slice(4)
        .every((p) => p === null),
    ).toBe(true);
    expect(row(5, 1).every((p) => p === null)).toBe(true);
    // A screen two lines tall: the third line types on the bottom row, the second moved up to the top.
    const first = Array.from({ length: frames }, (_, f) => f).find((f) => cursorAt(f, 2) === 1) ?? -1;
    const third = Array.from({ length: frames }, (_, f) => f).filter((f) => cursorAt(f, 2) === 1)[1] ?? -1;
    expect(first).toBeGreaterThan(0);
    expect(cursorAt(third, 0)).toBe(-1);
    expect(row(third, 0).some((p) => p === 'ink')).toBe(true);
  });
});

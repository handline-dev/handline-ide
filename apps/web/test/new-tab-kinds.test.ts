import { describe, expect, it } from 'vitest';
import { newTabKinds } from '#src/layout/newTabKinds.ts';
import { carry } from '#src/layout/kinds.tsx';

const kinds = [
  { kind: 'newshell', label: 'Terminal' },
  { kind: 'diff', label: 'Changes' },
  { kind: 'tasks', label: 'Tasks' },
];
const nameOf = (k: string) => (k === 'a2' ? 'architect' : k);
const agents = [{ key: 'a2', label: 'architect' }];

describe("the + menu's Terminal", () => {
  it('names the agent on screen, whose worktree the shell opens in, and asks nothing', () => {
    const m = newTabKinds(kinds, 'a2', nameOf, agents);
    expect(m[0]).toEqual({ kind: 'newshell', label: 'Terminal', hint: 'architect' });
    expect(m[1]!.agents).toBeUndefined();
  });
  it('with no agent on screen opens a plain shell: no name, no agent list; Changes still asks which agent', () => {
    const m = newTabKinds(kinds, null, nameOf, agents);
    expect(m[0]).toEqual({ kind: 'newshell', label: 'Terminal', hint: undefined });
    expect(m[1]!.agents).toEqual(agents);
    expect(m[2]).toEqual({ kind: 'tasks', label: 'Tasks' });
  });
});

describe('a plain shell', () => {
  it('never takes the agent of the pane it is moved or reopened in; an agent kind with none still does', () => {
    expect(carry('shell:h1', 'a2')).toBe('shell:h1');
    expect(carry('shell:h1|a3', 'a2')).toBe('shell:h1|a3');
    expect(carry('diff', 'a2')).toBe('diff|a2');
  });
});

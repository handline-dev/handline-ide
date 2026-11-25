import { describe, expect, it } from 'vitest';
import type { AgentView, PendingRequest, Workspace } from '@handline/client';
import { townOf } from '#src/town/model.ts';

const agent = (key: string, name: string, workspace: string, state = 'idle') =>
  ({ key, name, workspace, state, status: null, task: null, doing: null }) as unknown as AgentView;
const ws = (key: string, name: string, lead: string | null) =>
  ({ key, name, folder: '', folders: [], added_folders: [], lead }) as unknown as Workspace;
const ask = (task: number, agent: string | null) => ({ task, title: `T${task}`, kind: 'decide', agent }) as PendingRequest;

describe('townOf', () => {
  it('puts each agent and each decision in its own workspace house', () => {
    const t = townOf(
      [ws('w1', 'core', 'a1'), ws('w2', 'web', null)],
      [agent('a1', 'lead', 'w1'), agent('a2', 'ui', 'w2')],
      [ask(1, 'a1'), ask(2, 'ui'), ask(3, null)],
    );
    expect(t.houses.map((h) => [h.name, h.lead?.name ?? null, h.people.map((p) => p.name), h.decisions.map((d) => d.task)])).toEqual([
      ['core', 'lead', ['lead'], [1]],
      ['web', null, ['ui'], [2]],
    ]);
    // The founder's own house holds every decision, the daemon's own (no agent) among them.
    expect(t.decisions.map((d) => d.task)).toEqual([1, 2, 3]);
  });

  it('knows whose prompt cache still holds its conversation, at the time it is asked', () => {
    const now = Date.parse('2026-01-01T12:00:00Z');
    const until = (key: string, at: string | null) => ({ ...agent(key, key, 'w'), cache_until: at }) as unknown as AgentView;
    const t = townOf(
      [ws('w', 'core', null)],
      [until('a', '2026-01-01T12:04:00Z'), until('b', '2026-01-01T11:59:00Z'), until('c', null)],
      [],
      now,
    );
    expect(t.houses[0].people.map((p) => [p.key, p.cached])).toEqual([
      ['a', true],
      ['b', false],
      ['c', false],
    ]);
  });
});

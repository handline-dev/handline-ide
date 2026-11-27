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

  it('knows the task each one holds, and why one on hold or stopped waits', () => {
    const status = (state: string, reason: string, text: string) => ({ state, reason, text, until: null, since: '', tasks: [] });
    const on = (key: string, more: object) => ({ ...agent(key, key, 'w'), ...more }) as unknown as AgentView;
    const t = townOf(
      [ws('w', 'core', null)],
      [
        on('a', {
          state: 'running',
          task: { id: 7, title: 'Plan the notes' },
          steps: { done: 3, total: 7 },
          doing: { text: 'Bash runs the tests', at: '' },
        }),
        on('b', { status: status('waiting', 'wall', 'usage limit, back at 14:00') }),
        on('c', { status: status('stopped', 'stalled', 'Start failed') }),
        on('d', { status: status('off', '', '') }),
      ],
      [],
    );
    expect(t.houses[0].people.map((p) => [p.key, p.activity, p.doing, p.task, p.steps, p.why])).toEqual([
      // The task's title names the work, over the tool call of the moment; its steps come with the list.
      ['a', 'working', 'Plan the notes', 7, { done: 3, total: 7 }, null],
      ['b', 'held', null, null, null, 'Usage limit, back at 14:00'],
      ['c', 'error', null, null, null, 'Start failed'],
      // With no session it rests; its prompt cache alone says whether it dozes.
      ['d', 'resting', null, null, null, null],
    ]);
  });

  it("carries what a person's card says: its team, role, model, effort and context use", () => {
    const more = { role: ' Reviews every change before it lands ', model: 'sonnet', effort: 'high', ctx_pct: 25 };
    const t = townOf(
      [ws('w', 'core', null)],
      [{ ...agent('a', 'reviewer', 'w'), ...more } as unknown as AgentView, agent('b', 'ui', 'w')],
      [],
    );
    const [a, b] = t.houses[0].people;
    expect([a.team, a.role, a.model, a.effort, a.ctx]).toEqual(['core', 'Reviews every change before it lands', 'sonnet', 'high', 25]);
    // What the daemon does not say stays empty, so the card leaves the line out.
    expect([b.team, b.role, b.model, b.effort, b.ctx]).toEqual(['core', '', null, null, null]);
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

  it('counts one resting at its prompt while its subagents or background commands run as working', () => {
    const jobs = (key: string, ...kinds: string[]) =>
      ({ ...agent(key, key, 'w'), background: kinds.map((kind) => ({ kind })) }) as AgentView;
    const t = townOf([ws('w', 'core', null)], [jobs('a', 'subagent'), jobs('b', 'shell'), jobs('c', 'monitor'), jobs('d')], []);
    // A Monitor watch only waits.
    expect(t.houses[0].people.map((p) => [p.key, p.activity])).toEqual([
      ['a', 'working'],
      ['b', 'working'],
      ['c', 'resting'],
      ['d', 'resting'],
    ]);
  });
});

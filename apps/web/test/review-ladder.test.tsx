import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';

const { calls, write, ok, ladder } = vi.hoisted(() => {
  const calls: { method: string; params: unknown }[] = [];
  const write = (method: string) => {
    const send = (params: unknown) => (calls.push({ method, params }), Promise.resolve({}));
    return { mutate: send, mutateAsync: send, isPending: false, error: null };
  };
  const ok = (data: unknown) => ({ isSuccess: true, data });
  const record = (oks: boolean[]) => oks.map((ok_first, i) => ({ task: 9100 + i, ok_first, at: '2026-11-20T00:00:00Z' }));
  const ladder = [
    { work: 'contract', level: 'every', record: record([true, true]), looks: 2, suggested: null },
    { work: 'screen', level: 'one_in_two', record: record([false, true, true, true, true, true, true]), looks: 7, suggested: null },
    { work: 'daemon', level: 'every', record: [], looks: 0, suggested: null },
  ];
  return { calls, write, ok, ladder };
});
vi.mock('@handline/app-data', () => ({
  useAgentReview: () => ok({ agent: 'ui', ladder, lessons: 0 }),
  useLessons: () => ok({ items: [] }),
  useNameOf: () => (k: string) => k,
  useProposals: () => ok({ proposals: [] }),
  useTask: () => ({}),
  useWrite: write,
  classify: (e: unknown) => e,
  sentence: String,
}));
vi.mock('#src/requests/RowMenu.tsx', () => ({ useRowActions: () => ({}), RowMenu: () => null, usePick: () => ({}) }));
import { AgentPage } from '#src/settings/AgentPage.tsx';
import { TurnQuick } from '#src/requests/TurnQuick.tsx';

let root: Root | undefined;
const mount = async (node: React.ReactNode) => {
  const el = document.createElement('div');
  document.body.append(el);
  const r = (root = createRoot(el));
  await act(async () => r.render(node));
};
afterEach(async () => {
  calls.length = 0;
  await act(async () => root?.unmount());
  document.body.innerHTML = '';
});
const flush = () => act(async () => new Promise((r) => setTimeout(r, 20)));
const button = (text: string) => [...document.querySelectorAll('button')].reverse().find((b) => b.textContent === text)!;
const rows = () => [...document.querySelectorAll('li')].map((li) => li.textContent);

describe('the review ladder', () => {
  it('lists one row per kind done, the run toward the next step, and contract locked at every', async () => {
    await mount(<AgentPage agent="ui" back={() => {}} />);
    expect(rows()).toEqual(['Contract and dataEvery', 'Screen6 of 101 in 2', 'Daemon0 of 10Every']);
    expect(document.querySelector('[aria-label="Contract and data level"]')).toBeNull();
  });

  it('sets a level by hand for its kind, from its menu', async () => {
    await mount(<AgentPage agent="ui" back={() => {}} />);
    await act(async () => (document.querySelector('[aria-label="Screen level"]') as HTMLButtonElement).click());
    await flush();
    await act(async () =>
      ([...document.querySelectorAll('[role="menuitem"]')].find((x) => x.textContent === '1 in 3') as HTMLElement).click(),
    );
    expect(calls).toEqual([{ method: 'agent.review_set', params: { agent: 'ui', work: 'screen', level: 'one_in_three' } }]);
  });
});

describe('the step-down offer in Requests', () => {
  const offer = {
    id: 'review:ui:screen',
    source: 'machine',
    kind: 'warning',
    warning: { kind: 'review', agent: 'ui', work: 'screen', level: 'one_in_three', line: '' },
  } as unknown as Parameters<typeof TurnQuick>[0]['p'];

  it('Yes sets the offered level for its kind', async () => {
    await mount(<TurnQuick p={offer} />);
    await act(async () => button('Yes').click());
    expect(calls).toEqual([{ method: 'agent.review_set', params: { agent: 'ui', work: 'screen', level: 'one_in_three' } }]);
  });

  it('Not now dismisses the offer', async () => {
    await mount(<TurnQuick p={offer} />);
    await act(async () => button('Not now').click());
    expect(calls).toEqual([{ method: 'warning.dismiss', params: { id: 'review:ui:screen' } }]);
  });
});

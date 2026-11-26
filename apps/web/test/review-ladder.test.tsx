import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { calls, write, ok, ladder, settings } = vi.hoisted(() => {
  const settings: Record<string, string> = {};
  const calls: { method: string; params: unknown }[] = [];
  const write = (method: string) => {
    const send = (params: unknown) => {
      calls.push({ method, params });
      if (method === 'agent.set') Object.assign(settings, (params as { values: object }).values);
      return Promise.resolve({});
    };
    return { mutate: send, mutateAsync: send, isPending: false, error: null };
  };
  const ok = (data: unknown) => ({ isSuccess: true, data });
  const record = (oks: boolean[]) => oks.map((ok_first, i) => ({ task: 9100 + i, ok_first, at: '2026-11-20T00:00:00Z' }));
  const ladder = [
    { work: 'contract', level: 'every', record: record([true, true]), looks: 2, suggested: null },
    { work: 'screen', level: 'one_in_two', record: record([false, true, true, true, true, true, true]), looks: 7, suggested: null },
    { work: 'daemon', level: 'every', record: [], looks: 0, suggested: null },
  ];
  return { calls, write, ok, ladder, settings };
});
vi.mock('#src/settings/data/devices.ts', () => ({ useAgentDevice: () => null }));
vi.mock('@handline/app-data', () => ({
  useAgentContext: () => ({ data: undefined }),
  useAgentMemory: () => ({ data: undefined }),
  useAgentReview: () => ok({ agent: 'ui', ladder, lessons: 0 }),
  useAgentDetail: () => ok({ settings: { ...settings } }),
  useLessons: () => ok({ items: [] }),
  Hooks: {
    useAccessList: () => ({ data: { rules: [] } }),
    useAccessRuleRemove: () => ({ mutate() {} }),
    useSchedulesList: () => ({ data: undefined }),
  },
  useSettings: () => ({ isSuccess: false }),
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
beforeEach(() => void Object.assign(settings, { sandbox: 'off', sandbox_allow: '' }));
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
    await mount(<AgentPage agent="ui" />);
    expect(rows()).toEqual(['Contract and dataEvery', 'Screen6 of 101 in 2', 'Daemon0 of 10Every']);
    expect(document.querySelector('[aria-label="Contract and data level"]')).toBeNull();
  });

  it('sets a level by hand for its kind, from its menu', async () => {
    await mount(<AgentPage agent="ui" />);
    await act(async () => (document.querySelector('[aria-label="Screen level"]') as HTMLButtonElement).click());
    await flush();
    await act(async () =>
      ([...document.querySelectorAll('[role="menuitem"]')].find((x) => x.textContent === '1 in 3') as HTMLElement).click(),
    );
    expect(calls).toEqual([{ method: 'agent.review_set', params: { agent: 'ui', work: 'screen', level: 'one_in_three' } }]);
  });
});

describe('the sandbox block', () => {
  const sw = (label: string) => document.querySelector<HTMLButtonElement>(`[role="switch"][aria-label="${label}"]`)!;
  const list = '2026-11-20 host pypi.org\n2026-11-21 cert\n2026-11-22 command pnpm install';
  it('turns the sandbox on with agent.set, and lists nothing allowed until there is an entry', async () => {
    await mount(<AgentPage agent="ui" />);
    expect(document.querySelector('[aria-label^="Stop allowing"]')).toBeNull();
    await act(async () => sw('Run in a sandbox').click());
    expect(calls).toEqual([{ method: 'agent.set', params: { agent: 'ui', values: { sandbox: 'on' } } }]);
  });
  it('lists each entry, and its x writes the list without that line, one entry per line', async () => {
    Object.assign(settings, { sandbox: 'on', sandbox_allow: `${list}\nnot an entry` });
    await mount(<AgentPage agent="ui" />);
    const entries = [...document.querySelectorAll('li')]
      .filter((li) => li.querySelector('[aria-label^="Stop allowing"]'))
      .map((li) => li.textContent);
    expect(entries).toEqual([
      expect.stringMatching(/^pypi\.orgMay reachallowed /),
      expect.stringMatching(/^Go tools may use the macOS certificate checkallowed /),
      expect.stringMatching(/^pnpm installRuns outside the sandboxallowed /),
    ]);
    await act(async () => document.querySelector<HTMLButtonElement>('[aria-label="Stop allowing pypi.org"]')!.click());
    expect(calls.map((c) => (c.params as { values: object }).values)).toEqual([
      { sandbox_allow: '2026-11-21 cert\n2026-11-22 command pnpm install' },
    ]);
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

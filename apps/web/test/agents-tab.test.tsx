import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';

const mutate = vi.fn();
const ok = (data: unknown) => ({ isSuccess: true, data });
const values: Record<string, string> = {
  default_model_claude: 'opus',
  default_effort_claude: 'high',
  default_model_codex: '',
  default_effort_codex: 'medium',
};
vi.mock('@handline/app-data', () => ({
  useSettings: () => ok({ values }),
  hooks: { useAgentOptions: () => ok({ models: ['sonnet', 'opus', 'haiku'], efforts: ['low', 'medium', 'high', 'xhigh', 'max'] }) },
  useWrite: () => ({ mutate, isPending: false, error: null }),
  classify: (e: unknown) => e,
  sentence: String,
}));
import { AgentsTab } from '#src/settings/AgentsTab.tsx';

let root: Root | undefined;
let el: HTMLElement;
const mount = async () => {
  el = document.createElement('div');
  document.body.append(el);
  const r = (root = createRoot(el));
  await act(async () => r.render(<AgentsTab />));
};
afterEach(() => (mutate.mockClear(), act(() => root?.unmount())));
const pick = (label: string) => el.querySelector(`[aria-label="${label}"]`) as HTMLSelectElement & HTMLInputElement;
const change = async (t: HTMLSelectElement | HTMLInputElement, v: string, proto: object) =>
  act(async () => {
    Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(t, v);
    t.dispatchEvent(new Event(t instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true }));
  });

describe('Settings > Agents', () => {
  it("shows the daemon's defaults and saves a new Claude model", async () => {
    await mount();
    expect(pick('Claude model').value).toBe('opus');
    await change(pick('Claude model'), 'haiku', HTMLSelectElement.prototype);
    expect(mutate).toHaveBeenCalledWith({ changes: { default_model_claude: 'haiku' } });
  });
  it('saves the Codex model on Enter, trimmed', async () => {
    await mount();
    const box = pick('Codex model');
    await change(box, ' gpt-5 ', HTMLInputElement.prototype);
    await act(async () => box.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })));
    expect(mutate).toHaveBeenLastCalledWith({ changes: { default_model_codex: 'gpt-5' } });
  });
  it('an emptied Codex box sends null, so Codex picks again, and an unchanged one sends nothing', async () => {
    values.default_model_codex = 'gpt-5';
    await mount();
    const box = pick('Codex model');
    await act(async () => box.dispatchEvent(new FocusEvent('focusout', { bubbles: true })));
    expect(mutate).not.toHaveBeenCalled();
    await change(box, '', HTMLInputElement.prototype);
    await act(async () => box.dispatchEvent(new FocusEvent('focusout', { bubbles: true })));
    expect(mutate).toHaveBeenCalledWith({ changes: { default_model_codex: null } });
    values.default_model_codex = '';
  });
});

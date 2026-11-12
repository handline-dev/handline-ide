import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.stubGlobal('Notification', { permission: 'default', requestPermission: async () => 'granted' });
const ok = (data: unknown) => ({ isSuccess: true, isPending: false, isFetching: false, data });
const machine = { accounts: [] as object[], missing: [] as object[], repos: 0, agents: 0, push: {} as Record<string, boolean> };
const set = vi.fn();
vi.mock('@handline/app-data', () => ({
  useStatus: () => ok({ missing_tools: machine.missing, vendors: [] }),
  useAccounts: () => ok({ accounts: machine.accounts }),
  useRepos: () => ok({ repos: Array.from({ length: machine.repos }, () => ({})) }),
  useAgents: () => ok({ agents: Array.from({ length: machine.agents }, () => ({})) }),
  useFoundLogins: () => ({ isLoading: false, data: [] }),
  useSettings: () => ok({ values: machine.push }),
  useWrite: () => ({ mutateAsync: vi.fn(), mutate: set, isPending: false }),
  hooks: { useReposRecent: () => ok({ folders: [{}, {}] }) },
  classify: (e: unknown) => e,
  sentence: String,
}));
import { openOnboarding, resetOnboardingForTest } from '#src/onboarding/data/progress.ts';
import { NotificationsTab } from '#src/settings/NotificationsTab.tsx';
import { OnboardingTab } from '#src/settings/OnboardingTab.tsx';

let root: Root;
let el: HTMLElement;
const mount = async (ui: React.ReactNode) => {
  el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
  await act(async () => root.render(ui));
};
beforeEach(() => {
  resetOnboardingForTest();
  openOnboarding(2);
  Object.assign(machine, {
    accounts: [{ login_state: 'ok' }],
    missing: [{ tool: 'tmux', fix: 'brew install tmux' }],
    repos: 0,
    agents: 0,
    push: {},
  });
  set.mockClear();
});
afterEach(() => act(() => root.unmount()));

describe('Settings > Onboarding', () => {
  it('shows which of the five are done, the count, and where Continue opens', async () => {
    await mount(<OnboardingTab go={() => {}} />);
    expect(el.textContent).toContain('1 of 5 done');
    expect(el.querySelector('[data-step=ai] [aria-label=Done]')).not.toBeNull();
    expect(el.querySelector('[data-step=tools]')?.textContent).toContain('tmux missing');
    expect(el.querySelector('[data-step=project]')?.textContent).toContain('None yet · 2 found on this Mac');
    expect(el.textContent).toContain('Continue onboarding');
    expect(el.textContent).toContain('Opens where you left off: Notifications.');
  });
  it('leads each item to its own page', async () => {
    const go = vi.fn();
    await mount(<OnboardingTab go={go} />);
    await act(async () => (el.querySelector('[data-step=tools] button') as HTMLElement).click());
    expect(go).toHaveBeenCalledWith('tools');
  });
});

describe('Settings > Notifications', () => {
  it('turns one kind off for every device, the others staying on by default', async () => {
    await mount(<NotificationsTab />);
    const sw = [...el.querySelectorAll('[role=switch]')];
    expect(sw.map((s) => s.getAttribute('aria-checked'))).toEqual(['true', 'true', 'true', 'true']);
    await act(async () => (sw[2] as HTMLElement).click());
    expect(set).toHaveBeenCalledWith({ changes: { push_stall: false } });
  });
  it('offers to allow the browser, and sends no test before it is allowed', async () => {
    await mount(<NotificationsTab />);
    expect(el.textContent).toContain('Allow notifications');
    expect([...el.querySelectorAll('button')].find((b) => b.textContent === 'Send test notification')?.disabled).toBe(true);
  });
});

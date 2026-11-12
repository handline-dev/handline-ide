import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.stubGlobal('Notification', { permission: 'default', requestPermission: async () => 'granted' });
const ok = (data: unknown) => ({ isSuccess: true, isPending: false, data });
const machine = { accounts: [] as object[], missing: [] as object[], repos: 0, agents: 0 };
vi.mock('@handline/app-data', () => ({
  useStatus: () => ok({ missing_tools: machine.missing, vendors: [] }),
  useAccounts: () => ok({ accounts: machine.accounts }),
  useRepos: () => ok({ repos: Array.from({ length: machine.repos }, () => ({})) }),
  useAgents: () => ok({ agents: Array.from({ length: machine.agents }, () => ({})) }),
  useFoundLogins: () => ({ isLoading: false, data: [] }),
  useWrite: () => ({ mutateAsync: vi.fn() }),
}));
import { closeOnboarding, resetOnboardingForTest, useOnboarding, useOnboardingOpen } from '#src/onboarding/data/progress.ts';

const seen = {} as { open: boolean; o: ReturnType<typeof useOnboarding> };
function Probe({ into }: { into: typeof seen }) {
  Object.assign(into, { open: useOnboardingOpen(), o: useOnboarding() });
  return null;
}
let root: Root;
const mount = async () => {
  const el = document.createElement('div');
  document.body.append(el);
  root = createRoot(el);
  await act(async () => root.render(<Probe into={seen} />));
};
beforeEach(() => {
  resetOnboardingForTest();
  Object.assign(machine, { accounts: [], missing: [{ tool: 'tmux' }], repos: 0, agents: 0 });
});
afterEach(() => act(() => root.unmount()));

describe('useOnboarding', () => {
  it('opens by itself the first time, then shows the sidebar card once closed', async () => {
    await mount();
    expect(seen.open).toBe(true);
    expect(seen.o.cardVisible).toBe(true);
    await act(async () => closeOnboarding());
    expect(seen.open).toBe(false);
    expect([seen.o.doneCount, seen.o.total, seen.o.cardVisible]).toEqual([0, 5, true]);
  });
  it('does not open again after the first time, and keeps the step it was left on', async () => {
    await mount();
    await act(async () => seen.o.goTo(2));
    await act(async () => closeOnboarding());
    act(() => root.unmount());
    await mount();
    expect(seen.open).toBe(false);
    expect(seen.o.resumeStep).toBe(2);
  });
  it('never opens on a machine that already has agents', async () => {
    machine.agents = 1;
    await mount();
    expect(seen.open).toBe(false);
    expect(seen.o.cardVisible).toBe(false);
  });
  it('hides the card when closed, and when everything is done', async () => {
    await mount();
    await act(async () => seen.o.hideCard());
    expect(seen.o.cardVisible).toBe(false);
    resetOnboardingForTest();
    Object.assign(machine, { accounts: [{ login_state: 'ok' }], missing: [], repos: 1, agents: 1 });
    await act(async () => seen.o.skipNotify());
    expect([seen.o.allDone, seen.o.cardVisible]).toEqual([true, false]);
  });
});

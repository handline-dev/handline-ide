import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RpcError, type Repo } from '@handline/client';

const mutate = vi.fn();
const write = { error: null as unknown };
vi.mock('@handline/app-data', () => ({
  useWrite: () => ({ mutate, reset: () => {}, isPending: false, error: write.error }),
  classify: (e: unknown) => e,
  sentence: String,
}));
import { ProjectSetup, setupSummary } from '#src/settings/ProjectSetup.tsx';

const repo = (setup: Partial<Repo['setup']> = {}): Repo =>
  ({ name: 'app', path: '/work/app', default_branch: 'main', setup: { script: '', copy: [], last: null, ...setup } }) as Repo;

let root: Root | undefined;
let el: HTMLElement;
const mount = async (ui: React.ReactNode) => {
  el = document.createElement('div');
  document.body.append(el);
  const r = (root = createRoot(el));
  await act(async () => r.render(ui));
};
afterEach(() => act(() => root?.unmount()));
const type = async (t: HTMLTextAreaElement, v: string) =>
  act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(t, v);
    t.dispatchEvent(new Event('input', { bubbles: true }));
  });

describe('Settings > Projects setup', () => {
  it('says in a few words what the setup does', () => {
    expect(setupSummary(repo().setup)).toBe('No setup');
    expect(setupSummary(repo({ copy: ['.env'], script: 'pnpm install' }).setup)).toBe('Setup copies .env, runs a script');
    expect(setupSummary(repo({ copy: ['.env', 'a.key'] }).setup)).toBe('Setup copies 2 paths');
  });
  it('saves the files one per line, blank lines dropped, and the script', async () => {
    await mount(<ProjectSetup repo={repo()} close={() => {}} />);
    const [copy, script] = [...el.querySelectorAll('textarea')];
    await type(copy!, '.env\n\n config/local.json \n');
    await type(script!, 'pnpm install\n');
    await act(async () => (el.querySelector('button[type=submit]') as HTMLElement).click());
    expect(mutate.mock.calls[0]![0]).toEqual({ path: '/work/app', copy: ['.env', 'config/local.json'], script: 'pnpm install' });
  });
  it('shows a refusal under the field it names', async () => {
    write.error = new RpcError('bad_request', 'copy path "/etc" is not a path inside the repo', { field: 'copy' });
    await mount(<ProjectSetup repo={repo()} close={() => {}} />);
    const alerts = [...el.querySelectorAll('[role=alert]')];
    expect(alerts.map((a) => a.closest('label')?.textContent?.startsWith('Files to copy'))).toEqual([true]);
    write.error = null;
  });
  it('shows a failed last run with its reason and the files the project lacked', async () => {
    const last = {
      at: new Date().toISOString(),
      worktree: '/w/app.x',
      ok: false,
      copied: [],
      missing: ['.env'],
      error: 'script exited 1',
      output: 'boom',
    };
    await mount(<ProjectSetup repo={repo({ script: 'false', last })} close={() => {}} />);
    expect(el.textContent).toContain('in app.x: script exited 1');
    expect(el.textContent).toContain('not in the project: .env');
    expect(el.textContent).toContain('Output');
  });
});

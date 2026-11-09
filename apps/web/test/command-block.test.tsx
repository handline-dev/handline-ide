import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';

const mutateAsync = vi.fn(async (_: unknown) => ({ terminal: '%7' }));
const openTab = vi.fn();
vi.mock('@handline/app-data', () => ({ useWrite: () => ({ mutateAsync, isPending: false }) }));
vi.mock('#src/layout/openTab.tsx', () => ({ useOpenTab: () => openTab, howOf: () => 'here' }));

const { CommandsOf } = await import('#src/chat/CommandBlock.tsx');
const { Markdown } = await import('#src/ui/Markdown.tsx');

const text = 'Run:\n```bash\ngh auth login\n```\nNot this:\n```json\n{}\n```\nThen:\n```console\n$ git push\nrejected\n```';
type Runs = Parameters<typeof CommandsOf.Provider>[0]['value'];

const render = async (runs?: NonNullable<Runs>['runs'], inChat = true) => {
  const el = document.createElement('div');
  document.body.append(el);
  const md = <Markdown text={text} />;
  await act(async () =>
    createRoot(el).render(inChat ? <CommandsOf.Provider value={{ agent: 'a', owner: 'r7', runs }}>{md}</CommandsOf.Provider> : md),
  );
  return el;
};
const buttons = (el: Element) => [...el.querySelectorAll('button')].map((b) => b.textContent).filter((t) => t !== '');

afterEach(() => {
  document.body.innerHTML = '';
  vi.clearAllMocks();
});

it('each command fence in a chat line can run; other code, and code outside a chat line, cannot', async () => {
  expect(buttons(await render()).filter((t) => t === 'Run in terminal')).toHaveLength(2);
  expect(buttons(await render(undefined, false))).not.toContain('Run in terminal');
});

it('a run is named by its fence among all the code fences, nested ones too, in a message as in a reply', async () => {
  const el = document.createElement('div');
  document.body.append(el);
  await act(async () =>
    createRoot(el).render(
      <CommandsOf.Provider value={{ agent: 'a', owner: '41' }}>
        <Markdown text={'```txt\nlog\n```\n- step:\n\n  ```sh\n  make\n  ```'} />
      </CommandsOf.Provider>,
    ),
  );
  await act(async () => [...el.querySelectorAll('button')].find((b) => b.textContent === 'Run in terminal')!.click());
  expect(mutateAsync).toHaveBeenCalledWith({ agent: 'a', command: 'make', block: '41:1' });
});

it('Run sends the command and its block, then opens its terminal beside the chat', async () => {
  const el = await render();
  const run = [...el.querySelectorAll('button')].filter((b) => b.textContent === 'Run in terminal')[1]!;
  await act(async () => run.click());
  expect(mutateAsync).toHaveBeenCalledWith({ agent: 'a', command: 'git push', block: 'r7:2' });
  expect(openTab).toHaveBeenCalledWith('shell:%7', 'side');
});

it('a block says how its last run went: running offers its terminal, an ended one runs again', async () => {
  const at = new Date().toISOString();
  const el = await render([
    { block: 'r7:0', terminal: '%3', started_at: at, state: 'running' },
    { block: 'r7:2', terminal: '%4', started_at: at, state: 'failed', exit: 1, seconds: 2 },
  ]);
  expect([...el.querySelectorAll('[role=status]')].map((s) => s.textContent)).toEqual(['Running · 0s', 'Failed · exit 1 · 2s']);
  expect(buttons(el)).toEqual(expect.arrayContaining(['Open terminal', 'Run again']));
  await act(async () => [...el.querySelectorAll('button')].find((b) => b.textContent === 'Open terminal')!.click());
  expect(openTab).toHaveBeenCalledWith('shell:%3', 'side');
});

it('an inline `! <command>` runs too, numbered from 1000, and leaves the fences their own numbers', async () => {
  const el = document.createElement('div');
  document.body.append(el);
  await act(async () =>
    createRoot(el).render(
      <CommandsOf.Provider value={{ agent: 'a', owner: 'r9' }}>
        <Markdown text={'Run `! gh auth login` yourself, then:\n```sh\nmake\n```\nand `! npm login`.'} />
      </CommandsOf.Provider>,
    ),
  );
  const runs = [...el.querySelectorAll('button')].filter((b) => b.textContent === 'Run in terminal');
  expect(runs).toHaveLength(3);
  for (const b of runs) await act(async () => b.click());
  expect(mutateAsync.mock.calls.map((c) => c[0])).toEqual([
    { agent: 'a', command: 'gh auth login', block: 'r9:1000' },
    { agent: 'a', command: 'make', block: 'r9:0' },
    { agent: 'a', command: 'npm login', block: 'r9:1001' },
  ]);
});

it('a `! ` span inside link text is plain words and takes no number', async () => {
  const el = document.createElement('div');
  document.body.append(el);
  await act(async () =>
    createRoot(el).render(
      <CommandsOf.Provider value={{ agent: 'a', owner: '5' }}>
        <Markdown text={'See [`! a`](https://example.com), then `! b`.'} />
      </CommandsOf.Provider>,
    ),
  );
  const runs = [...el.querySelectorAll('button')].filter((b) => b.textContent === 'Run in terminal');
  expect(runs).toHaveLength(1);
  await act(async () => runs[0]!.click());
  expect(mutateAsync).toHaveBeenLastCalledWith({ agent: 'a', command: 'b', block: '5:1000' });
});

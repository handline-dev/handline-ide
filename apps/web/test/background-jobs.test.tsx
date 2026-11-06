import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it } from 'vitest';
import { BackgroundItem, JobList, type Job } from '#src/chat/agent/BackgroundItem.tsx';

const since = new Date(Date.now() - 5 * 60_000).toISOString();
const jobs: Job[] = [
  { kind: 'shell', label: 'tools/check.sh', since },
  { kind: 'subagent', label: 'Review the diff', since, foreground: false },
];

const render = async (node: React.ReactNode) => {
  const el = document.createElement('div');
  await act(async () => createRoot(el).render(node));
  return el;
};

it('lists each job with its kind, label and how long it has run', async () => {
  const el = await render(<JobList jobs={jobs} />);
  const rows = [...el.querySelectorAll('li')];
  expect(rows.map((r) => r.textContent)).toEqual(['tools/check.sh5m', 'Review the diff5m']);
  expect(rows[1]!.querySelector('[aria-label="subagent"]')).not.toBeNull();
});

it('the status line counts the jobs, and says nothing when none run', async () => {
  expect((await render(<BackgroundItem jobs={jobs} />)).textContent).toBe('2 running');
  expect((await render(<BackgroundItem jobs={[]} />)).textContent).toBe('');
});

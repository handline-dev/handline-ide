// task.pr: a task's pull request as a seed gave it. The fake reads no GitHub: a task that names a pull request and was given no facts
// answers as a daemon whose gh is not logged in, so that state can be seen too.
import type { TaskPR } from '@handline/contract';
import { invalidParams } from '#src/errors.ts';

interface Tasks {
  find(id: unknown): { id: number; pr?: number | null; pr_repo?: string | null; pull?: TaskPR };
}

export function prOf(p: Record<string, unknown>, tasks: Tasks): TaskPR | null {
  if (!Number.isInteger(p.task)) throw invalidParams('task is a task number', 'task');
  const t = tasks.find(p.task);
  if (t.pr == null) return null;
  if (t.pull) return structuredClone(t.pull);
  const repo = t.pr_repo ?? null;
  return {
    number: t.pr,
    repo,
    url: repo ? `https://github.com/${repo}/pull/${t.pr}` : null,
    state: null,
    draft: false,
    base: null,
    review: null,
    failing: [],
    waiting: 0,
    passing: 0,
    head_sha: null,
    merge_sha: null,
    closes_task: false,
    unavailable: { reason: 'gh_logged_out', text: 'gh is not logged in: run gh auth login' },
  };
}

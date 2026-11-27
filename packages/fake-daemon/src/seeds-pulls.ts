// Pull requests on tasks and branches, so a task's card and a branch's chip can be seen in each state.
import type { TaskPR } from '@handline/contract';
import type { FakeDaemon } from '#src/daemon.ts';

const REPO = 'handline-dev/handline';

const pull = (number: number, over: Partial<TaskPR>): TaskPR => ({
  number,
  repo: REPO,
  url: `https://github.com/${REPO}/pull/${number}`,
  state: 'open',
  draft: false,
  base: 'main',
  review: null,
  failing: [],
  waiting: 0,
  passing: 0,
  head_sha: '1f0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e',
  merge_sha: null,
  closes_task: false,
  unavailable: null,
  ...over,
});

export const PULL_SEEDS: Record<string, (d: FakeDaemon) => void> = {
  /** The terminal pane task has an open pull request with two failed checks and changes asked for, whose merge closes it, and its
   * holder's branch shows it; the closed fixtures task has a merged one, on its holder's branch; the audit task names one this
   * daemon's gh cannot read. */
  'pull-requests': (d) => {
    const link = (task: number, number: number, facts?: Partial<TaskPR>) =>
      Object.assign(d.tasks.find(task), { pr: number, pr_repo: REPO, ...(facts ? { pull: pull(number, facts) } : {}) });
    /** The branch of the agent that holds a task shows the task's pull request. */
    const branch = (task: number, number: number, verdict: 'checks_failing' | 'merged', failing?: number) => {
      const row = d.admin.worktrees.rows.find((w) => w.owner === d.tasks.find(task).to);
      if (row) row.pr = { number, url: `https://github.com/${REPO}/pull/${number}`, verdict, ...(failing ? { failing } : {}) };
    };
    link(1688, 901, { review: 'changes_requested', failing: ['build', 'lint'], waiting: 1, passing: 5, closes_task: true });
    link(1650, 880, { state: 'merged', review: 'approved', passing: 8, merge_sha: '984c6570a1b2c3d4e5f60718293a4b5c6d7e8f90' });
    link(1690, 905);
    branch(1688, 901, 'checks_failing', 2);
    branch(1650, 880, 'merged');
  },
  /** Pull requests off GitHub: a GitLab merge request read through glab, open with its pipeline failed and an approval owed; and a
   * repository on a host nothing here reads, said plainly. */
  'other-hosts': (d) => {
    const repo = 'gitlab.com/handline-dev/handline';
    Object.assign(d.tasks.find(1684), {
      pr: 12,
      pr_repo: repo,
      pull: pull(12, {
        repo,
        host: 'GitLab',
        url: `https://${repo}/-/merge_requests/12`,
        review: 'review_required',
        failing: ['pipeline'],
      }),
    });
    const other = 'git.example.com/team/app';
    Object.assign(d.tasks.find(1685), {
      pr: 4,
      pr_repo: other,
      pull: pull(4, {
        repo: other,
        host: 'git.example.com',
        url: null,
        state: null,
        base: null,
        head_sha: null,
        unavailable: { reason: 'unread_host', text: 'Pull requests on git.example.com are not read yet; this repository lands by push.' },
      }),
    });
  },
};

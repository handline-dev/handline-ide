// A task's pull request, on the task page: where it stands on GitHub, the checks that failed by name, the review's decision, and a
// link out. Nothing for a task that names none. What could not be read is said in a line, never left as an empty box.
import { pullChecks, pullReview, pullState, pullUnavailableText } from '@handline/app-core';
import { useRead } from '@handline/app-data';
import type { TaskDetail } from '@handline/client';
import { ExternalLink, GitPullRequest } from 'lucide-react';
import { PULL_TONE } from '#src/ui/PullChip.tsx';

/** GitHub is asked again this often while the page is open: the daemon keeps a read about that long. */
const AGAIN_MS = 60_000;

export function TaskPull({ t }: { t: TaskDetail }) {
  const has = t.pr != null;
  const pr = useRead('task.pr', { task: t.id }, { enabled: has, refetchMs: AGAIN_MS }).data;
  if (!has) return null;
  const state = pr ? pullState(pr) : null;
  const review = pr ? pullReview(pr) : null;
  const checks = pr ? pullChecks(pr) : [];
  const where = pr?.repo ?? t.pr_repo;
  return (
    <section aria-label="Pull request" className="flex items-start gap-2 rounded-md border border-border px-3 py-2 text-sm">
      <GitPullRequest size={14} className="mt-0.5 shrink-0 text-muted" />
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="font-medium text-fg">PR #{t.pr}</span>
          {where && <span className="min-w-0 truncate text-muted">{where}</span>}
          {state && <span className={`font-medium ${PULL_TONE[state.tone]}`}>{state.text}</span>}
          {pr?.state === 'merged' && pr.merge_sha && <span className="font-mono text-xs text-muted">{pr.merge_sha.slice(0, 7)}</span>}
          <span className="flex-1" />
          {pr?.url && (
            <a
              href={pr.url}
              target="_blank"
              rel="noreferrer"
              className="flex shrink-0 items-center gap-1 rounded-sm text-accent outline-none hover:underline focus-visible:ring-2 focus-visible:ring-accent"
            >
              Open on GitHub
              <ExternalLink size={12} />
            </a>
          )}
        </div>
        {review && <span className={PULL_TONE[review.tone]}>{review.text}</span>}
        {checks.length > 0 && (
          <p className="min-w-0 break-words text-muted">
            {checks.map((c, i) => (
              <span key={c.text}>
                {i > 0 && ' · '}
                <span className={PULL_TONE[c.tone]}>{c.text}</span>
              </span>
            ))}
          </p>
        )}
        {pr?.unavailable && <p className="text-muted">{pullUnavailableText(pr.unavailable)}</p>}
        {pr === undefined && <p className="text-muted">Reading it from GitHub…</p>}
      </div>
    </section>
  );
}

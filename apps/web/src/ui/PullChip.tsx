// A branch's pull request as a chip: its number and what holds it, a link to its page. And the tones every line about a pull request
// is drawn in, shared with the task's card.
import { pullChip, pullUnavailableText, type PullTone } from '@handline/app-core';
import type { PullUnavailable, WorktreePR } from '@handline/client';
import { GitPullRequest } from 'lucide-react';

export const PULL_TONE: Record<PullTone, string> = {
  good: 'text-success',
  bad: 'text-danger',
  wait: 'text-warning',
  quiet: 'text-muted',
};

/** The pull request of a branch (worktrees.list's `pr`): `#901 2 checks failing`. A click opens it on GitHub. */
export function PullChip({ pr }: { pr: WorktreePR }) {
  const line = pullChip(pr);
  return (
    <a
      href={pr.url}
      target="_blank"
      rel="noreferrer"
      title={`Pull request #${pr.number}: ${line.text}. Opens on GitHub.`}
      className="-mx-1 flex min-w-0 items-center gap-1 rounded-md px-1 py-0.5 text-xs outline-none hover:bg-hover/30 focus-visible:ring-2 focus-visible:ring-accent"
    >
      <GitPullRequest size={13} className={`shrink-0 ${PULL_TONE[line.tone]}`} />
      <span className="shrink-0 text-fg tabular-nums">#{pr.number}</span>
      {/* In a narrow row the words give way before the number does; the mark keeps their tone. */}
      <span className={`truncate ${PULL_TONE[line.tone]}`}>{line.text}</span>
    </a>
  );
}

/** Why no branch shows a pull request, when it is the daemon's gh that cannot be used; nothing when it can. */
export function PullsUnavailable({ why, className = '' }: { why?: PullUnavailable | null; className?: string }) {
  if (!why) return null;
  return <p className={`text-xs text-muted ${className}`}>{pullUnavailableText(why, 'Pull requests are not shown')}</p>;
}

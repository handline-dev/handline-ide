// What a pull request's facts say in words: a task's card (task.pr) and a branch's chip (worktrees.list's pr). The daemon reads the
// facts; the words and their tone are decided here, once, for every screen.
import type { PullUnavailable, TaskPR, WorktreePR } from '@handline/client';

/** How a line about a pull request reads: fine, something to fix, something to wait for, or neither. */
export type PullTone = 'good' | 'bad' | 'wait' | 'quiet';

export interface PullLine {
  text: string;
  tone: PullTone;
}

const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/** A branch's pull request in a few words: what holds it, or that nothing does. */
export function pullChip(pr: WorktreePR): PullLine {
  switch (pr.verdict) {
    case 'merged':
      return { text: 'merged', tone: 'good' };
    case 'draft':
      return { text: 'draft', tone: 'quiet' };
    case 'checks_failing':
      return { text: `${count(pr.failing ?? 1, 'check', 'checks')} failing`, tone: 'bad' };
    case 'changes_requested':
      return { text: 'changes requested', tone: 'bad' };
    case 'waiting_review':
      return { text: 'waiting for review', tone: 'wait' };
    default:
      return { text: 'ready', tone: 'good' };
  }
}

/** Where a task's pull request stands: merged, closed, a draft, or open. null when it could not be read. */
export function pullState(pr: TaskPR): PullLine | null {
  switch (pr.state) {
    case 'merged':
      return { text: 'Merged', tone: 'good' };
    case 'closed':
      return { text: 'Closed without merging', tone: 'bad' };
    case 'open':
      return pr.draft ? { text: 'Draft', tone: 'quiet' } : { text: 'Open', tone: 'quiet' };
    default:
      return null;
  }
}

/** GitHub is asked about an open pull request again this often while its card is on screen: the daemon keeps a read about that long. */
export const PULL_AGAIN_MS = 60_000;

/** When a task's pull request is asked about again: each minute while it can still change (open, or not read yet), and never once it
 * is merged or closed, which nothing follows. */
export function pullAgainMs(pr: Pick<TaskPR, 'state'> | undefined): number | false {
  return pr?.state === 'merged' || pr?.state === 'closed' ? false : PULL_AGAIN_MS;
}

/** Its checks, as the parts of one line: the ones that failed by name, then how many still run and how many passed. Empty when it
 * has none. Only the failed part reads as something to fix; passed is good only when nothing else is said. */
export function pullChecks(pr: TaskPR): PullLine[] {
  const parts: PullLine[] = [];
  if (pr.failing.length)
    parts.push({ text: `${count(pr.failing.length, 'check', 'checks')} failing: ${pr.failing.join(', ')}`, tone: 'bad' });
  if (pr.waiting) parts.push({ text: `${pr.waiting} running`, tone: 'wait' });
  if (pr.passing) parts.push({ text: `${pr.passing} passed`, tone: parts.length ? 'quiet' : 'good' });
  return parts;
}

/** The review's decision; null when no review is asked for, and once the pull request is merged or closed. */
export function pullReview(pr: TaskPR): PullLine | null {
  if (pr.state !== 'open') return null;
  switch (pr.review) {
    case 'approved':
      return { text: 'Approved', tone: 'good' };
    case 'changes_requested':
      return { text: 'Changes requested', tone: 'bad' };
    case 'review_required':
      return { text: 'Waiting for review', tone: 'wait' };
    default:
      return null;
  }
}

/** Why a pull request's facts are not shown, as a line a person can act on, after `what` ("Not shown" on a card, where the pull request
 * is named above it). The daemon's machine is named: it may not be this screen's. */
export function pullUnavailableText(u: PullUnavailable, what = 'Not shown'): string {
  switch (u.reason) {
    case 'gh_missing':
      return `${what}: the GitHub CLI (gh) is not installed on the machine Handline runs on.`;
    case 'gh_logged_out':
      return `${what}: gh is not logged in on the machine Handline runs on. Run gh auth login there.`;
    case 'gone':
      return `${what}: GitHub has no such pull request, or the gh login on the machine Handline runs on cannot see it.`;
    case 'no_repo':
    case 'unread_host':
      return u.text;
    default:
      return `${what}: ${u.text}`;
  }
}

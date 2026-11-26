import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { TaskPR } from '@handline/client';
import { pullAgainMs, pullChecks, pullChip, pullReview, pullState, pullUnavailableText } from '#src/index.ts';

const pr = (over: Partial<TaskPR> = {}): TaskPR => ({
  number: 7,
  repo: 'acme/app',
  url: 'https://github.com/acme/app/pull/7',
  state: 'open',
  draft: false,
  base: 'main',
  review: null,
  failing: [],
  waiting: 0,
  passing: 0,
  head_sha: '1f0e9d8c',
  merge_sha: null,
  unavailable: null,
  ...over,
});

test("a branch's chip says what holds its pull request, and counts one failing check as one", () => {
  const chip = (verdict: string, failing?: number) =>
    pullChip({ number: 7, url: 'https://github.com/acme/app/pull/7', verdict, failing } as never);
  assert.deepEqual(chip('checks_failing', 2), { text: '2 checks failing', tone: 'bad' });
  assert.deepEqual(chip('checks_failing', 1), { text: '1 check failing', tone: 'bad' });
  assert.deepEqual(chip('changes_requested'), { text: 'changes requested', tone: 'bad' });
  assert.deepEqual(chip('waiting_review'), { text: 'waiting for review', tone: 'wait' });
  assert.deepEqual(chip('draft'), { text: 'draft', tone: 'quiet' });
  assert.deepEqual(chip('merged'), { text: 'merged', tone: 'good' });
  assert.deepEqual(chip('ready'), { text: 'ready', tone: 'good' });
});

test('where a pull request stands: a draft is said before open, and one not read has no state', () => {
  assert.deepEqual(pullState(pr()), { text: 'Open', tone: 'quiet' });
  assert.deepEqual(pullState(pr({ draft: true })), { text: 'Draft', tone: 'quiet' });
  assert.deepEqual(pullState(pr({ state: 'merged' })), { text: 'Merged', tone: 'good' });
  assert.deepEqual(pullState(pr({ state: 'closed' })), { text: 'Closed without merging', tone: 'bad' });
  assert.equal(pullState(pr({ state: null })), null);
});

test('a pull request is asked about again each minute while it can change, and never once it is merged or closed', () => {
  assert.equal(pullAgainMs(undefined), 60_000);
  assert.equal(pullAgainMs(pr()), 60_000);
  assert.equal(pullAgainMs(pr({ state: null, unavailable: { reason: 'failed', text: 'gh pr: HTTP 502' } })), 60_000);
  assert.equal(pullAgainMs(pr({ state: 'merged' })), false);
  assert.equal(pullAgainMs(pr({ state: 'closed' })), false);
});

test('the checks line names the failed ones, then counts the running and the passed; only the failed part reads as bad', () => {
  assert.deepEqual(pullChecks(pr({ failing: ['build', 'lint'], waiting: 1, passing: 5 })), [
    { text: '2 checks failing: build, lint', tone: 'bad' },
    { text: '1 running', tone: 'wait' },
    { text: '5 passed', tone: 'quiet' },
  ]);
  assert.deepEqual(pullChecks(pr({ failing: ['build'] })), [{ text: '1 check failing: build', tone: 'bad' }]);
  assert.deepEqual(pullChecks(pr({ waiting: 2, passing: 3 })), [
    { text: '2 running', tone: 'wait' },
    { text: '3 passed', tone: 'quiet' },
  ]);
  assert.deepEqual(pullChecks(pr({ passing: 6 })), [{ text: '6 passed', tone: 'good' }]);
  assert.deepEqual(pullChecks(pr()), []);
});

test("the review's decision shows while the pull request is open, and not after", () => {
  assert.deepEqual(pullReview(pr({ review: 'changes_requested' })), { text: 'Changes requested', tone: 'bad' });
  assert.deepEqual(pullReview(pr({ review: 'review_required' })), { text: 'Waiting for review', tone: 'wait' });
  assert.deepEqual(pullReview(pr({ review: 'approved' })), { text: 'Approved', tone: 'good' });
  assert.equal(pullReview(pr()), null);
  assert.equal(pullReview(pr({ state: 'merged', review: 'approved' })), null);
});

test('why a pull request is not shown names the machine whose gh it is, and what to run', () => {
  assert.match(
    pullUnavailableText({ reason: 'gh_logged_out', text: 'x' }),
    /gh is not logged in on the machine Handline runs on\. Run gh auth login/,
  );
  assert.match(pullUnavailableText({ reason: 'gh_missing', text: 'x' }), /\(gh\) is not installed on the machine Handline runs on/);
  assert.match(pullUnavailableText({ reason: 'gone', text: 'x' }), /no such pull request, or the gh login .* cannot see it/);
  assert.equal(pullUnavailableText({ reason: 'no_repo', text: 'Name it with: hl task edit' }), 'Name it with: hl task edit');
  assert.equal(pullUnavailableText({ reason: 'failed', text: 'gh pr: HTTP 502' }), 'Not shown: gh pr: HTTP 502');
  assert.equal(
    pullUnavailableText({ reason: 'gh_missing', text: 'x' }, 'Pull requests are not shown'),
    'Pull requests are not shown: the GitHub CLI (gh) is not installed on the machine Handline runs on.',
  );
});

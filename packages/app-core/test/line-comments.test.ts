import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Task } from '@handline/client';
import { diffFiles } from '#src/diff-lines.ts';
import {
  anchorOf,
  draftsKey,
  lineCommentsSummary,
  lineCommentsText,
  lineLabel,
  movedDrafts,
  placedRow,
  redoLabel,
  rowOf,
  taskCommentRoute,
  type LineComment,
} from '#src/line-comments.ts';

const [file] = diffFiles(
  [
    'diff --git a/q.sql b/q.sql',
    '--- a/q.sql',
    '+++ b/q.sql',
    '@@ -4,4 +4,3 @@',
    ' select 1;',
    '--- old',
    '-gone',
    ' select 2;',
    '+++ new',
    '',
  ].join('\n'),
);
// rows: 0 hunk, 1 ctx 4/4, 2 del 5, 3 del 6, 4 ctx 7/5, 5 add 6

test('an anchor is the new-side lines, the old side only when every picked line was deleted', () => {
  assert.deepEqual(anchorOf(file.rows, 5, 5), {
    lines: { side: 'new', from: 6, to: 6 },
    at: { side: 'new', no: 6, line: '+++ new' },
    quote: ['+++ new'],
  });
  // either order, and a hunk header inside the range is not a line
  assert.deepEqual(anchorOf(file.rows, 4, 0), {
    lines: { side: 'new', from: 4, to: 5 },
    at: { side: 'new', no: 5, line: ' select 2;' },
    quote: [' select 1;', '--- old', '-gone', ' select 2;'],
  });
  assert.deepEqual(anchorOf(file.rows, 2, 3), {
    lines: { side: 'old', from: 5, to: 6 },
    at: { side: 'old', no: 6, line: '-gone' },
    quote: ['--- old', '-gone'],
  });
  // a range that ends on a deleted line is drawn under it and still named by the new side
  assert.deepEqual(anchorOf(file.rows, 1, 2)?.at, { side: 'old', no: 5, line: '--- old' });
  assert.deepEqual(anchorOf(file.rows, 1, 2)?.lines, { side: 'new', from: 4, to: 4 });
  assert.equal(anchorOf(file.rows, 0, 0), null);
});

test('a comment finds its line again by side and number', () => {
  assert.equal(rowOf(file.rows, { side: 'old', no: 6 }), 3);
  assert.equal(rowOf(file.rows, { side: 'new', no: 5 }), 4);
  assert.equal(rowOf(file.rows, { side: 'new', no: 99 }), -1);
  assert.equal(lineLabel({ lines: { side: 'new', from: 4, to: 5 } }), '4-5');
  assert.equal(lineLabel({ lines: { side: 'old', from: 6, to: 6 } }), 'old 6');
  assert.equal(lineLabel({}), 'File');
  assert.equal(lineLabel({ lines: { side: 'new', from: 4, to: 5 }, moved: true }), 'was 4-5');
});

test('drafts are kept apart per cluster: a joined cluster has its own task of the same number and agent of the same name', () => {
  assert.equal(draftsKey('', 'task:12'), 'linecomments/task:12');
  assert.equal(draftsKey('a1b2c3d4', 'task:12'), 'linecomments/@a1b2c3d4/task:12');
  assert.equal(draftsKey('a1b2c3d4', 'agent:team.builder'), 'linecomments/@a1b2c3d4/agent:team.builder');
  assert.notEqual(draftsKey('a1b2c3d4', 'agent:x'), draftsKey('e5f6a7b8', 'agent:x'));
});

const diffOf = (...body: string[]) => diffFiles(['diff --git a/q.sql b/q.sql', '--- a/q.sql', '+++ b/q.sql', ...body, ''].join('\n'));

test('a draft stays on its line only while a line of that number still reads as it did', () => {
  const draft = (id: string, a: number, b = a): LineComment => ({
    id,
    path: 'q.sql',
    view: 'head',
    text: 'why?',
    ...anchorOf(file.rows, a, b)!,
  });
  const drafts = [draft('new6', 5), draft('ctx5', 4), draft('old6', 2, 3), { ...draft('other', 5), view: 'base' }];
  assert.equal(placedRow(file.rows, drafts[0].at!), 5);
  assert.deepEqual(movedDrafts(drafts, [file], 'head'), []);
  // a line added above: new line 6 is another line now, new line 5 too; the deleted lines kept their old numbers
  const shifted = diffOf('@@ -4,4 +4,4 @@', '+first', ' select 1;', '--- old', '-gone', ' select 2;', '+++ new');
  assert.equal(placedRow(shifted[0].rows, drafts[0].at!), -1);
  assert.deepEqual(movedDrafts(drafts, shifted, 'head'), ['new6', 'ctx5']);
  // the same number with other words is another line
  const reworded = diffOf('@@ -4,4 +4,3 @@', ' select 1;', '--- old', '-gone', ' select 2;', '+++ newer');
  assert.deepEqual(movedDrafts(drafts, reworded, 'head'), ['new6']);
  // the file left the diff, or its numbers do not hold any more: every draft on a line of it has moved
  assert.deepEqual(movedDrafts(drafts, [], 'head'), ['new6', 'ctx5', 'old6']);
  assert.deepEqual(movedDrafts(drafts, [{ ...file, cut: true }], 'head'), ['new6', 'ctx5', 'old6']);
  // a comment on the whole file has no line to lose, one already marked is not named again, and another diff's drafts are not judged
  const rest = [
    { ...draft('file', 5), at: undefined, lines: undefined },
    { ...draft('was', 5), moved: true as const },
  ];
  assert.deepEqual(movedDrafts(rest, [], 'head'), []);
  assert.deepEqual(movedDrafts(drafts, [], 'base'), ['other']);
});

const c = (over: Partial<LineComment>): LineComment => ({ id: 'x', path: 'go/x/y.go', view: 'all', quote: [], text: 'why?', ...over });

test('the first line is whole on its own: the count, where to read the rest, then the files', () => {
  const three = [c({ lines: { side: 'new', from: 42, to: 42 } }), c({ path: 'apps/z.tsx' }), c({ lines: { side: 'old', from: 7, to: 9 } })];
  assert.equal(lineCommentsSummary(three, 'hl task show'), '3 line comments, read them with hl task show: go/x/y.go, apps/z.tsx');
  assert.equal(lineCommentsSummary(three), '3 line comments on go/x/y.go, apps/z.tsx');
  assert.equal(lineCommentsSummary([c({ path: 'a' })]), '1 file comment on a');
  assert.equal(lineCommentsSummary([c({ path: 'a' })], 'hl task show'), '1 file comment, read it with hl task show: a');
  const many = ['a', 'b', 'c', 'd', 'e'].map((path) => c({ path, lines: { side: 'new', from: 1, to: 1 } }));
  assert.equal(lineCommentsSummary(many), '5 line comments on a, b, c and 2 more');
});

test('the text is the summary, then a block per comment: where, the lines quoted, the comment', () => {
  const text = lineCommentsText(
    [
      c({ lines: { side: 'new', from: 42, to: 43 }, quote: ['+\treturn nil', ' }'], text: ' Wrap this error. ', commit: 'abc1234def' }),
      c({ path: 'apps/z.tsx', text: 'Split this file.' }),
      c({ lines: { side: 'old', from: 7, to: 7 }, quote: ['--- note'], text: 'Why did this go?' }),
      c({ text: 'Rename the package.' }),
    ],
    { readWith: 'hl task show', lead: ' Not yet. ' },
  );
  assert.equal(
    text,
    [
      '4 line comments, read them with hl task show: go/x/y.go, apps/z.tsx',
      '',
      'Not yet.',
      '',
      '`go/x/y.go` (the whole file)',
      'Rename the package.',
      '',
      '`go/x/y.go:7` (deleted lines, numbered as before the change)',
      '```diff',
      '--- note',
      '```',
      'Why did this go?',
      '',
      '`go/x/y.go:42-43` in commit abc1234',
      '```diff',
      '+\treturn nil',
      ' }',
      '```',
      'Wrap this error.',
      '',
      '`apps/z.tsx` (the whole file)',
      'Split this file.',
    ].join('\n'),
  );
});

test('a long quote is bounded, and its fence outlasts any backticks in it', () => {
  const quote = ['+const s = ```x```;', `+${'w'.repeat(300)}`, ...Array.from({ length: 10 }, (_, i) => ` line ${i}`)];
  const lines = lineCommentsText([c({ lines: { side: 'new', from: 1, to: 12 }, quote })]).split('\n');
  assert.equal(lines[3], '````diff');
  assert.equal(lines[5], `+${'w'.repeat(159)}…`);
  assert.deepEqual(lines.slice(9), [' line 3', '… 6 more lines', '````', 'why?']);
});

test('a path or a quoted line names no one: only the words the person wrote can mention an agent', () => {
  const mention = /(^|[^A-Za-z0-9_.@<-])@[A-Za-z0-9]|<@[a-z]/m; // what the daemon reads as a name
  const quote = ["+import { x } from '@scope/pkg';", '+ask(`<@abc12>`, "@lead")'];
  const text = lineCommentsText([
    c({ path: 'packages/@scope/pkg/x.ts', lines: { side: 'new', from: 1, to: 2 }, quote, text: 'ask @lead' }),
  ]);
  const mine = text.split('\n').at(-1)!;
  assert.equal(mine, 'ask @lead');
  assert.equal(mention.test(text.slice(0, -mine.length)), false);
  assert.equal(text.replaceAll(String.fromCharCode(0x200b), '').includes("+import { x } from '@scope/pkg';"), true);
});

test('a comment whose line moved is sent on its file, with the lines as they stood and no number', () => {
  const text = lineCommentsText([
    c({
      lines: { side: 'new', from: 42, to: 43 },
      quote: ['+\treturn nil', ' }'],
      text: 'Wrap this error.',
      moved: true,
      commit: 'abc1234def',
    }),
    c({ lines: { side: 'new', from: 7, to: 7 }, quote: ['+x'], text: 'Still here.' }),
  ]);
  assert.equal(
    text,
    [
      '2 line comments on go/x/y.go',
      '',
      '`go/x/y.go` in commit abc1234 (on lines that have moved or changed since; they are quoted as they stood)',
      '```diff',
      '+\treturn nil',
      ' }',
      '```',
      'Wrap this error.',
      '',
      '`go/x/y.go:7`',
      '```diff',
      '+x',
      '```',
      'Still here.',
    ].join('\n'),
  );
  assert.equal(/42|43/.test(text), false);
});

test('a path is said on one line and inside one code span, whatever the file is called', () => {
  const odd = 'docs/two\nlines `and` ticks\u2028\ttab.md';
  const text = lineCommentsText([c({ path: odd, lines: { side: 'new', from: 3, to: 3 }, quote: ['+x'] }), c({ path: odd })], {
    readWith: 'hl task show',
  });
  const lines = text.split('\n');
  assert.equal(lines[0], '2 line comments, read them with hl task show: docs/two lines  and  ticks  tab.md');
  assert.equal(lines[2], '`docs/two lines  and  ticks  tab.md` (the whole file)');
  assert.equal(lines[5], '`docs/two lines  and  ticks  tab.md:3`');
});

const turn = (agent: string, by: string) =>
  ({ agent, ask: '', by, since: '', on_pass: null, evidence: null }) as NonNullable<Task['turns']>[number];

test('where the comments go: a done task is reopened, an open one commented, and an answer to my turn says so', () => {
  const open = { to: 'team.builder', state: 'doing', acting: 'team.builder', turns: [] };
  assert.deepEqual(taskCommentRoute({ ...open, state: 'done' }, 'me', 3), { via: 'reopen', label: 'Reopen with 3 comments' });
  assert.deepEqual(taskCommentRoute(open, 'me', 3), { via: 'comment', label: 'Comment' });
  const mine = { ...open, acting: 'human', turns: [turn('human', 'team.builder')] };
  assert.deepEqual(taskCommentRoute(mine, 'me', 1), { via: 'comment', label: 'Answer with 1 comment', answers: 'team.builder' });
  // Ready for the founder: a comment asks and the turn stays, so it is a plain comment; Redo is its own button
  assert.deepEqual(taskCommentRoute({ ...mine, review: { ready: true } }, 'me', 2), { via: 'comment', label: 'Comment' });
  assert.equal(redoLabel(2), 'Redo with 2 comments');
});

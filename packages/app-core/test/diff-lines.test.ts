import { test } from 'node:test';
import assert from 'node:assert/strict';
import { diffFiles, diffSections, lineNumbersHold, unquotePath, type DiffRow } from '#src/diff-lines.ts';

const text = (...lines: string[]) => lines.join('\n') + '\n';
/** A row as `kind old new text`, a missing number as `-`. */
const said = (r: DiffRow) => `${r.kind} ${r.oldNo ?? '-'} ${r.newNo ?? '-'} ${r.text}`;

test('several hunks: each line is numbered from its hunk header', () => {
  const [f, ...rest] = diffFiles(
    text(
      'diff --git a/multi.txt b/multi.txt',
      'index 1111111..2222222 100644',
      '--- a/multi.txt',
      '+++ b/multi.txt',
      '@@ -1,3 +1,3 @@',
      ' l1',
      '-l2',
      '+L2',
      ' l3',
      '@@ -17,4 +17,5 @@ l16',
      ' l17',
      ' l18',
      '+new',
      ' l19',
      ' l20',
    ),
  );
  assert.equal(rest.length, 0);
  assert.equal(f.path, 'multi.txt');
  assert.equal(f.status, 'modified');
  assert.equal(lineNumbersHold(f), true);
  assert.deepEqual(f.rows.map(said), [
    'hunk - - @@ -1,3 +1,3 @@',
    'ctx 1 1 l1',
    'del 2 - l2',
    'add - 2 L2',
    'ctx 3 3 l3',
    'hunk - - @@ -17,4 +17,5 @@ l16',
    'ctx 17 17 l17',
    'ctx 18 18 l18',
    'add - 19 new',
    'ctx 19 20 l19',
    'ctx 20 21 l20',
  ]);
});

test('a new file and a deleted file: the path is the side that exists, a count left out is one line', () => {
  const [added, deleted] = diffFiles(
    text(
      'diff --git a/added.txt b/added.txt',
      'new file mode 100644',
      'index 0000000..3333333',
      '--- /dev/null',
      '+++ b/added.txt',
      '@@ -0,0 +1 @@',
      '+fresh',
      'diff --git a/dead.txt b/dead.txt',
      'deleted file mode 100644',
      'index 4444444..0000000',
      '--- a/dead.txt',
      '+++ /dev/null',
      '@@ -1,2 +0,0 @@',
      '-gone',
      '-too',
    ),
  );
  assert.deepEqual([added.path, added.status, added.rows.slice(1).map(said)], ['added.txt', 'added', ['add - 1 fresh']]);
  assert.deepEqual(
    [deleted.path, deleted.status, deleted.rows.slice(1).map(said)],
    ['dead.txt', 'deleted', ['del 1 - gone', 'del 2 - too']],
  );
  assert.equal(lineNumbersHold(added) && lineNumbersHold(deleted), true);
});

test('inside a hunk a deleted `-- x` and an added `++ x` are content, and the no-newline marker is not a line', () => {
  const [f] = diffFiles(
    text(
      'diff --git a/q.sql b/q.sql',
      'index a47746a..e71a6dc 100644',
      '--- a/q.sql',
      '+++ b/q.sql',
      '@@ -1,4 +1,4 @@',
      ' select 1;',
      '--- old note',
      ' select 2;',
      '-a',
      '\\ No newline at end of file',
      '+++ plus',
      '+b',
      '\\ No newline at end of file',
    ),
  );
  assert.equal(f.path, 'q.sql');
  assert.deepEqual(f.rows.slice(1).map(said), [
    'ctx 1 1 select 1;',
    'del 2 - -- old note',
    'ctx 3 2 select 2;',
    'del 4 - a',
    'meta - - \\ No newline at end of file',
    'add - 3 ++ plus',
    'add - 4 b',
    'meta - - \\ No newline at end of file',
  ]);
  assert.equal(lineNumbersHold(f), true);
});

test('a rename keeps both paths, with or without changed lines', () => {
  const [moved, edited] = diffFiles(
    text(
      'diff --git a/docs/old name.md b/docs/new name.md',
      'similarity index 100%',
      'rename from docs/old name.md',
      'rename to docs/new name.md',
      'diff --git a/old-name.txt b/new-name.txt',
      'similarity index 80%',
      'rename from old-name.txt',
      'rename to new-name.txt',
      'index 5555555..6666666 100644',
      '--- a/old-name.txt',
      '+++ b/new-name.txt',
      '@@ -4,2 +4,2 @@ three',
      ' four',
      '-five',
      '+FIVE',
    ),
  );
  assert.deepEqual([moved.path, moved.oldPath, moved.status, moved.rows], ['docs/new name.md', 'docs/old name.md', 'renamed', []]);
  assert.deepEqual([edited.path, edited.oldPath, edited.status], ['new-name.txt', 'old-name.txt', 'renamed']);
  assert.deepEqual(edited.rows.slice(1).map(said), ['ctx 4 4 four', 'del 5 - five', 'add - 5 FIVE']);
});

test('a binary file has a note and no numbered line; a note in place of a diff is kept', () => {
  const [bin, big] = diffFiles(
    text(
      'diff --git a/bin.dat b/bin.dat',
      'index 7777777..8888888 100644',
      'Binary files a/bin.dat and b/bin.dat differ',
      'diff --git a/dump.txt b/dump.txt',
      'new file mode 100644',
      '(new file, 400000 bytes: not shown)',
    ),
  );
  assert.equal(bin.binary, true);
  assert.deepEqual(bin.rows.map(said), ['meta - - Binary files a/bin.dat and b/bin.dat differ']);
  assert.deepEqual([big.status, big.binary, big.rows.map(said)], ['added', false, ['meta - - (new file, 400000 bytes: not shown)']]);
});

test('names with a space, in quotes, or both: the path is read whole', () => {
  const korean = '"b/\\355\\225\\234 two.sql"';
  const files = diffFiles(
    text(
      'diff --git a/with space.txt b/with space.txt',
      '--- a/with space.txt\t',
      '+++ b/with space.txt\t',
      '@@ -1 +1 @@',
      '-x',
      '+y',
      `diff --git a/q one.sql ${korean}`,
      '--- a/q one.sql\t',
      `+++ ${korean}\t`,
      '@@ -1 +1 @@',
      '-x',
      '+y',
      'diff --git "a/tab\\there.txt" "b/tab\\there.txt"',
      'new file mode 100644',
      'diff --git a/a b/b b/a b/b',
      'old mode 100644',
      'new mode 100755',
    ),
  );
  assert.deepEqual(
    files.map((f) => f.path),
    ['with space.txt', '한 two.sql', 'tab\there.txt', 'a b/b'],
  );
  assert.equal(files[1].rows.length, 3);
  assert.equal(unquotePath('"caf\\303\\251 \\"x\\" 100%.txt"'), 'café "x" 100%.txt');
  assert.equal(unquotePath('plain.txt'), 'plain.txt');
});

test('a text cut inside a hunk: the lines before the cut are kept and the file says its numbers do not hold', () => {
  const whole = text('diff --git a/a.txt b/a.txt', '--- a/a.txt', '+++ b/a.txt', '@@ -1 +1 @@', '-x', '+y');
  const head = ['diff --git a/b.txt b/b.txt', '--- a/b.txt', '+++ b/b.txt', '@@ -1,3 +1,4 @@', ' one', '+tw'];
  for (const cut of [head.join('\n'), head.join('\n') + '\n']) {
    const [a, b] = diffFiles(whole + cut, true);
    assert.equal(lineNumbersHold(a), true);
    assert.deepEqual([b.cut, lineNumbersHold(b)], [true, false]);
    assert.deepEqual(b.rows.slice(1).map(said), ['ctx 1 1 one', 'add - 2 tw']);
  }
  // a hunk that ends early because the next file starts is cut too, with nothing said about the text
  const [short, next] = diffFiles(head.join('\n') + '\n' + whole);
  assert.deepEqual([short.cut, next.cut, next.path], [true, false, 'a.txt']);
  // a text cut between two lines looks whole: the caller says it was cut, and the last file is not trusted
  assert.deepEqual(
    diffFiles(whole + whole.replaceAll('a.txt', 'c.txt'), true).map((f) => f.cut),
    [false, true],
  );
  // a hunk header cut short opens no hunk and numbers nothing
  const [stub] = diffFiles('diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1,3 +1', true);
  assert.deepEqual([stub.rows.map(said), stub.cut], [['meta - - @@ -1,3 +1'], true]);
});

test('several sections of one path: every section is kept, in order, and none of its lines is numbered', () => {
  const part = (from: number, line: string) =>
    text('diff --git a/a.txt b/a.txt', '--- a/a.txt', '+++ b/a.txt', `@@ -${from},1 +${from},2 @@`, ' keep', `+${line}`);
  const other = text('diff --git a/b.txt b/b.txt', '--- a/b.txt', '+++ b/b.txt', '@@ -1 +1 @@', '-x', '+y');
  const diff = part(1, 'first') + other + part(7, 'second');
  assert.equal(diffSections(diff).length, 3);
  const [a, b] = diffFiles(diff);
  assert.deepEqual([a.path, a.sections, lineNumbersHold(a)], ['a.txt', 2, false]);
  assert.deepEqual(a.rows.map(said), [
    'hunk - - @@ -1,1 +1,2 @@',
    'ctx - - keep',
    'add - - first',
    'hunk - - @@ -7,1 +7,2 @@',
    'ctx - - keep',
    'add - - second',
  ]);
  assert.deepEqual([b.path, b.sections, lineNumbersHold(b)], ['b.txt', 1, true]);
});

test("a merge's combined diff is shown and not numbered; text before the first file is skipped", () => {
  const [f] = diffFiles(
    text(
      'commit 0123456789abcdef',
      'diff --cc both.txt',
      'index 1111111,2222222..3333333',
      '--- a/both.txt',
      '+++ b/both.txt',
      '@@@ -1,2 -1,2 +1,3 @@@',
      '  same',
      '--- theirs',
      '++ours',
    ),
  );
  assert.deepEqual([f.path, f.combined, lineNumbersHold(f)], ['both.txt', true, false]);
  assert.deepEqual(f.rows.map(said), ['hunk - - @@@ -1,2 -1,2 +1,3 @@@', 'meta - -   same', 'meta - - --- theirs', 'meta - - ++ours']);
  assert.deepEqual(diffFiles(''), []);
});

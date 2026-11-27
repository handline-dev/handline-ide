// task.changes' seed: the terminal task's commits, one landed, the next still on the agent's branch; and a done task's, both landed.
import type { SeedCommit } from '#src/taskchanges.ts';

export const termBuildCommits = (ago: (min: number) => string): SeedCommit[] => [
  {
    sha: 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678',
    repo: 'handline',
    subject: 'Terminal pane reads the bytes channel (T1688)', // local-ref-ok
    agent: null,
    at: ago(150),
    branch: 'main',
    landed: true,
    files: [
      {
        path: 'apps/web/src/term/Pane.tsx',
        repo: 'handline',
        added: 2,
        deleted: 1,
        binary: false,
        diff: [
          'diff --git a/apps/web/src/term/Pane.tsx b/apps/web/src/term/Pane.tsx',
          '--- a/apps/web/src/term/Pane.tsx',
          '+++ b/apps/web/src/term/Pane.tsx',
          '@@ -1,2 +1,3 @@',
          '-const source = lines();',
          "+import { bytes } from './bytes';",
          '+const source = bytes();',
          ' export default source;',
          '',
        ].join('\n'),
      },
    ],
  },
  {
    sha: 'f0e1d2c3b4a5968778695a4b3c2d1e0f98765432',
    repo: 'handline',
    subject: 'Terminal pane keeps its scroll on resize (T1688)', // local-ref-ok
    agent: 'handline.term-build',
    at: ago(30),
    branch: 'main',
    landed: false,
    files: [
      {
        path: 'apps/web/src/term/Pane.tsx',
        repo: 'handline',
        added: 1,
        deleted: 0,
        binary: false,
        diff: [
          'diff --git a/apps/web/src/term/Pane.tsx b/apps/web/src/term/Pane.tsx',
          '--- a/apps/web/src/term/Pane.tsx',
          '+++ b/apps/web/src/term/Pane.tsx',
          '@@ -3,1 +3,2 @@',
          ' export default source;',
          '+export const keepScroll = true;',
          '',
        ].join('\n'),
      },
      {
        path: 'apps/web/src/term/scroll.ts',
        repo: 'handline',
        added: 1,
        deleted: 0,
        binary: false,
        diff: [
          'diff --git a/apps/web/src/term/scroll.ts b/apps/web/src/term/scroll.ts',
          'new file mode 100644',
          '--- /dev/null',
          '+++ b/apps/web/src/term/scroll.ts',
          '@@ -0,0 +1 @@',
          '+export const at = 0;',
          '',
        ].join('\n'),
      },
    ],
  },
];

const lines = (...l: string[]) => [...l, ''].join('\n');

/** A done task's commits: two files changed by the first, one of them again by the second (so All changes has two sections of
 *  it), a deleted line that is a SQL comment, and a new file. */
export const fixtureCommits = (ago: (min: number) => string): SeedCommit[] => [
  {
    sha: '3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f',
    repo: 'handline',
    subject: 'A fixture that is missing says so (T1650)', // local-ref-ok
    agent: null,
    at: ago(1700),
    branch: 'main',
    landed: true,
    files: [
      {
        path: 'contract/fixtures/schema.sql',
        repo: 'handline',
        added: 2,
        deleted: 2,
        binary: false,
        diff: lines(
          'diff --git a/contract/fixtures/schema.sql b/contract/fixtures/schema.sql',
          '--- a/contract/fixtures/schema.sql',
          '+++ b/contract/fixtures/schema.sql',
          '@@ -1,5 +1,5 @@',
          '--- fixtures: one row per method',
          '+-- fixtures: one row per call',
          ' create table fixtures (',
          '   name text primary key,',
          '-  body text',
          '+  body text not null',
          ' );',
        ),
      },
      {
        path: 'packages/fake-daemon/src/fixtures.ts',
        repo: 'handline',
        added: 3,
        deleted: 2,
        binary: false,
        diff: lines(
          'diff --git a/packages/fake-daemon/src/fixtures.ts b/packages/fake-daemon/src/fixtures.ts',
          '--- a/packages/fake-daemon/src/fixtures.ts',
          '+++ b/packages/fake-daemon/src/fixtures.ts',
          '@@ -12,6 +12,7 @@ export function fixture(name: string) {',
          '   const file = path.join(dir, `${name}.json`);',
          "-  const text = fs.readFileSync(file, 'utf8');",
          '-  return JSON.parse(text);',
          '+  if (!fs.existsSync(file)) throw new Error(`no fixture ${name}`);',
          "+  const text = fs.readFileSync(file, 'utf8');",
          '+  return JSON.parse(text) as Fixture;',
          ' }',
          ' ',
          ' export const names = () =>',
        ),
      },
    ],
  },
  {
    sha: '9a8b7c6d5e4f30211203f4e5d6c7b8a99a8b7c6d',
    repo: 'handline',
    subject: 'A fixture carries a result or an error (T1650)', // local-ref-ok
    agent: null,
    at: ago(1600),
    branch: 'main',
    landed: true,
    files: [
      {
        path: 'contract/fixtures/README.md',
        repo: 'handline',
        added: 3,
        deleted: 0,
        binary: false,
        diff: lines(
          'diff --git a/contract/fixtures/README.md b/contract/fixtures/README.md',
          'new file mode 100644',
          '--- /dev/null',
          '+++ b/contract/fixtures/README.md',
          '@@ -0,0 +1,3 @@',
          '+# Fixtures',
          '+',
          '+One file per call: its method, its params, and its result or its error.',
        ),
      },
      {
        path: 'packages/fake-daemon/src/fixtures.ts',
        repo: 'handline',
        added: 1,
        deleted: 0,
        binary: false,
        diff: lines(
          'diff --git a/packages/fake-daemon/src/fixtures.ts b/packages/fake-daemon/src/fixtures.ts',
          '--- a/packages/fake-daemon/src/fixtures.ts',
          '+++ b/packages/fake-daemon/src/fixtures.ts',
          '@@ -41,5 +41,6 @@ export function check(f: Fixture) {',
          "   if (!f.method) throw new Error('a fixture names its method');",
          "   if (!f.params) throw new Error('a fixture carries params');",
          "+  if (!f.result && !f.error) throw new Error('a fixture carries a result or an error');",
          '   return f;',
          ' }',
          ' ',
        ),
      },
    ],
  },
];

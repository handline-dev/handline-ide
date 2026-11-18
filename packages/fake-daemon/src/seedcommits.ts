// task.changes' seed: T1688's commits, one landed, the next still on the agent's branch.
import type { SeedCommit } from '#src/taskchanges.ts';

export const termBuildCommits = (ago: (min: number) => string): SeedCommit[] => [
  {
    sha: 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678',
    repo: 'handline',
    subject: 'Terminal pane reads the bytes channel (T1688)', // local-ref-ok
    agent: null,
    at: ago(150),
    on_main: true,
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
    on_main: false,
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

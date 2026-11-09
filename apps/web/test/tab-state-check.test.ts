import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from 'vitest';

const files = (dir: string): string[] =>
  readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? files(join(dir, e.name)) : /\.tsx?$/.test(e.name) ? [join(dir, e.name)] : [],
  );

// A view word kept in plain useState in a tab view is lost on leaving the tab (persist/tabState.ts). By name only, and not `open`: most
// useState (typing, hover, a menu or a fold of one line) rightly start fresh. One that rightly is not kept says why: `// not kept: <why>`.
const TAB_VIEWS = ['chat', 'changes', 'tools', 'usage', 'requests'];
const VIEW_WORD =
  /const \[(show|only|filter|sort|view|mode|against|shut|folded|collapsed|expanded|reply|replyTo|picked|range|win)\b[^\]]*\] = useState\b/;

test('a view word in a tab view is kept with the tab (useTabState), or says why not', () => {
  const src = join(import.meta.dirname, '../src');
  const found = TAB_VIEWS.flatMap((d) => files(join(src, d))).flatMap((f) =>
    readFileSync(f, 'utf8')
      .split('\n')
      .flatMap((line, i) => (VIEW_WORD.test(line) && !line.includes('// not kept:') ? [`${f.slice(src.length + 1)}:${i + 1}`] : [])),
  );
  expect(found).toEqual([]);
});

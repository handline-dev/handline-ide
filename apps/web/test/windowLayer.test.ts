import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from 'vitest';

const files = (dir: string): string[] =>
  readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? files(join(dir, e.name)) : e.name.endsWith('.tsx') ? [join(dir, e.name)] : [],
  );

// A full-screen layer must say whether it moves the window (windowLayer.tsx), or the page's drag strips under it take its top buttons.
test('every full-screen layer declares itself with useWindowLayer', () => {
  const src = join(import.meta.dirname, '../src');
  const missing = files(src).filter((f) => {
    const text = readFileSync(f, 'utf8');
    return /\bfixed inset-0\b|createPortal\(/.test(text) && !text.includes('useWindowLayer(');
  });
  expect(missing.map((f) => f.slice(src.length + 1))).toEqual([]);
});

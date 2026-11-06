import fs from 'node:fs';
import path from 'node:path';
import { expect, test } from 'vitest';

const here = path.dirname(new URL(import.meta.url).pathname);
const files = (dir: string): string[] =>
  fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? files(path.join(dir, e.name)) : [path.join(dir, e.name)]));

test('every shell:* key the desktop shell leaves for the page has a reader in the page', () => {
  const preload = fs.readFileSync(path.join(here, '../../electron/src/preload.ts'), 'utf8');
  const keys = [...preload.matchAll(/setItem\('(shell:[^']+)'/g)].map((m) => m[1]!);
  expect(keys.length).toBeGreaterThan(0);
  const page = files(path.join(here, '../src'))
    .map((f) => fs.readFileSync(f, 'utf8'))
    .join('\n');
  expect(keys.filter((k) => !page.includes(`getItem('${k}')`))).toEqual([]);
});

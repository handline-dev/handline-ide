import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { rmBundle } from '#src/swap.ts';

test('rmBundle removes a bundle folder with an .asar file in it, with asar reading off only while it runs', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'hl-swap-'));
  const res = path.join(dir, 'Handline.app', 'Contents', 'Resources');
  fs.mkdirSync(res, { recursive: true });
  fs.writeFileSync(path.join(res, 'app.asar'), 'x');
  process.noAsar = false;
  rmBundle(path.join(dir, 'Handline.app'));
  assert.equal(fs.existsSync(path.join(dir, 'Handline.app')), false);
  assert.equal(process.noAsar, false);
  fs.rmSync(dir, { recursive: true, force: true });
});

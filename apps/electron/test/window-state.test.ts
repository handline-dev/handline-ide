import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test, { after } from 'node:test';
import { readFrame, readRoute, restorable, writeFrame, writeRoute, type Frame } from '#src/window-state.ts';

const made: string[] = [];
const dir = () => (made.push(fs.mkdtempSync(path.join(os.tmpdir(), 'hl-ws-'))), made.at(-1)!);
after(() => made.forEach((d) => fs.rmSync(d, { recursive: true, force: true })));
const frame = (o: Partial<Frame> = {}): Frame => ({ x: 100, y: 80, width: 1280, height: 860, maximized: false, fullScreen: false, ...o });
const main = { x: 0, y: 0, width: 1920, height: 1080 };
const second = { x: 1920, y: 0, width: 1920, height: 1080 };

test('a route is kept and read without being taken; a missing or odd file is no route', () => {
  const f = path.join(dir(), 'route.txt');
  assert.equal(readRoute(f), '');
  writeRoute(f, '#/agents/a?x=1');
  assert.equal(readRoute(f), '#/agents/a?x=1');
  assert.equal(readRoute(f), '#/agents/a?x=1', 'read twice: the file stays');
  writeRoute(f, 'javascript:alert(1)');
  assert.equal(readRoute(f), '');
  writeRoute(f, '#/tasks/12 ; x');
  assert.equal(readRoute(f), '');
});

test('a frame round-trips; garbage or a tiny size is no frame', () => {
  const f = path.join(dir(), 'window.json');
  assert.equal(readFrame(f), null);
  writeFrame(f, frame({ maximized: true }));
  assert.deepEqual(readFrame(f), frame({ maximized: true }));
  fs.writeFileSync(f, '{"x":1');
  assert.equal(readFrame(f), null);
  writeFrame(f, frame({ width: 10 }));
  assert.equal(readFrame(f), null);
});

test('a frame is restored only while enough of it is on a connected display', () => {
  const on2 = frame({ x: 2200 });
  assert.equal(restorable(on2, [main, second]), on2);
  assert.equal(restorable(on2, [main]), null, 'the second display was unplugged');
  assert.equal(restorable(frame({ x: 1850 }), [main]), null, 'only a sliver is still visible');
  assert.equal(restorable(frame({ x: 1700 }), [main]) !== null, true);
  assert.equal(restorable(null, [main]), null);
});

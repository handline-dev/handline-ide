import { after, test } from 'node:test';
import type { spawn } from 'node:child_process';
import { EventEmitter } from 'node:events';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import type { Sender } from '#src/bridge.ts';
import { Composer, loginAnswer, parseLoginEnv, splitWords, sweepTemp, type EditorEnv } from '#src/editor.ts';

const ORIGIN = 'http://127.0.0.1:5000';
const top: Sender = { senderFrame: { url: `${ORIGIN}/`, parent: null } };
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'hl-ed-'));
after(() => fs.rmSync(tmp, { recursive: true, force: true }));
const bin = path.join(tmp, 'bin');
fs.mkdirSync(bin);
// Editors run in-process; the files exist only for the PATH lookup. The process-group cancel test spawns a real one.
type Edit = (file: string) => Promise<number>;
const editors = new Map<string, Edit>();
const script = (name: string, edit: Edit) => {
  const p = path.join(bin, name);
  fs.writeFileSync(p, '#!/bin/sh\n', { mode: 0o755 });
  editors.set(p, edit);
  return p;
};
const appendLine = (line: string) => async (f: string) => (fs.appendFileSync(f, `${line}\n`), 0);
const appends = script('appends', appendLine('added'));
const quick = script('quick', async () => 0);
const fails = script('fails', async () => 1);
const slow = path.join(bin, 'slow');
fs.writeFileSync(slow, '#!/bin/sh\nsleep 30\n', { mode: 0o755 });
const symlinks = script('symlinks', async (f) => (fs.rmSync(f), fs.symlinkSync('/etc/hosts', f), 0));
const big = script('big', async (f) => (fs.appendFileSync(f, 'a'.repeat(300000)), 0));
const spaced = script('my editor', appendLine('spaced'));
let pids = 1000;
const fakeSpawn = ((file: string, args: string[]) => {
  const child = Object.assign(new EventEmitter(), { pid: ++pids });
  void editors.get(file)!(args.at(-1)!).then((c) => child.emit('exit', c));
  return child;
}) as unknown as typeof spawn;
const mk = (env: EditorEnv) => new Composer(ORIGIN, { env: async () => env, tmp, spawn: fakeSpawn });
const left = () => fs.readdirSync(tmp).filter((n) => n.startsWith('handline-'));
const code = (r: { ok: boolean }) => (r.ok ? 'ok' : (r as unknown as { code: string }).code);

test('the editor is split into words without a shell: quotes group, operators are refused', () => {
  assert.deepEqual(splitWords('code --wait'), ['code', '--wait']);
  assert.deepEqual(splitWords(`"/Applications/My Editor/bin/e" -w 'a b'`), ['/Applications/My Editor/bin/e', '-w', 'a b']);
  assert.deepEqual(splitWords('a\\ b c'), ['a b', 'c']);
  for (const bad of ['a; b', 'a | b', 'a && b', 'a > b', '$(x)', 'a `b`', 'a\nb', '(a)', '"open']) assert.equal(splitWords(bad), null, bad);
});

test('an editor that adds a line returns the new text, changed, and the temp directory is gone', async () => {
  const c = mk({ VISUAL: appends, PATH: '' });
  const r = await c.compose(top, { text: 'hello\n' });
  assert.deepEqual(r, { ok: true, text: 'hello\nadded\n', changed: true });
  assert.deepEqual(left(), []);
});

test('VISUAL wins over EDITOR; a quoted path with a space runs; arguments come before the file', async () => {
  assert.equal(((await mk({ VISUAL: `"${spaced}"`, EDITOR: fails, PATH: '' }).compose(top, {})) as { text: string }).text, 'spaced\n');
  assert.equal(code(await mk({ EDITOR: fails, PATH: '' }).compose(top, {})), 'editor_failed');
});

test('the temp file is mode 0600 in a 0700 handline-* directory, named for the chosen extension, and holds only the text', async () => {
  let seen = { dir: '', dirMode: 0, name: '', mode: 0, body: '' };
  const probe = script('probe', async (f) => {
    const d = path.dirname(f);
    seen = {
      dir: path.basename(d),
      dirMode: fs.statSync(d).mode & 0o777,
      name: path.basename(f),
      mode: fs.statSync(f).mode & 0o777,
      body: fs.readFileSync(f, 'utf8'),
    };
    fs.appendFileSync(f, 'x');
    return 0;
  });
  assert.equal(code(await mk({ VISUAL: probe, PATH: '' }).compose(top, { text: 'secret', ext: 'txt' })), 'ok');
  assert.match(seen.dir, /^handline-/);
  assert.deepEqual([seen.dirMode, seen.name, seen.mode, seen.body], [0o700, 'compose.txt', 0o600, 'secret']);
});

test('an editor that returns at once with the file unchanged is not_waiting; one that exits 1 is editor_failed', async () => {
  assert.equal(code(await mk({ VISUAL: quick, PATH: '' }).compose(top, { text: 'x' })), 'not_waiting');
  assert.equal(code(await mk({ VISUAL: fails, PATH: '' }).compose(top, { text: 'x' })), 'editor_failed');
  assert.deepEqual(left(), []);
});

test('a terminal editor, no editor, an operator and a missing program are told apart, and none starts anything', async () => {
  assert.equal(code(await mk({ VISUAL: 'vim', PATH: '/nowhere' }).compose(top, {})), 'terminal_editor');
  assert.equal(code(await mk({ EDITOR: '/usr/bin/nano -w' }).compose(top, {})), 'terminal_editor');
  assert.equal(code(await mk({ PATH: '' }).compose(top, {})), 'no_editor');
  assert.equal(code(await mk({ VISUAL: 'code --wait; rm -rf x', PATH: bin }).compose(top, {})), 'bad_editor');
  assert.equal(code(await mk({ VISUAL: 'nope-editor', PATH: bin }).compose(top, {})), 'bad_editor');
  assert.equal(code(await mk({ VISUAL: 'sub/dir', PATH: bin }).compose(top, {})), 'bad_editor');
  assert.deepEqual(left(), []);
});

test('the first word is found on the PATH the shell was given, and not on another', async () => {
  assert.equal(code(await mk({ VISUAL: 'appends', PATH: `/usr/bin:${bin}` }).compose(top, { text: 'a' })), 'ok');
  assert.equal(code(await mk({ VISUAL: 'appends', PATH: '/usr/bin:/bin' }).compose(top, { text: 'a' })), 'bad_editor');
});

test('a second compose while one runs is busy; cancelCompose kills the editor and the compose settles cancelled; the directory is removed', async () => {
  const c = new Composer(ORIGIN, { env: async () => ({ VISUAL: slow, PATH: '' }), tmp });
  const first = c.compose(top, { text: 'a' });
  await new Promise((r) => setTimeout(r, 200));
  assert.equal(c.running, true);
  assert.equal(code(await c.compose(top, {})), 'busy');
  assert.deepEqual(await c.cancel(top, undefined), { ok: true });
  assert.equal(code(await first), 'cancelled');
  assert.equal(c.running, false);
  assert.deepEqual(left(), []);
});

test('a compose.md replaced by a symlink is refused, and a file over 256 KiB is too_large', async () => {
  assert.notEqual(code(await mk({ VISUAL: symlinks, PATH: '' }).compose(top, { text: 'a' })), 'ok');
  assert.equal(code(await mk({ VISUAL: big, PATH: '' }).compose(top, { text: 'a' })), 'too_large');
  assert.equal(code(await mk({ VISUAL: appends, PATH: '' }).compose(top, { text: 'a'.repeat(256 * 1024 + 1) })), 'too_large');
  assert.deepEqual(left(), []);
});

test('bad arguments and other senders are bad_request', async () => {
  const c = mk({ VISUAL: appends, PATH: '' });
  for (const a of [{ ext: 'sh' }, { text: 1 }, { extra: 1 }, 'x', [1]]) assert.equal(code(await c.compose(top, a)), 'bad_request');
  assert.equal(code(await c.compose({ senderFrame: { url: 'https://evil.example/', parent: null } }, {})), 'bad_request');
  assert.equal(code(await c.compose({ senderFrame: null }, {})), 'bad_request');
  assert.equal((await c.cancel({ senderFrame: null }, undefined)).ok, false);
});

test('the login shell reply is read between the markers; rc output around it is ignored', () => {
  const out = 'welcome\n__HL_BEGIN__\nVISUAL=code --wait\nEDITOR=\nPATH=/opt/homebrew/bin:/usr/bin\n__HL_END__\nbye\n';
  assert.deepEqual(parseLoginEnv(out), { VISUAL: 'code --wait', EDITOR: undefined, PATH: '/opt/homebrew/bin:/usr/bin' });
  assert.deepEqual(parseLoginEnv('nothing'), { VISUAL: undefined, EDITOR: undefined, PATH: undefined });
});

test('at launch a handline-* directory older than a day is removed and a fresh one stays', async () => {
  const old = path.join(tmp, 'handline-old');
  const fresh = path.join(tmp, 'handline-fresh');
  fs.mkdirSync(old);
  fs.mkdirSync(fresh);
  const t = Date.now() / 1000 - 3 * 24 * 3600;
  fs.utimesSync(old, t, t);
  await sweepTemp(tmp);
  assert.equal(fs.existsSync(old), false);
  assert.equal(fs.existsSync(fresh), true);
  fs.rmSync(fresh, { recursive: true });
});

test('a cancel with no editor open does nothing, so the next compose is not cancelled', async () => {
  const c = mk({ VISUAL: appends, PATH: '' });
  assert.deepEqual(await c.cancel(top, undefined), { ok: true });
  assert.deepEqual(await c.compose(top, { text: 'hello\n' }), { ok: true, text: 'hello\nadded\n', changed: true });
});

test('a cancel while the compose is still being set up stops the editor from starting', async () => {
  let started = false;
  const touch = script('touch-marker', async () => ((started = true), 0));
  const c = new Composer(ORIGIN, {
    env: async () => (await new Promise((r) => setTimeout(r, 150)), { VISUAL: touch, PATH: '' }),
    tmp,
    spawn: fakeSpawn,
  });
  const first = c.compose(top, { text: 'a' });
  await new Promise((r) => setTimeout(r, 30));
  assert.deepEqual(await c.cancel(top, undefined), { ok: true });
  assert.equal(code(await first), 'cancelled');
  assert.equal(started, false);
  assert.deepEqual(left(), []);
});

test('loginAnswer: a shell stopped for taking too long is a timeout, with what it printed; a failure with no output is no environment', () => {
  const out = '__HL_BEGIN__\nVISUAL=\nEDITOR=\nPATH=/opt/homebrew/bin\n__HL_END__\n';
  assert.deepEqual(loginAnswer(null, out), { env: { VISUAL: undefined, EDITOR: undefined, PATH: '/opt/homebrew/bin' }, timedOut: false });
  assert.deepEqual(loginAnswer({ killed: true }, ''), { env: {}, timedOut: true });
  assert.deepEqual(loginAnswer({ killed: false }, ''), { env: {}, timedOut: false });
  assert.equal(loginAnswer({ killed: true }, out).env.PATH, '/opt/homebrew/bin');
});

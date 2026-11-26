// The read-only git calls behind opening a repo path or #N: run in an agent's worktree, never with a shell, with the names checked first.
import { execFile } from 'node:child_process';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import type { OpenDeps } from '#src/open.ts';

const run = (cwd: string, args: string[]): Promise<Buffer | null> =>
  new Promise((resolve) =>
    execFile('git', ['-C', cwd, ...args], { encoding: 'buffer', maxBuffer: 20 * 1024 * 1024, timeout: 10_000 }, (err, out) =>
      resolve(err ? null : out),
    ),
  );

export const gitView: NonNullable<OpenDeps['git']> = {
  async hasBranch(cwd, name) {
    for (const ref of [`refs/heads/${name}`, `refs/remotes/origin/${name}`])
      if (await run(cwd, ['rev-parse', '--verify', '--quiet', ref])) return true;
    return false;
  },
  async show(cwd, ref, file) {
    // A branch is tried as itself, then as origin's; the ref is one the caller named and `hasBranch` has vouched for, or the default branch.
    for (const r of [ref, `origin/${ref}`]) {
      const out = await run(cwd, ['show', `${r}:${file}`]);
      if (out) return out;
    }
    return null;
  },
  async repoOf(cwd) {
    const out = await run(cwd, ['remote', 'get-url', 'origin']);
    const m = /github\.com[:/]([\w.-]+\/[\w.-]+?)(?:\.git)?\s*$/.exec(out?.toString() ?? '');
    return m ? m[1]! : null;
  },
  async defaultBranch(cwd) {
    // As the daemon reads it: origin's HEAD when it names a branch origin has, else main or master when the repo has one.
    const head = (await run(cwd, ['symbolic-ref', '-q', '--short', 'refs/remotes/origin/HEAD']))
      ?.toString()
      .trim()
      .replace(/^origin\//, '');
    if (head && (await run(cwd, ['rev-parse', '--verify', '--quiet', `refs/remotes/origin/${head}`]))) return head;
    for (const b of ['main', 'master']) if (await gitView.hasBranch(cwd, b)) return b;
    return null;
  },
};

/** A fresh folder under the temp dir holding the file, read-only, so the default app opens a copy and nothing can be saved over the repo. */
export async function scratch(tmp: string, name: string, bytes: Buffer): Promise<string> {
  const dir = await fs.mkdtemp(path.join(tmp || os.tmpdir(), 'handline-view-'));
  const file = path.join(dir, name.replace(/[^\w.@+-]/g, '_') || 'file');
  await fs.writeFile(file, bytes, { mode: 0o444 });
  return file;
}

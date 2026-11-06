// `pnpm dev <team>`: work on the app against a team's dev instance (`hl dev on <team>`), never the live home. The vite page serves
// itself (src/dev-team.ts) and watch.ts rebuilds the main process and preloads on every save and restarts Electron, on an IPC channel:
// Ctrl-C, a TERM or HUP, or watch.ts ending stops both, and watch.ts quits with its Electron the moment this process is gone, even by
// kill -9. An instance that is off is said and the run stops; there is no fallback to the live daemon. The run is not a follow-main one, so
// it offers no update and never needs dist/build.json.
import { spawn } from 'node:child_process';
import os from 'node:os';
import { answers, freePort, instanceFile, offReason, readInstance, teamEnv } from '#src/dev-team.ts';

const team = process.argv[2];
if (!team) {
  console.error('usage: pnpm dev <team>; the live app is pnpm app');
  process.exit(2);
}
const i = readInstance(instanceFile(os.homedir(), team), team);
const why = typeof i === 'string' ? i : await offReason(i);
if (typeof i === 'string' || why) {
  console.error(typeof i === 'string' ? i : why);
  process.exit(1);
}
const port = await freePort();
const devUrl = `http://127.0.0.1:${port}`;
const env = teamEnv(process.env, i, devUrl);
console.log(`${team}: ${i.branch}@${i.sha}, listener ${i.listen}, page ${devUrl}`);
const vite = spawn('pnpm', ['--filter', '@handline/web', 'dev', '--port', String(port), '--strictPort'], { stdio: 'inherit', env });
const stop = (code: number): never => {
  vite.kill();
  shell?.kill('SIGTERM');
  process.exit(code);
};
vite.once('exit', () => stop(1));
for (let n = 0; !(await answers(`127.0.0.1:${port}`)); n++) {
  if (n > 120) stop(1);
  await new Promise((r) => setTimeout(r, 250));
}
const shell = spawn(process.execPath, ['watch.ts'], {
  cwd: import.meta.dirname,
  stdio: ['inherit', 'inherit', 'inherit', 'ipc'],
  env: { ...env, HANDLINE_DEV_TEAM: team, HANDLINE_DEV_WORKTREE: i.worktree ?? '' },
});
shell.once('exit', (c) => stop(c ?? 1));
for (const s of ['SIGINT', 'SIGTERM', 'SIGHUP'] as const) process.once(s, () => stop(130));

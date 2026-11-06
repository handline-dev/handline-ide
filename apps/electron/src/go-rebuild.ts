// Under `pnpm dev <team>` a save in go/ rebuilds the team's daemon: `hl dev rebuild <team>` builds first and restarts the job only when
// the build worked, so a failed build leaves the old daemon running and the next save tries again. The window reconnects by itself, as it
// does whenever a daemon goes away and comes back; the shell is not restarted.
import { execFile } from 'node:child_process';
import path from 'node:path';

/** A Go source whose change changes the daemon: a .go file that is not a test. */
export const isDaemonSource = (file: string): boolean => file.endsWith('.go') && !file.endsWith('_test.go');

/** Whether this checkout is where the team's daemon is built from; otherwise a Go change here is not applied. */
export const buildsFromHere = (worktree: string | undefined, checkout: string): boolean =>
  !!worktree && path.resolve(worktree) === path.resolve(checkout);

export interface Rebuilt {
  ok: boolean;
  /** The command's one line when it worked, its error when it did not. */
  text: string;
}

/** `hl dev rebuild <team>` with the live home's environment: the dev run's own HANDLINE_ variables name the instance, which `hl dev` must not see. */
export const hlRebuild =
  (team: string, env: NodeJS.ProcessEnv = process.env) =>
  (): Promise<Rebuilt> =>
    new Promise((resolve) => {
      const live = Object.fromEntries(Object.entries(env).filter(([k]) => !k.startsWith('HANDLINE_')));
      execFile('hl', ['dev', 'rebuild', team], { env: live, timeout: 600_000 }, (err, out, errOut) => {
        const oneLine = (s: string): string => s.trim().split('\n').slice(-1)[0] ?? '';
        resolve(err ? { ok: false, text: (errOut || out || err.message).trim() } : { ok: true, text: oneLine(out) || 'rebuilt' });
      });
    });

/**
 * The rebuild loop: a change waits `settleMs` for the rest of a save, then one rebuild runs at a time; a change during a rebuild
 * queues exactly one more, which runs after it.
 */
export function goRebuilder(o: { rebuild: () => Promise<Rebuilt>; log: (line: string) => void; settleMs?: number }): {
  changed(file: string): void;
  idle(): Promise<void>;
} {
  let timer: NodeJS.Timeout | undefined;
  let running: Promise<void> | undefined;
  let again = false;
  const run = async (): Promise<void> => {
    do {
      again = false;
      const r = await o.rebuild();
      o.log(r.ok ? `go: ${r.text}` : `go: the build failed, the old daemon keeps running:\n${r.text}`);
    } while (again);
    running = undefined;
  };
  const start = (): void => {
    timer = undefined;
    if (running) again = true;
    else running = run();
  };
  return {
    changed(file) {
      if (!isDaemonSource(file)) return;
      clearTimeout(timer);
      timer = setTimeout(start, o.settleMs ?? 300);
    },
    idle: async () => {
      while (timer || running) await (running ?? new Promise((r) => setTimeout(r, 10)));
    },
  };
}

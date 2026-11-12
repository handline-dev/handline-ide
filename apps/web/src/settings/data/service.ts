// This Mac's daemon as the desktop shell reports it, with the shell's Start, Restart and Stop; null in a plain browser and on another device's daemon.
import type { ShellDaemon } from '@handline/app-core';
import { hooks } from '@handline/app-data';
import { useEffect, useState } from 'react';
import { useKeyed } from '#src/settings/data/keyed.ts';
import { shell } from '#src/shell.ts';

type Reply = { ok: true } | { ok: false; message: string };
const done = (r: Reply) => (r.ok ? Promise.resolve() : Promise.reject(new Error(r.message)));

export function useService() {
  const status = hooks.useDaemonStatus();
  const [daemon, setDaemon] = useState<ShellDaemon | null>(null);
  const [build, setBuild] = useState<{ sha: string | null; canRestart: boolean } | null>(null);
  const { pending, run } = useKeyed();
  useEffect(() => shell()?.onDaemon?.(setDaemon), []);
  useEffect(() => {
    let live = true;
    shell()
      ?.build?.()
      .then((r) => live && r.ok && setBuild(r))
      .catch(() => {});
    return () => {
      live = false;
    };
  }, []);
  const s = shell();
  const { restartDaemon, stopDaemon, startDaemon } = s ?? {};
  if (!daemon?.canStart || !restartDaemon || !stopDaemon || !startDaemon) return null;
  const version = status.data?.version;
  return {
    state: daemon.state,
    reason: daemon.reason,
    startedAt: status.data?.started_at ?? null,
    thisBuild: build?.sha && version ? version.includes(build.sha.slice(0, 7)) : null,
    canRestart: !!build?.canRestart,
    pending,
    restart: () => run('restart', () => restartDaemon().then(done)),
    stop: () => run('stop', () => stopDaemon().then(done)),
    start: () => run('start', () => startDaemon().then(done)),
  };
}

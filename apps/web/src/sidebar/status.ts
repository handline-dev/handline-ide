// The sidebar's status row: the machine's level and line, the pause switch, keep-awake, the inbox count and the week's Usage %.
import { hooks, useAccounts } from '@handline/app-data';
import { useMachine, type KeepAwake } from '#src/settings/data/machine.ts';
import { useKeyed } from '#src/settings/data/keyed.ts';

export type Level = 'ok' | 'near' | 'held' | 'down';
const NEXT_AWAKE: Record<KeepAwake, KeepAwake> = { off: 'working', working: 'on', on: 'off' };

/** The highest weekly use over the accounts, or null when none reports one. */
export const weekUsagePct = (accounts: { weekly: { used_pct: number } | null }[]): number | null => {
  const used = accounts.flatMap((a) => (a.weekly ? [a.weekly.used_pct] : []));
  return used.length ? Math.round(Math.max(...used)) : null;
};

export function useStatusRow() {
  const status = hooks.useDaemonStatus();
  // The machine's own name: its name.local without .local (terminal_host is the terminal kind, not the machine).
  const mdns = hooks.useConnectionAddresses().data?.addresses.find((a) => a.kind === 'mdns')?.host;
  const machine = useMachine();
  const accounts = useAccounts();
  const pause = hooks.useDaemonPause();
  const resume = hooks.useDaemonResume();
  const { pending, run } = useKeyed();
  const s = status.data;
  const paused = s?.paused ?? null;
  return {
    level: (status.isError ? 'down' : (s?.health?.level ?? 'ok')) as Level,
    text: s?.health?.text ?? '',
    unread: s?.inbox.unread ?? 0,
    host: mdns?.replace(/\.local$/, '') ?? '',
    usagePct: weekUsagePct(accounts.data?.accounts ?? []),
    awake: machine.awake,
    cycleAwake: () => machine.setAwake(NEXT_AWAKE[machine.awake]),
    paused,
    /** Pausing stops new work until resumed; the switch dims while the daemon answers. */
    togglePause: () => run('pause', () => (paused ? resume.mutateAsync({}) : pause.mutateAsync({}))).then(() => {}),
    pending: pending ?? machine.pending,
  };
}

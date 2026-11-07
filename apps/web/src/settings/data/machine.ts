// Settings > Machine. Write keys: 'awake', 'record', 'load_per_core_pct', 'ram_free_min_pct', 'stop:<name>'.
import { DEFAULT_LIMITS, useRows, type Limits } from '@handline/app-core';
import type { SettingsChanges } from '@handline/client';
import { hooks } from '@handline/app-data';
import { useKeyed } from '#src/settings/data/keyed.ts';

export type KeepAwake = 'off' | 'on' | 'working';

export function useMachine() {
  const status = hooks.useDaemonStatus();
  const settings = hooks.useSettingsGet();
  const set = hooks.useSettingsSet();
  const stopAgent = hooks.useSessionStop();
  const { pending, run } = useKeyed();
  const resources = status.data?.resources ?? null;
  const values = settings.data?.values;
  const limits: Limits = {
    load_per_core_pct: values?.load_per_core_pct ?? DEFAULT_LIMITS.load_per_core_pct,
    ram_free_min_pct: values?.ram_free_min_pct ?? DEFAULT_LIMITS.ram_free_min_pct,
  };
  const change = (key: string, changes: SettingsChanges) => run(key, () => set.mutateAsync({ changes })).then(() => {});
  return {
    loading: status.isPending || settings.isPending,
    error: status.isError || settings.isError,
    resources,
    /** Why the daemon starts no new work now, or null. */
    startHold: resources?.start_hold ?? null,
    limits,
    awake: (values?.keep_awake ?? 'off') as KeepAwake,
    /** Agents mid-turn: what keep-awake "working" holds the machine for. */
    working: status.data?.agents.by_state.running ?? 0,
    agents: useRows(resources).rows.map((r) => ({ name: r.agent, cpu_pct: r.cpuPct, rss_bytes: r.rss })),
    /** chat_transcript_rows: each turn's tool calls and text are stored for the chat's folded record. */
    record: values?.chat_transcript_rows ?? false,
    setRecord: (on: boolean) => change('record', { chat_transcript_rows: on }),
    setAwake: (v: KeepAwake) => change('awake', { keep_awake: v }),
    setLimit: (key: keyof Limits, n: number) => change(key, { [key]: n }),
    stop: (name: string) => run(`stop:${name}`, () => stopAgent.mutateAsync({ agent: name, mode: 'close' })).then(() => {}),
    pending,
  };
}

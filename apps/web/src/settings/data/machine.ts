// Settings > This Mac. Write keys: 'disk_keep_free_gb', 'agents_memory_max_gb', 'awake', 'record', 'load_per_core_pct', 'ram_free_min_pct',
// 'heavy_runs_policy', 'heavy_runs_at_once', 'stop:<name>'.
import { DEFAULT_LIMITS, useRows, type Limits, type RunPolicy } from '@handline/app-core';
import type { SettingsChanges } from '@handline/client';
import { Hooks } from '@handline/app-data';
import { useKeyed } from '#src/settings/data/keyed.ts';

export type KeepAwake = 'off' | 'on' | 'working';

export function useMachine() {
  const status = Hooks.useDaemonStatus();
  const settings = Hooks.useSettingsGet();
  const set = Hooks.useSettingsSet();
  const stopAgent = Hooks.useSessionStop();
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
    /** Free disk to keep and the agents' most memory together, in GB; 0 is off. */
    diskKeepFree: values?.disk_keep_free_gb ?? 0,
    agentsMemoryMax: values?.agents_memory_max_gb ?? 0,
    setTarget: (key: 'disk_keep_free_gb' | 'agents_memory_max_gb', n: number) => change(key, { [key]: n }),
    /** What happens to heavy runs (heavy_runs_policy) and, when they always take turns, how many go at once; 0 is half the cores. */
    runPolicy: (values?.heavy_runs_policy ?? 'observe') as RunPolicy,
    runsAtOnce: values?.heavy_runs_at_once ?? 0,
    setRunPolicy: (p: RunPolicy) => change('heavy_runs_policy', { heavy_runs_policy: p }),
    setRunsAtOnce: (n: number) => change('heavy_runs_at_once', { heavy_runs_at_once: n }),
    stop: (name: string) => run(`stop:${name}`, () => stopAgent.mutateAsync({ agent: name, mode: 'close' })).then(() => {}),
    pending,
  };
}

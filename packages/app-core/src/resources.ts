// What the machine and each agent use (daemon.status resources): the status bar's one line and the admin view's rows, heaviest first.
import type { Resources } from '@handline/client';

const GB = 2 ** 30;
const MB = 2 ** 20;

/** The disk line turns red under the 10 GB where the daemon raises a card. */
export const LOW_DISK_BYTES = 10 * GB;

/** Bytes as the shortest honest figure: 3.6 GB, 420 MB. */
export function bytesText(n: number): string {
  if (n >= GB) return `${(n / GB).toFixed(n >= 10 * GB ? 0 : 1)} GB`;
  return `${Math.round(n / MB)} MB`;
}

/** The limits the daemon holds new starts at, as settings.get keeps them: 150 is 1.5 load per core, 10 is 10% memory free, 0 is never. */
export type Limits = { load_per_core_pct: number; ram_free_min_pct: number };
export const DEFAULT_LIMITS: Limits = { load_per_core_pct: 0, ram_free_min_pct: 0 };

/** Whether the load is at the limit where the daemon holds new starts; a limit of 0 never holds. */
export const loadFull = (r: Resources, limitPct: number): boolean => limitPct > 0 && r.machine.load_per_core * 100 >= limitPct;
export const memoryLow = (r: Resources, floorPct: number): boolean => floorPct > 0 && r.machine.ram_free_pct < floorPct;

export type UseRow = { agent: string; cpu: string; memory: string; cpuPct: number; rss: number };

/** One row per agent, the heaviest first (CPU, then memory, then name), and the totals line over them. */
export function useRows(r: Resources | null | undefined): { rows: UseRow[]; total: string } {
  const rows = Object.entries(r?.agents ?? {})
    .map(([agent, u]) => ({ agent, cpu: `${Math.round(u.cpu_pct)}%`, memory: bytesText(u.rss_bytes), cpuPct: u.cpu_pct, rss: u.rss_bytes }))
    .sort((a, b) => b.cpuPct - a.cpuPct || b.rss - a.rss || a.agent.localeCompare(b.agent));
  const cpu = rows.reduce((n, x) => n + x.cpuPct, 0);
  const rss = rows.reduce((n, x) => n + x.rss, 0);
  const n = rows.length;
  return { rows, total: `${n} agent${n === 1 ? '' : 's'} · CPU ${Math.round(cpu)}% · memory ${bytesText(rss)}` };
}

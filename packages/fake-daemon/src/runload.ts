// The fake's run load (DaemonStatus.run_load, run.load, run.admit): a calm machine where each running agent has one run on time, or,
// with the slow-mac seed, a slow one where the runs are late and one waits for its turn under a cap.
import type { Methods, RunLoad, Timestamp } from '@handline/contract';
import { badRequest } from '#src/errors.ts';
import type { Session } from '#src/session.ts';

type Load = NonNullable<Methods['run.load']['result']>;
export type HeavyRuns = { policy: RunLoad['policy']; atOnce: number };

const MB = 2 ** 20;
const SHAPES = ['check.sh', 'test-go.sh', '@build-test'];
const USUAL_S = [364, 266, 95];
const ago = (now: Timestamp, s: number): Timestamp => new Date(Date.parse(now) - s * 1000).toISOString().replace('.000Z', 'Z');

/** run.load: null never, as the fake always watches. */
export function runLoadReply(sessions: Map<string, Session>, now: Timestamp, slow: boolean, set: HeavyRuns): Load {
  const names = [...sessions.keys()];
  // one agent is left to wait for its turn
  const going = slow
    ? names.slice(0, Math.max(1, Math.min(5, names.length - 1)))
    : names.filter((n) => sessions.get(n)!.state === 'running').slice(0, 2);
  const runs = going.map((agent, i) => {
    const usual = USUAL_S[i % 3]!;
    const late = slow && i < 3;
    return {
      agent,
      shape: SHAPES[i % 3]!,
      started: ago(now, Math.round(late ? usual * (1.6 + i * 0.2) : usual * 0.4)),
      usual_s: usual,
      background: i === 4,
      heavy: true,
      late,
      cpu_pct: i === 3 ? null : 180 + i * 95,
      rss_bytes: i === 3 ? null : (350 + i * 220) * MB,
      processes: i === 3 ? null : 7 + i * 4,
    };
  });
  const cap = set.policy === 'fixed' ? set.atOnce || 5 : set.policy === 'turns' && slow ? Math.max(2, runs.length - 1) : null;
  const waits = cap !== null && slow && names.length > going.length;
  return {
    policy: set.policy,
    slow,
    since: slow ? ago(now, 240) : null,
    causes: slow ? ['runs', 'loop'] : [],
    cap,
    late: runs.filter((r) => r.late).length,
    timed_out: 0,
    loop_ms: slow ? 41_000 : 1_900,
    host: { load_per_core: slow ? 17.2 : 0.32, memory_pressure: slow ? 'warn' : 'normal', compressed_pages_per_s: slow ? 17_600 : 0 },
    runs,
    waiting: waits ? [{ agent: names[going.length]!, shape: 'check.sh', since: ago(now, 75) }] : [],
  };
}

/** DaemonStatus.run_load, from the same figures. */
export function runLoadStatus(load: Load): RunLoad {
  const { policy, slow, since, causes, cap } = load;
  return { policy, slow, since, causes, cap, heavy: load.runs.filter((r) => r.heavy).length, waiting: load.waiting.length };
}

/** heavy_runs_policy and heavy_runs_at_once as the settings say them. */
export const heavyRunsOf = (v: { heavy_runs_policy?: RunLoad['policy']; heavy_runs_at_once?: number }): HeavyRuns => ({
  policy: v.heavy_runs_policy ?? 'observe',
  atOnce: v.heavy_runs_at_once ?? 0,
});

/** settings.set refuses a heavy runs value that is not its own kind, as the daemon does. */
export function checkHeavyRuns(key: string, value: unknown): void {
  if (key === 'heavy_runs_policy' && value !== null && !['observe', 'priority', 'turns', 'fixed'].includes(value as string))
    throw badRequest('heavy_runs_policy is observe, priority, turns or fixed', key);
  if (key === 'heavy_runs_at_once' && value !== null && (!Number.isInteger(value) || (value as number) < 0))
    throw badRequest('heavy_runs_at_once is a whole number, at least 0', key);
}

/** run.admit: the fake holds no run back. */
export function runAdmit(p: Record<string, unknown>) {
  for (const key of ['agent', 'id', 'shape'])
    if (typeof p[key] !== 'string' || (key !== 'shape' && !p[key])) throw badRequest(`${key} is a string`, key);
  return { go: true, waited_s: 0 };
}

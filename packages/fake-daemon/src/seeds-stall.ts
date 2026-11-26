// The founder's Requests row for a stalled agent, as the daemon raises it when no agent opened the work waiting for it.
import type { FakeDaemon } from '#src/daemon.ts';

export function stallNotice(d: FakeDaemon, agent: string): void {
  d.tasks.warnings.push({
    task: null,
    title: `@${agent} is STALLED: 3 starts for the work waiting for it failed. The last said: the claude binary exited with status 1.`,
    kind: 'warning',
    source: 'machine',
    id: `notice:${agent}:start_stall`,
    agent: null,
    ask: null,
    warning: { kind: 'notice', agent, stall: true },
    items: [],
    unread: 0,
    first_unread: null,
    last_at: new Date(Date.now() - 5 * 60_000).toISOString(),
  });
}

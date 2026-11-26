// The fake daemon's schedules: schedules.list over the rows a seed or a test puts in. Read-only, as the daemon's is: a schedule comes
// to exist by a proposal, which the fake does not apply.
import type { Methods } from '@handline/contract';
import { Failure } from '#src/errors.ts';

type Reply = Methods['schedules.list']['result'];
export type FakeSchedule = Reply['schedules'][number];

/** The machine's zone as the daemon gives it: its short name and its minutes east of UTC now. */
function zoneOf(now: Date): Reply['zone'] {
  const name = new Intl.DateTimeFormat('en-US', { timeZoneName: 'short' }).formatToParts(now).find((p) => p.type === 'timeZoneName');
  return { name: name?.value ?? 'UTC', offset_min: -now.getTimezoneOffset() };
}

export class FakeSchedules {
  /** Every agent's schedules, in the order the list gives them. */
  rows: FakeSchedule[] = [];
  /** The agents whose stored value cannot be read. */
  unreadable: Reply['unreadable'] = [];
  private host: { now: () => Date; sessions: ReadonlyMap<string, unknown> };

  constructor(host: { now: () => Date; sessions: ReadonlyMap<string, unknown> }) {
    this.host = host;
  }

  owns = (method: string): boolean => method === 'schedules.list';

  call(p: Record<string, unknown>): Reply {
    const zone = zoneOf(this.host.now());
    if (p.agent === undefined) return { schedules: this.rows, unreadable: this.unreadable, zone };
    const agent = p.agent;
    if (typeof agent !== 'string' || !this.host.sessions.has(agent))
      throw new Failure('not_found', `no such agent: ${String(agent)}`, { what: 'agent', name: agent });
    return {
      schedules: this.rows.filter((r) => r.agent === agent),
      unreadable: this.unreadable.filter((u) => u.agent === agent),
      zone,
    };
  }
}

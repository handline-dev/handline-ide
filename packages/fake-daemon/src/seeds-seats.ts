// Seeds for an agent's seats: an agent at rest while its seats work.
import type { FakeDaemon } from '#src/daemon.ts';
import { task } from '#src/seedkit.ts';
import { newSession } from '#src/session.ts';

const ME = 'human';

export const SEAT_SEEDS: Record<string, (d: FakeDaemon) => void> = {
  /** handline.auditor rests while two of its seats (a5, a6) work on tasks of their own: its row counts them and lists them. */
  'seats-busy': (d) => {
    const ago = (m: number) => new Date(Date.now() - m * 60_000).toISOString();
    const own = d.sessions.get('handline.auditor')!;
    d.sessions.set('handline.auditor', newSession({ ...own.fx, state: 'idle', wall: undefined, task: null }, ago(30)));
    for (const [seat, id, title] of [
      ['a5', 9470, 'Settings page spacing'],
      ['a6', 9471, 'Dark mode on Usage'],
    ] as const) {
      task(d, id, { id, from: ME, to: seat, title, state: 'doing' });
      d.sessions.set(
        seat,
        newSession(
          { ...own.fx, name: seat, state: 'running', wall: undefined, task: { id, title }, view: { hired_from: 'handline.auditor' } },
          ago(4),
        ),
      );
    }
  },
};

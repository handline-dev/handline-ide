// AgentView.waits_on as the daemon reads it from the board.
import type { AgentView } from '@handline/contract';
import type { Row } from '#src/taskstore.ts';

type WaitsOn = NonNullable<AgentView['waits_on']>;

/** The first task an agent's held tasks all wait on, its holder and how many more; null when one held task waits on nothing. */
export function waitsOn(rows: readonly Row[], agent: string, current: number | null, open: (id: number) => boolean): WaitsOn | null {
  const live = rows.filter((t) => open(t.id));
  const acting = (t: Row) => t.acting ?? t.to;
  const held = live.filter((t) => (t.to === agent && (t.id === current || t.state !== 'draft')) || (acting(t) === agent && t.to !== agent));
  const found: { task: number; holder: string | null }[] = [];
  const add = (task: number, holder: string) => {
    if (!found.some((f) => f.task === task)) found.push({ task, holder: holder === 'gate' ? null : holder });
  };
  for (const t of held) {
    if (acting(t) !== agent) {
      add(t.id, acting(t));
      continue;
    }
    const kids = live.filter((r) => r.parent === t.id).sort((a, b) => (a.priority ?? 2) - (b.priority ?? 2) || a.id - b.id);
    const on = [
      ...t.blockers
        .map((b) => b.task)
        .filter(open)
        .sort((a, b) => a - b),
      ...kids.map((r) => r.id),
    ];
    if (!on.length) return null;
    for (const id of on) {
      const b = live.find((r) => r.id === id)!;
      const holder = b.state === 'draft' && !b.acting ? 'human' : acting(b);
      if (holder !== agent) add(id, holder);
    }
  }
  return found.length ? { ...found[0]!, more: found.length - 1 } : null;
}

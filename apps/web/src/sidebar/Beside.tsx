// An agent's row with its seats: its own mark (a seat's when that seat waits on the person), and the tasks they run beside its own.
import { withSeats } from '@handline/app-core';
import { useRead } from '@handline/app-data';
import type { AgentView } from '@handline/client';
import { Layers } from 'lucide-react';
import { useMemo } from 'react';
import { AlongsideStrip } from '#src/chat/agent/AlongsideStrip.tsx';

export type Merged = AgentView & { beside?: AgentView[] };

/** Each agent as its row shows it: a seat's state facts when that seat waits on the person, and the seats at work. */
export function useWithSeats(agents: readonly AgentView[]): Merged[] {
  const all = useRead('agents.list', {}).data?.agents;
  return useMemo(() => {
    const seats = new Map<string, AgentView[]>();
    for (const s of all ?? []) if (s.hired_from) seats.set(s.hired_from, [...(seats.get(s.hired_from) ?? []), s]);
    return agents.map((a) => {
      const { lead, beside } = withSeats(a, seats.get(a.key) ?? []);
      const facts =
        lead === a
          ? {}
          : {
              state: lead.state,
              has_dialog: lead.has_dialog,
              handoff: lead.handoff,
              status: lead.status,
              wall_reason: lead.wall_reason,
              state_detail: lead.state_detail,
              state_since: lead.state_since,
            };
      return beside.length ? { ...a, ...facts, beside } : { ...a, ...facts };
    });
  }, [agents, all]);
}

/** The trail's count of tasks run beside the agent's own. */
export function BesideCount({ n }: { n: number }) {
  return (
    <span className="flex items-center gap-0.5" title="tasks alongside">
      <Layers size={12} />
      {n}
    </span>
  );
}

/** The hover card's list of those tasks, as the chat's Working alongside strip draws them. */
export function BesideList({ seats }: { seats: AgentView[] }) {
  const running = seats.flatMap((s) => (s.task ? [{ task: s.task.id, at: s.state_since }] : []));
  return <AlongsideStrip running={running} />;
}

// At the end of a chat whose agent asked the founder for something (a proposal: run a command, change a file): what it asks, why, and
// Deny or Run here, the same answers its Requests row gives.
import { proposalTitle } from '@handline/app-core';
import { Hooks, useNameOf, useProposals } from '@handline/app-data';
import { Lightbulb } from 'lucide-react';
import { TurnQuick } from '#src/requests/TurnQuick.tsx';

export function ProposalCards({ agent }: { agent: string }) {
  const listed = useProposals().data?.proposals ?? [];
  const asked = (id: string) => listed.find((x) => String(x.id) === id && x.requested_by === agent);
  const turns = Hooks.useRequestsPending().data?.answer.filter((x) => x.source === 'proposal' && x.id && asked(x.id)) ?? [];
  const nameOf = useNameOf();
  return turns.map((t) => {
    const p = asked(t.id!);
    const command = p?.kind === 'run' ? (p.changes?.[0]?.diff ?? p.diff) : null;
    return (
      <div key={t.id} data-proposal={t.id} className="mt-2 flex max-w-150 flex-col gap-3 rounded-lg border border-border p-3 text-sm">
        <div className="flex gap-2.5">
          <Lightbulb size={16} className="mt-0.5 shrink-0 text-warning" />
          <div className="flex min-w-0 flex-col gap-1">
            <p className="text-fg">{p ? proposalTitle(p, nameOf) : t.title}</p>
            {command && <code className="block truncate rounded bg-surface px-1.5 py-0.5 font-mono text-xs text-fg">{command}</code>}
            {p?.reason && <p className="text-muted">{p.reason}</p>}
          </div>
        </div>
        <div className="flex gap-2 pl-6">
          <TurnQuick p={t} />
        </div>
      </div>
    );
  });
}

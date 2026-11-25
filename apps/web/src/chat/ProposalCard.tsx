// At the end of a chat whose agent asked the founder for something (a proposal: run a command, change a file or a setting): what it
// asks, why, exactly what changes (the command, each change's lines, a file's text before and after), and Deny or Run here, the same
// answers its Requests row gives.
import { changeRows, plainRefs, proposalTitle } from '@handline/app-core';
import { Hooks, useDirectory, useNameOf, useProposals } from '@handline/app-data';
import type { Proposal } from '@handline/client';
import { ArrowRight, Lightbulb } from 'lucide-react';
import { useState } from 'react';
import { TextDiff } from '#src/requests/ProposalView.tsx';
import { TurnQuick } from '#src/requests/TurnQuick.tsx';

/** Lines shown before a long change folds into "Show N more lines". */
const SHOWN = 6;

export function ProposalCards({ agent }: { agent: string }) {
  const listed = useProposals().data?.proposals ?? [];
  const asked = (id: string) => listed.find((x) => String(x.id) === id && x.requested_by === agent);
  const turns = Hooks.useRequestsPending().data?.answer.filter((x) => x.source === 'proposal' && x.id && asked(x.id)) ?? [];
  const nameOf = useNameOf();
  return turns.map((t) => {
    const p = asked(t.id!)!;
    return (
      <div key={t.id} data-proposal={t.id} className="mt-2 flex flex-col gap-3 rounded-lg border border-border p-3 text-sm">
        <div className="flex gap-2.5">
          <Lightbulb size={16} className="mt-0.5 shrink-0 text-warning" />
          <div className="flex min-w-0 flex-col gap-0.5">
            <p className="font-medium text-fg">{proposalTitle(p, nameOf)}</p>
            {p.reason && <p className="text-muted">{p.reason}</p>}
          </div>
        </div>
        <Changes p={p} />
        <div className="flex justify-end gap-2">
          <TurnQuick p={t} />
        </div>
      </div>
    );
  });
}

/** Each change a yes makes, full width: a file's text before and after, any other its own lines. */
function Changes({ p }: { p: Proposal }) {
  const nameOf = useNameOf();
  const dir = useDirectory();
  const steps = p.changes?.length ? p.changes : [{ kind: p.kind ?? '', agent: p.agent, diff: p.diff }];
  return (
    <div className="flex flex-col gap-2">
      {steps.map((c, i) =>
        c.after != null ? (
          <div key={i} className="flex flex-col gap-1">
            <p className="text-xs text-muted">
              {nameOf(c.agent)}'s {c.file}: removed lines in red, added in green
            </p>
            <TextDiff before={c.before ?? ''} after={c.after} />
          </div>
        ) : c.kind === 'run' ? (
          <Lines
            key={i}
            lines={plainRefs(dir, c.diff)
              .split('\n')
              .filter((l) => l.trim())}
          />
        ) : (
          <Rows key={i} diff={plainRefs(dir, c.diff)} />
        ),
      )}
    </div>
  );
}

/** A change's lines in a code block; past SHOWN lines the rest fold under one button. */
function Lines({ lines }: { lines: string[] }) {
  const [open, setOpen] = useState(false);
  const hidden = open ? 0 : Math.max(0, lines.length - SHOWN);
  return (
    <div className="overflow-hidden rounded-md border border-border bg-surface">
      <pre className="overflow-x-auto px-3 py-2 font-mono text-xs leading-5 whitespace-pre text-fg">
        {(hidden ? lines.slice(0, SHOWN) : lines).join('\n')}
      </pre>
      {hidden > 0 && (
        <button
          type="button"
          onClick={() => setOpen(true)}
          className="w-full border-t border-border px-3 py-1 text-left text-xs text-muted hover:text-fg"
        >
          Show {hidden} more {hidden === 1 ? 'line' : 'lines'}
        </button>
      )}
    </div>
  );
}

/** A setting change as a person reads it: what changes, the old value struck through when the change names it, an arrow, the new. */
function Rows({ diff }: { diff: string }) {
  return (
    <div className="flex flex-col divide-y divide-border rounded-md border border-border">
      {changeRows(diff).map((r, k) =>
        'text' in r ? (
          <p key={k} className="px-3 py-2 text-fg">
            {r.text}
          </p>
        ) : (
          <p key={k} className="flex flex-wrap items-center gap-2 px-3 py-2">
            <span className="w-24 shrink-0 text-muted">{r.what}</span>
            {r.from && <span className="text-muted line-through">{r.from}</span>}
            {r.from && <ArrowRight size={14} className="text-muted" />}
            {!r.from && <span className="text-muted">set to</span>}
            <span className="font-medium text-fg">{r.to}</span>
          </p>
        ),
      )}
    </div>
  );
}

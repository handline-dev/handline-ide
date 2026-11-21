// An HR proposal read whole: what it does, who asked and why, each change, and Allow or Deny while it waits on the founder.
import { proposalTitle } from '@handline/app-core';
import { Hooks, useNameOf, useProposals } from '@handline/app-data';
import { TurnQuick } from '#src/requests/TurnQuick.tsx';
import { ago } from '#src/format/time.ts';
import { DiffLines, diffLines, type Line } from '#src/tools/TaskDiff.tsx';

/** Changed lines with two unchanged lines around each; a longer unchanged run reads as one line saying how many it hides. */
function folded(lines: Line[]): (Line | { hidden: number })[] {
  const changed = (i: number) => lines.slice(Math.max(0, i - 2), i + 3).some((l) => l.kind !== 'same');
  // A lone unchanged line between two kept ones is drawn: its fold line would take the same room.
  const near = lines.map((_, i) => changed(i) || (i > 0 && i < lines.length - 1 && changed(i - 1) && changed(i + 1)));
  const out: (Line | { hidden: number })[] = [];
  lines.forEach((l, i) => {
    if (near[i]) return out.push(l);
    const last = out.at(-1);
    if (last && 'hidden' in last) last.hidden++;
    else out.push({ hidden: 1 });
  });
  return out;
}

const rows = (diff: string) => diff.split('\n').filter((l) => l.trim());

function TextDiff({ before, after }: { before: string; after: string }) {
  return (
    <div className="flex flex-col overflow-hidden rounded-md border border-border font-mono text-xs leading-6">
      {folded(diffLines(before, after)).map((l, k) =>
        'hidden' in l ? (
          <span key={k} className="bg-muted/10 px-3 font-sans text-muted">
            {l.hidden} unchanged {l.hidden === 1 ? 'line' : 'lines'}
          </span>
        ) : (
          <DiffLines key={k} lines={[l]} />
        ),
      )}
    </div>
  );
}

export function ProposalView({ id }: { id: number }) {
  const nameOf = useNameOf();
  const list = useProposals();
  const p = list.data?.proposals.find((x) => x.id === id);
  // The answer buttons are the row's own, from the founder's turn for this proposal.
  const turn = Hooks.useRequestsPending().data?.answer.find((x) => x.source === 'proposal' && x.id === String(id));
  if (!list.data) return null;
  if (!p) return <p className="m-auto text-sm text-muted">This proposal is not listed.</p>;
  // Each step approving makes: a context step as its file's text before and after, any other as its own lines.
  const steps = p.changes?.length ? p.changes : [{ kind: p.kind ?? '', agent: p.agent, diff: p.diff }];
  return (
    <div className="h-full overflow-auto">
      <div className="mx-auto flex max-w-3xl flex-col gap-5 px-6 py-5 text-sm">
        <div className="flex items-start gap-3">
          <div className="min-w-0 flex-1">
            <h1 className="text-lg font-semibold text-fg">{proposalTitle(p, nameOf)}</h1>
            <p className="text-xs text-muted">
              {[p.requested_by && `Asked by ${nameOf(p.requested_by)}`, ago(p.created), p.state !== 'open' && p.state]
                .filter(Boolean)
                .join(' · ')}
            </p>
          </div>
          {turn && (
            <div className="flex shrink-0 gap-2">
              <TurnQuick p={turn} />
            </div>
          )}
        </div>
        {p.reason && (
          <section className="flex flex-col gap-1">
            <h2 className="text-xs font-medium text-muted">Why</h2>
            <p className="whitespace-pre-wrap text-fg">{p.reason}</p>
          </section>
        )}
        <section className="flex flex-col gap-1">
          <h2 className="text-xs font-medium text-muted">What changes</h2>
          {steps.map((c, i) =>
            c.after != null ? (
              <div key={i} className="flex flex-col gap-1">
                <p className="text-xs text-muted">
                  {c.file} · {nameOf(c.agent)}
                </p>
                <TextDiff before={c.before ?? ''} after={c.after} />
              </div>
            ) : (
              <ul key={i} className="flex flex-col rounded-md border border-border">
                {rows(c.diff).map((l, k) => (
                  <li key={k} className="border-t border-border px-3 py-2 font-mono text-xs first:border-t-0">
                    {l}
                  </li>
                ))}
              </ul>
            ),
          )}
        </section>
      </div>
    </div>
  );
}

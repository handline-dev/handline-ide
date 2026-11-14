// Requests: what waits on the founder (their turn, notes, rows put off) and what they decided today. Their requests in progress and
// finished are in Tasks (My requests). The bell's popover previews the turn and the finished.
import { ListLoading } from '#src/ui/Loading.tsx';
import { useEffect, useState, type ReactNode } from 'react';
import type { Task } from '@handline/client';
import { useNameOf, useTasks } from '@handline/app-data';
import { clock } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { laterWords, pickAllState, turnKey, turnRow, type Picked } from '@handline/app-core';
import { useRequests } from '#src/requests/data.ts';
import { BulkButtons, PickProvider, usePick } from '#src/requests/RowMenu.tsx';
import { TurnLine } from '#src/requests/rows.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { selectionRoom, WithSelectionBar } from '#src/ui/SelectionBar.tsx';

/** A section of the list. Its title holds a box that picks every row of it, beside picks in other sections. */
function Section({
  title,
  rows,
  empty,
  action,
  children,
}: {
  title: string;
  rows: Picked[];
  empty: string;
  action?: ReactNode;
  children?: ReactNode;
}) {
  const pick = usePick();
  const all = pick ? pickAllState(rows, pick.picked) : 'none';
  return (
    <section className="flex flex-col">
      {/* The section's name stays under the tab bar while its rows pass below it. */}
      <h2 className="sticky top-0 z-10 flex min-h-9 items-center gap-2.5 border-b border-border bg-bg px-2 text-sm font-medium text-fg">
        {pick && rows.length > 0 && (
          <input
            type="checkbox"
            aria-label={`Pick every row of ${title}`}
            title={`Pick every row of ${title}`}
            checked={all === 'all'}
            ref={(box) => {
              if (box) box.indeterminate = all === 'some';
            }}
            onChange={() => pick.pickAll(rows, all !== 'all')}
            className="size-4 accent-accent"
          />
        )}
        <span>{title}</span>
        <span className="flex-1 text-muted tabular-nums">{rows.length > 0 ? rows.length : ''}</span>
        {action}
      </h2>
      {rows.length > 0 ? <div className="flex flex-col">{children}</div> : <p className="px-2 py-3 text-muted">{empty}</p>}
    </section>
  );
}

export function RequestsTab() {
  return (
    <PickProvider>
      <Requests />
    </PickProvider>
  );
}

function Requests() {
  const r = useRequests();
  const pick = usePick();
  const rows = {
    turn: r.turn.map((p) => turnRow(p)),
    notes: r.notes.map((p) => turnRow(p)),
    later: r.later.map((x) => turnRow(x.p)),
  };
  // A picked row that leaves the list (answered, or done elsewhere) is not picked any more.
  const listed = Object.values(rows)
    .flat()
    .map((x) => x.key)
    .join('|');
  useEffect(() => pick?.keep(new Set(listed.split('|'))), [listed]); // eslint-disable-line react-hooks/exhaustive-deps
  const n = pick?.picked.size ?? 0;
  return (
    <WithSelectionBar count={n} onClear={() => pick?.clear()} actions={<BulkButtons />} className="flex-1">
      <ScrollFade keep="requests" className={`min-h-0 flex-1 ${selectionRoom(n)}`}>
        <Page className="flex flex-col gap-6 py-4 text-sm">
          {r.failed && <p className="px-2 text-danger">Requests did not load.</p>}
          {!r.loaded && !r.failed && <ListLoading label="Loading requests" />}
          {r.loaded && (
            <>
              <Section title="Your turn" rows={rows.turn} empty="Nothing waits on you.">
                {r.turn.map((p) => (
                  <TurnLine key={turnKey(p)} p={p} full />
                ))}
              </Section>
              {r.notes.length > 0 && (
                <Section title="Notes" rows={rows.notes} empty="">
                  {r.notes.map((p) => (
                    <TurnLine key={turnKey(p)} p={p} full />
                  ))}
                </Section>
              )}
              {r.later.length > 0 && (
                <Section title="Later" rows={rows.later} empty="">
                  {r.later.map(({ p, at }) => (
                    <TurnLine key={turnKey(p)} p={p} until={laterWords(at)} full />
                  ))}
                </Section>
              )}
              <Decided />
            </>
          )}
        </Page>
      </ScrollFade>
    </WithSelectionBar>
  );
}

/** What the founder decided today, newest first: per task, the question, their answer, and what came of it. */
function Decided() {
  const [since] = useState(() => {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    return d.toISOString();
  });
  const q = useTasks({ decided_since: since });
  const nameOf = useNameOf();
  const items = (q.data?.items ?? []).filter(isDecided).sort((a, b) => b.decided.at.localeCompare(a.decided.at));
  if (!items.length) return <Section title="Decided today" rows={[]} empty={q.isPending ? '' : 'Nothing decided today.'} />;
  return (
    <section className="flex flex-col">
      <h2 className="sticky top-0 z-10 flex min-h-9 items-center border-b border-border bg-bg px-2 text-sm font-medium text-fg">
        Decided today
      </h2>
      <div className="flex flex-col">
        {items.map((t) => (
          <DecidedLine key={t.id} t={t} who={nameOf(t.to)} />
        ))}
      </div>
    </section>
  );
}

type DecidedTask = Task & { decided: NonNullable<Task['decided']> };
const isDecided = (t: Task): t is DecidedTask => !!t.decided;

function DecidedLine({ t, who }: { t: DecidedTask; who: string }) {
  const open = useOpenable(taskOf(t.id));
  const after = t.state === 'done' ? 'done' : t.state === 'discarded' ? 'discarded' : `${who} is on it`;
  return (
    <button type="button" {...open} className="flex w-full gap-2.5 border-b border-border px-2 py-3 text-left hover:bg-hover/30">
      <span className="w-14 shrink-0 text-muted tabular-nums">T{t.id}</span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex min-w-0 items-baseline gap-2">
          <span className="truncate font-medium text-fg">{t.title}</span>
          <span className="shrink-0 text-muted">{who}</span>
          <span className="ml-auto shrink-0 pl-3 text-xs text-muted tabular-nums">{clock(t.decided.at)}</span>
        </span>
        <span className="truncate text-muted">
          You “{t.decided.answer.replace(/\s+/g, ' ')}” → {after}
        </span>
      </span>
    </button>
  );
}

// Requests: the founder's turn, their requests in progress, and those finished. The bell's popover is a preview of this page.
import { useEffect, type ReactNode } from 'react';
import { laterWords, pickAllState, requestRow, turnKey, turnRow, unseenOf, type Picked } from '@handline/app-core';
import { Eye } from 'lucide-react';
import { useRequests } from '#src/requests/data.ts';
import { BulkButtons, PickProvider, usePick, useRowActions } from '#src/requests/RowMenu.tsx';
import { RequestLine, TurnLine } from '#src/requests/rows.tsx';
import { Button } from '#src/ui/Button.tsx';
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
  children: ReactNode;
}) {
  const pick = usePick();
  const all = pick ? pickAllState(rows, pick.picked) : 'none';
  return (
    <section className="flex flex-col gap-1">
      <h2 className="flex min-h-7 items-center gap-2.5 px-2 text-xs font-medium text-muted">
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
        {rows.length > 0 && <span className="flex-1 tabular-nums">{rows.length}</span>}
        {action}
      </h2>
      {rows.length > 0 ? children : <p className="px-2 py-1.5 text-muted">{empty}</p>}
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
  const act = useRowActions();
  const rows = {
    turn: r.turn.map((p) => turnRow(p)),
    later: r.later.map((x) => turnRow(x.p)),
    doing: r.doing.map(requestRow),
    done: r.done.map(requestRow),
  };
  const unseen = unseenOf(rows.done);
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
          <h1 className="px-2 text-lg font-semibold">Requests</h1>
          {r.failed && <p className="px-2 text-danger">Requests did not load.</p>}
          {r.loaded && (
            <>
              <Section title="Your turn" rows={rows.turn} empty="Nothing waits on you.">
                {r.turn.map((p) => (
                  <TurnLine key={turnKey(p)} p={p} />
                ))}
              </Section>
              {r.later.length > 0 && (
                <Section title="Later" rows={rows.later} empty="">
                  {r.later.map(({ p, at }) => (
                    <TurnLine key={turnKey(p)} p={p} until={laterWords(at)} />
                  ))}
                </Section>
              )}
              <Section title="In progress" rows={rows.doing} empty="No request is being worked on.">
                {r.doing.map((x) => (
                  <RequestLine key={x.cursor} r={x} />
                ))}
              </Section>
              <Section
                title="Finished"
                rows={rows.done}
                empty="Nothing finished in the last 30 days."
                action={
                  unseen.length > 0 && (
                    <Button variant="ghost" size="sm" onClick={() => void act.seen(unseen)}>
                      <Eye size={14} />
                      Mark all seen
                    </Button>
                  )
                }
              >
                {r.done.map((x) => (
                  <RequestLine key={x.cursor} r={x} />
                ))}
              </Section>
            </>
          )}
        </Page>
      </ScrollFade>
    </WithSelectionBar>
  );
}

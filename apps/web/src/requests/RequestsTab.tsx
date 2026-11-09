// Requests: the founder's turn, their requests in progress, and those finished. The bell's popover is a preview of this page.
import { useEffect, type ReactNode } from 'react';
import { laterWords, listedRows, pickAllState, turnKey, unseenOf, type Picked } from '@handline/app-core';
import { Eye } from 'lucide-react';
import { useRequests } from '#src/requests/data.ts';
import { BulkButtons, PickProvider, usePick, useRowActions } from '#src/requests/RowMenu.tsx';
import { RequestLine, TurnLine } from '#src/requests/rows.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { selectionRoom, WithSelectionBar } from '#src/ui/SelectionBar.tsx';

function Section({ title, count, empty, children }: { title: string; count: number; empty: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-1">
      <h2 className="flex items-center gap-2 px-2 text-xs font-medium text-muted">
        {title}
        {count > 0 && <span className="tabular-nums">{count}</span>}
      </h2>
      {count > 0 ? children : <p className="px-2 py-1.5 text-muted">{empty}</p>}
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

/** The list's head: a box that picks every row shown, and a way to clear away every finished row without picking. */
function Head({ shown }: { shown: Picked[] }) {
  const pick = usePick();
  const act = useRowActions();
  const unseen = unseenOf(shown);
  const all = pick ? pickAllState(shown, pick.picked) : 'none';
  return (
    <div className="flex items-center gap-2.5 px-2">
      {shown.length > 0 && (
        <input
          type="checkbox"
          aria-label="Pick every row"
          title="Pick every row"
          checked={all === 'all'}
          ref={(box) => {
            if (box) box.indeterminate = all === 'some';
          }}
          onChange={() => (all === 'all' ? pick?.clear() : pick?.pickAll(shown))}
          className="size-4 accent-accent"
        />
      )}
      <h1 className="flex-1 text-lg font-semibold">Requests</h1>
      {unseen.length > 0 && (
        <Button variant="ghost" onClick={() => void act.seen(unseen)}>
          <Eye size={14} />
          Mark all finished seen
        </Button>
      )}
    </div>
  );
}

function Requests() {
  const r = useRequests();
  const pick = usePick();
  // A picked row that leaves the list (answered, or done elsewhere) is not picked any more.
  const shown = listedRows([...r.turn, ...r.later.map((x) => x.p)], [...r.doing, ...r.done]);
  const listed = shown.map((x) => x.key).join('|');
  useEffect(() => pick?.keep(new Set(listed.split('|'))), [listed]); // eslint-disable-line react-hooks/exhaustive-deps
  const n = pick?.picked.size ?? 0;
  return (
    <WithSelectionBar count={n} onClear={() => pick?.clear()} actions={<BulkButtons />} className="flex-1">
      <ScrollFade keep="requests" className={`min-h-0 flex-1 ${selectionRoom(n)}`}>
        <Page className="flex flex-col gap-6 py-4 text-sm">
          <Head shown={shown} />
          {r.failed && <p className="px-2 text-danger">Requests did not load.</p>}
          {r.loaded && (
            <>
              <Section title="Your turn" count={r.turn.length} empty="Nothing waits on you.">
                {r.turn.map((p) => (
                  <TurnLine key={turnKey(p)} p={p} />
                ))}
              </Section>
              {r.later.length > 0 && (
                <Section title="Later" count={r.later.length} empty="">
                  {r.later.map(({ p, at }) => (
                    <TurnLine key={turnKey(p)} p={p} until={laterWords(at)} />
                  ))}
                </Section>
              )}
              <Section title="In progress" count={r.doing.length} empty="No request is being worked on.">
                {r.doing.map((x) => (
                  <RequestLine key={x.cursor} r={x} />
                ))}
              </Section>
              <Section title="Finished" count={r.done.length} empty="Nothing finished in the last 30 days.">
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

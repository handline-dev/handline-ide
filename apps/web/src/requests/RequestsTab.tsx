// Requests: the founder's turn, their requests in progress, and those finished. The bell's popover is a preview of this page.
import { useEffect, type ReactNode } from 'react';
import { laterWords } from '@handline/app-core';
import { useRequests } from '#src/requests/data.ts';
import { BulkItems, PickProvider, requestKey, turnKey, usePick } from '#src/requests/RowMenu.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Menu } from '#src/ui/Menu.tsx';
import { RequestLine, TurnLine } from '#src/requests/rows.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';

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

/** Over the list while rows are picked: how many, and what can be done to them all. */
function PickedBar() {
  const pick = usePick();
  const n = pick?.picked.size ?? 0;
  return (
    // Its row is kept, so picking the first row does not move the list.
    <div className="flex h-8 items-center gap-2 px-2">
      <h1 className={`text-lg font-semibold ${n ? 'hidden' : ''}`}>Requests</h1>
      {n > 0 && (
        <>
          <span className="text-fg">{`${n} picked`}</span>
          <Menu trigger={<Button>Act on them</Button>}>
            <BulkItems />
          </Menu>
          <span className="text-xs text-muted">Shift+click picks more</span>
        </>
      )}
    </div>
  );
}

function Requests() {
  const r = useRequests();
  const pick = usePick();
  // A picked row that leaves the list (answered, or done elsewhere) is not picked any more.
  const listed = [...r.turn, ...r.later.map((x) => x.p)]
    .map(turnKey)
    .concat([...r.doing, ...r.done].map(requestKey))
    .join('|');
  useEffect(() => pick?.keep(new Set(listed.split('|'))), [listed]); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <ScrollFade keep="requests" className="min-h-0 flex-1">
      <Page className="flex flex-col gap-6 py-4 text-sm">
        <PickedBar />
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
  );
}

// Requests: the founder's turn, their requests in progress, and those finished. The bell's popover is a preview of this page.
import type { ReactNode } from 'react';
import { useRequests } from '#src/requests/data.ts';
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
  const r = useRequests();
  return (
    <ScrollFade keep="requests" className="min-h-0 flex-1">
      <Page className="flex flex-col gap-6 py-4 text-sm">
        <h1 className="px-2 text-lg font-semibold">Requests</h1>
        {r.failed && <p className="px-2 text-danger">Requests did not load.</p>}
        {r.loaded && (
          <>
            <Section title="Your turn" count={r.turn.length} empty="Nothing waits on you.">
              {r.turn.map((p) => (
                <TurnLine key={p.task ?? `item ${p.items[0]}`} p={p} />
              ))}
            </Section>
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

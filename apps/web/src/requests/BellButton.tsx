import { UnreadDot } from '#src/ui/Dot.tsx';
import { Inbox } from 'lucide-react';
import { useOpenable } from '#src/layout/openable.tsx';
import { useRequests } from '#src/requests/data.ts';
import { Across, InCluster, type Of } from '#src/connection/clusters.tsx';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { TurnLine } from '#src/requests/rows.tsx';
import { ServiceRow, useServiceDown } from '#src/requests/ServiceRow.tsx';

const SHOWN = 5;

/** The Requests mark (its tab's inbox), the one way into Requests: its number is what waits on the founder's action in every cluster; a click opens Requests
 * as a tab, by the app's one scheme, and resting on it shows the first Your turn rows. A dot marks an unread note or notice, or a finished request not yet seen. */
export function BellButton() {
  return (
    <Across hook={useRequests} sig={(r) => JSON.stringify([r.count, r.notes.length, r.unseen, r.turn.slice(0, SHOWN)])}>
      {(all) => <Bell all={all} />}
    </Across>
  );
}

function Bell({ all }: { all: Of<ReturnType<typeof useRequests>>[] }) {
  const down = useServiceDown();
  const count = all.reduce((n, a) => n + a.data.count, 0) + (down ? 1 : 0);
  const fresh = all.some((a) => a.data.unseen > 0 || a.data.notes.length > 0);
  const open = useOpenable('requests');
  const rows = all.flatMap((a) => a.data.turn.map((p) => ({ cluster: a.cluster, p }))).slice(0, SHOWN);
  return (
    <HoverCard
      side="top"
      align="end"
      trigger={
        <button type="button" {...open} className="relative flex items-center gap-1" aria-label={`Requests, ${count} waiting on you`}>
          <Inbox size={14} className={count ? 'text-fg' : ''} />
          {count > 0 && <span className="text-fg tabular-nums">{count}</span>}
          {fresh && <UnreadDot label="new" className="absolute -top-0.5 -right-1" />}
        </button>
      }
    >
      <div className="flex w-80 flex-col text-sm">
        <p className="px-2 pt-1 pb-1 text-xs font-medium text-muted">Your turn</p>
        {down && <ServiceRow />}
        {rows.map(({ cluster, p }) => (
          <InCluster key={`${cluster}/${p.source}:${p.id ?? p.task}`} id={cluster}>
            <TurnLine p={p} />
          </InCluster>
        ))}
        {!down && rows.length === 0 && <p className="px-2 py-1.5 text-muted">Nothing waits on you</p>}
        <button type="button" {...open} className="mt-1 border-t border-border px-2 pt-2 pb-1 text-left text-muted hover:text-fg">
          Open Requests{count > rows.length + (down ? 1 : 0) ? ` · ${count} in all` : ''}
        </button>
      </div>
    </HoverCard>
  );
}

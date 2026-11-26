import { UnreadDot } from '#src/ui/Dot.tsx';
import { Inbox } from 'lucide-react';
import { useOpenable } from '#src/layout/openable.tsx';
import { useRequests } from '#src/requests/data.ts';
import { Across } from '#src/connection/clusters.tsx';

/** The Requests mark (its tab's inbox), the one way into Requests: its number is what waits on the founder's action in every cluster; a click opens Requests
 * as a tab, by the app's one scheme. A dot marks an unread note or notice, or a finished request not yet seen. */
export function BellButton() {
  return (
    <Across hook={useRequests} sig={(r) => JSON.stringify([r.count, r.notes.length, r.unseen])}>
      {(all) => {
        const count = all.reduce((n, a) => n + a.data.count, 0);
        const fresh = all.some((a) => a.data.unseen > 0 || a.data.notes.length > 0);
        return <BellCount count={count} fresh={fresh} />;
      }}
    </Across>
  );
}

function BellCount({ count, fresh }: { count: number; fresh: boolean }) {
  const open = useOpenable('requests');
  return (
    <button
      type="button"
      {...open}
      className="relative flex items-center gap-1"
      title="Requests"
      aria-label={`Requests, ${count} waiting on you`}
    >
      <Inbox size={14} className={count ? 'text-fg' : ''} />
      {count > 0 && <span className="text-fg tabular-nums">{count}</span>}
      {fresh && <UnreadDot label="new" className="absolute -top-0.5 -right-1" />}
    </button>
  );
}

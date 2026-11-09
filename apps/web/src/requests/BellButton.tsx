import { isUnseen, turnKey } from '@handline/app-core';
import { Bell } from 'lucide-react';
import { useState, type RefObject } from 'react';
import { goPage } from '#src/layout/openTab.tsx';
import { useRequests } from '#src/requests/data.ts';
import { RequestLine, TurnLine } from '#src/requests/rows.tsx';
import { Popover } from '#src/ui/Popover.tsx';

const SHOWN = 5;

/** The bell: its number is what waits on the founder, the same as the sidebar's Requests. A click opens a preview of Requests: the
 * first of the founder's turn and the finished ones not yet seen (else the latest), with a way to the whole page. A dot marks unseen results. */
export function BellButton({ bar }: { bar: RefObject<HTMLElement | null> }) {
  const r = useRequests();
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);
  const unseen = r.done.filter(isUnseen);
  const done = (unseen.length ? unseen : r.done).slice(0, Math.max(2, SHOWN - r.turn.length));
  return (
    <Popover
      side="top"
      align="end"
      corner={bar}
      open={open}
      onOpenChange={setOpen}
      trigger={
        <button type="button" className="relative flex items-center gap-1" aria-label={`Requests, ${r.count} waiting on you`}>
          <Bell size={14} className={r.count ? 'text-fg' : ''} />
          {r.count > 0 && <span className="text-fg tabular-nums">{r.count}</span>}
          {r.unseen > 0 && <span aria-label="new results" className="absolute -top-0.5 -right-1 size-1.5 rounded-full bg-accent" />}
        </button>
      }
    >
      <div className="-m-2 flex w-96 flex-col gap-2">
        <p className="px-2 pt-1 text-xs font-medium text-muted">Your turn</p>
        {!r.loaded ? null : r.turn.length ? (
          r.turn.slice(0, SHOWN).map((p) => <TurnLine key={turnKey(p)} p={p} onOpened={close} />)
        ) : (
          <p className="px-2 text-muted">Nothing waits on you.</p>
        )}
        {done.length > 0 && (
          <>
            <p className="border-t border-border px-2 pt-2 text-xs font-medium text-muted">Finished</p>
            {done.map((x) => (
              <RequestLine key={x.cursor} r={x} onOpened={close} />
            ))}
          </>
        )}
        <button
          type="button"
          onClick={() => (close(), goPage('requests', 'Requests'))}
          className="rounded-md border-t border-border px-2 py-1.5 text-left text-fg hover:bg-hover/30"
        >
          See all requests
        </button>
      </div>
    </Popover>
  );
}

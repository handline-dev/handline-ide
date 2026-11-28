// The nearby window: what the founder has stopped beside in an office (near.ts), in one window at a fixed spot over the picture's right
// edge (on a phone's width, a sheet under it), over any floating tab bar, with no tabs. It follows the founder from one thing to the next
// and closes as it walks off. Pin moves it into a tab bar docked on the left (Windows.tsx), whose tab bars stay as they are; beside something already there, that tab comes up
// instead. Esc or × shuts it until the founder stops beside something else.
import { Pin, X } from 'lucide-react';
import { useEffect, useEffectEvent, useState } from 'react';
import { tabIn } from '#src/connection/clusters.tsx';
import { agentOf } from '#src/layout/screenLayout.ts';
import { labelOf } from '#src/layout/kinds.tsx';
import { TabBody } from '#src/layout/TabBody.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { useEscape } from '#src/ui/useEscape.ts';
import { Face } from '#src/town/face.tsx';
import type { Founder } from '#src/town/founder.tsx';
import { WORDS } from '#src/town/model.ts';
import { nearOf, type Near } from '#src/town/near.ts';
import { useTownShown } from '#src/town/Windows.tsx';

/** What the founder is beside, kept as it walks (`sought`: what a click sent it to); its person as `things` has it now. */
export function useNear(founder: Founder, things: Near[], sought: Sought | null): Near | null {
  const [near, setNear] = useState<Near | null>(null);
  const look = useEffectEvent(() =>
    setNear((was) => {
      const now = nearOf(founder.get(), things, was, sought?.key);
      return now?.key === was?.key ? was : now;
    }),
  );
  useEffect(() => {
    // Where it already stands counts once the office is drawn, before it takes a step.
    const first = requestAnimationFrame(look);
    const off = founder.subscribe(look);
    return () => {
      cancelAnimationFrame(first);
      off();
    };
  }, [founder]);
  // A click on another thing on the spot the founder already stands at moves nobody: look again.
  useEffect(() => {
    const again = requestAnimationFrame(look);
    return () => cancelAnimationFrame(again);
  }, [sought]);
  return near && (things.find((t) => t.key === near.key) ?? null);
}

/** The nearby window's size beside the picture, in px. */
const NEAR_W = 360;
const NEAR_H = 460;

/** The thing a click last sent the founder to (near.ts thingsOf's key), and which click that was. */
export type Sought = { key: string; click: number };

/** `rows`: each person's figure on the people sheet, for its face; `sought`: the thing a click last sent the founder to. */
export function NearWindow(props: { founder: Founder; things: Near[]; rows: ReadonlyMap<string, number>; sought: Sought | null }) {
  const { founder, things, rows, sought } = props;
  const { show, pin, shows, keeps, narrow } = useTownShown();
  const near = useNear(founder, things, sought);
  // Shut by Esc or ×: stays shut while the founder is beside the same thing, until it is clicked again.
  const [shut, setShut] = useState<string | null>(null);
  const [heard, setHeard] = useState(sought);
  if (heard !== sought) {
    setHeard(sought);
    setShut(null);
  } else if (shut && shut !== near?.key) setShut(null);
  const pinned = !!near && keeps(near.opens);
  // Beside something already in a tab bar, that tab comes up, unless it is up already.
  const bringUp = useEffectEvent((key: string) => {
    const n = things.find((t) => t.key === key);
    if (n && !shows(n.opens)) show(n.opens, n.title);
  });
  const key = pinned ? near.key : null;
  useEffect(() => {
    if (key) bringUp(key);
  }, [key]);
  const open = !!near && !pinned && shut !== near.key;
  // Last, as in a tab bar: a key the chat takes first (stopping its agent, closing a list) does not shut the window.
  useEscape(open, () => near && setShut(near.key));
  if (!open) return null;
  const p = near.person;
  const row = p && rows.get(p.key);
  const { tab, cluster } = tabIn({ id: `near:${near.key}`, title: near.title, opens: near.opens });
  return (
    <div
      data-no-walk
      className={`pointer-events-auto absolute z-30 flex flex-col overflow-hidden border-border bg-bg ${
        narrow ? 'inset-x-0 bottom-0 h-1/2 border-t' : 'top-4 right-4 rounded-md border'
      }`}
      style={narrow ? undefined : { width: NEAR_W, height: NEAR_H, maxHeight: 'calc(100% - 2rem)' }}
    >
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border pr-1.5 pl-3 text-sm">
        {row !== undefined && <Face row={row} size={20} />}
        <span className="truncate font-medium">{p?.name ?? near.title}</span>
        {p && (
          <span className="flex shrink-0 items-center gap-1 text-xs text-muted">
            <ActivityIcon act={p.activity} />
            {WORDS[p.activity]}
          </span>
        )}
        <span className="flex-1" />
        <IconButton label="Pin" onClick={() => pin(near.opens, near.title)}>
          <Pin size={14} />
        </IconButton>
        <IconButton label="Close" onClick={() => setShut(near.key)}>
          <X size={14} />
        </IconButton>
      </div>
      <div className="relative flex min-h-0 flex-1">
        <TabBody
          tab={tab}
          cluster={cluster}
          who={agentOf(near.opens)}
          onOpen={(opens) => show(opens, labelOf(opens))}
          onChoose={(opens, title) => show(opens, title)}
        />
      </div>
    </div>
  );
}

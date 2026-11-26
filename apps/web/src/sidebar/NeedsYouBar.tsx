// An agent that needs the person whose row is scrolled out of sight in the sidebar: a bar at that edge of the list, its own row (one
// agent) or a count (several), that opens its chat and brings its row into view. Rows mark themselves (data-needs-you, Sidebar.tsx);
// which are out of sight, and at which edge, is app-core needsYouOut.
import { needsYouOut } from '@handline/app-core';
import { useAgents, useNameOf } from '@handline/app-data';
import { ArrowDown, ArrowUp } from 'lucide-react';
import { AnimatePresence, motion } from 'framer-motion';
import { useEffect, useRef, useState, type RefObject } from 'react';
import { holdLine } from '#src/layout/agentList.ts';
import { spring } from '#src/motion.ts';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { useRefs } from '#src/ui/refs.tsx';

type Out = { above: string[]; below: string[] };

/** Which marked agents are out of sight now, read again as the list scrolls, resizes or changes. */
function useOut(scroller: RefObject<HTMLDivElement | null>): Out {
  const [out, setOut] = useState<Out>({ above: [], below: [] });
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const read = () => {
      const view = el.getBoundingClientRect();
      const marks = [...el.querySelectorAll<HTMLElement>('[data-needs-you]')].map((m) => {
        const r = m.getBoundingClientRect();
        return { keys: (m.dataset.needsYou ?? '').split(' ').filter(Boolean), top: r.top, bottom: r.bottom };
      });
      const next = needsYouOut(marks, { top: view.top, bottom: view.bottom });
      setOut((o) => (JSON.stringify(o) === JSON.stringify(next) ? o : next));
    };
    read();
    el.addEventListener('scroll', read, { passive: true });
    const ro = new ResizeObserver(read);
    ro.observe(el);
    const mo = new MutationObserver(read);
    mo.observe(el, { subtree: true, childList: true, attributes: true, attributeFilter: ['data-needs-you'] });
    return () => {
      el.removeEventListener('scroll', read);
      ro.disconnect();
      mo.disconnect();
    };
  }, [scroller]);
  return out;
}

export function NeedsYouBar({ scroller, onSelect }: { scroller: RefObject<HTMLDivElement | null>; onSelect: (key: string) => void }) {
  const out = useOut(scroller);
  return (
    <AnimatePresence initial={false}>
      {out.above.length > 0 && <Bar key="above" keys={out.above} edge="top" scroller={scroller} onSelect={onSelect} />}
      {out.below.length > 0 && <Bar key="below" keys={out.below} edge="bottom" scroller={scroller} onSelect={onSelect} />}
    </AnimatePresence>
  );
}

function Bar({
  keys,
  edge,
  scroller,
  onSelect,
}: {
  keys: string[];
  edge: 'top' | 'bottom';
  scroller: RefObject<HTMLDivElement | null>;
  onSelect: (key: string) => void;
}) {
  const nameOf = useNameOf();
  const refs = useRefs();
  const agents = useAgents().data?.agents ?? [];
  // With several, each click goes to the next one.
  const next = useRef(0);
  const one = keys.length === 1 ? agents.find((a) => a.key === keys[0]) : undefined;
  const Arrow = edge === 'top' ? ArrowUp : ArrowDown;
  const show = () => {
    const key = keys[next.current++ % keys.length];
    onSelect(key);
    scroller.current?.querySelector(`[data-needs-you~="${CSS.escape(key)}"]`)?.scrollIntoView({ block: 'nearest' });
  };
  return (
    <motion.div
      initial={{ height: 0 }}
      animate={{ height: 'auto' }}
      exit={{ height: 0 }}
      transition={spring}
      className={`absolute inset-x-0 overflow-hidden bg-surface ${edge === 'top' ? 'top-0 border-b' : 'bottom-0 border-t'} border-border`}
    >
      <button
        type="button"
        onClick={show}
        aria-label={`Show ${keys.map((k) => nameOf(k)).join(', ')}`}
        title={`Show ${keys.map((k) => nameOf(k)).join(', ')}`}
        className="flex w-full flex-col px-4 py-1.5 text-left text-base outline-none hover:bg-hover/30 focus-visible:bg-hover"
      >
        <span className="flex h-5 items-center gap-1.5">
          <span className="grid size-4 shrink-0 place-items-center text-muted">
            <ActivityIcon act="you" />
          </span>
          <span className="flex-1 truncate text-fg">{one ? nameOf(one.key) : `${keys.length} agents need you`}</span>
          <Arrow size={14} className="shrink-0 text-muted" aria-hidden />
        </span>
        <span className="flex items-center gap-1.5 text-xs text-muted">
          <span className="w-4 shrink-0" />
          <span className="flex-1 truncate">{one ? refs(holdLine(one) ?? '') : keys.map((k) => nameOf(k)).join(', ')}</span>
        </span>
      </button>
    </motion.div>
  );
}

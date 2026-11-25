// A window floating over the town's picture at its right edge, under the bar above it: the chat of the agent the founder clicked, or the
// decisions on the founder's own desk. It slides in from the right and stays until Close; it is a fixed width until its left edge is
// dragged, and the width given is kept for the next.
import { motion } from 'framer-motion';
import { useRef, useState, type ReactNode } from 'react';
import { TabId } from '#src/persist/tabState.ts';

const WIDTH = 420;
const LEAST = 320;
let kept = WIDTH;

export function Floater({ title, sub, onClose, children }: { title: string; sub?: string; onClose: () => void; children: ReactNode }) {
  const [width, setWidth] = useState(kept);
  const drag = useRef<{ x: number; w: number } | null>(null);
  const most = () => Math.max(LEAST, window.innerWidth - 160);
  return (
    <motion.aside
      data-no-walk
      initial={{ x: 32, opacity: 0 }}
      animate={{ x: 0, opacity: 1 }}
      exit={{ x: 32, opacity: 0 }}
      transition={{ duration: 0.16, ease: 'easeOut' }}
      style={{ width }}
      className="absolute top-3 right-3 bottom-3 z-10 flex flex-col overflow-hidden rounded-lg border border-border bg-bg text-sm shadow-xl"
    >
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize"
        className="absolute inset-y-0 left-0 z-10 w-1.5 cursor-col-resize hover:bg-accent/40"
        onPointerDown={(e) => {
          e.currentTarget.setPointerCapture(e.pointerId);
          drag.current = { x: e.clientX, w: width };
        }}
        onPointerMove={(e) => {
          if (!drag.current) return;
          const w = Math.min(most(), Math.max(LEAST, drag.current.w + drag.current.x - e.clientX));
          kept = w;
          setWidth(w);
        }}
        onPointerUp={() => (drag.current = null)}
      />
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
        <span className="truncate font-medium">{title}</span>
        {sub && <span className="truncate text-xs text-muted">{sub}</span>}
        <span className="flex-1" />
        <button type="button" onClick={onClose} className="rounded-md px-1.5 text-muted hover:bg-hover hover:text-fg">
          Close
        </button>
      </div>
      {/* Not the Town tab's state: each window keeps its own while it is open. */}
      <TabId.Provider value={null}>
        <div className="flex min-h-0 flex-1 flex-col">{children}</div>
      </TabId.Provider>
    </motion.aside>
  );
}

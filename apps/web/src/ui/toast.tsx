import { AnimatePresence, motion } from 'framer-motion';
import { CircleAlert, Info, X } from 'lucide-react';
import { useSyncExternalStore, type ReactNode } from 'react';
import { spring } from '#src/motion.ts';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Refs } from '#src/ui/refs.tsx';

/** Toasts: `toast('...')` from anywhere; one <ToastHost /> at the root draws them at the bottom right.
 * Each leaves by itself after `ms` (4s), or by its X. */
type Tone = 'info' | 'warning';
type Entry = { id: number; message: ReactNode; tone: Tone };

let entries: Entry[] = [];
let nextId = 0;
const listeners = new Set<() => void>();
const set = (next: Entry[]) => {
  entries = next;
  listeners.forEach((l) => l());
};
const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};
const dismiss = (id: number) => set(entries.filter((e) => e.id !== id));

export function toast(message: ReactNode, { tone = 'info', ms = 4000 }: { tone?: Tone; ms?: number } = {}) {
  const id = ++nextId;
  set([...entries, { id, message, tone }]);
  setTimeout(() => dismiss(id), ms);
}

const icons: Record<Tone, ReactNode> = {
  info: <Info size={16} className="text-accent" />,
  warning: <CircleAlert size={16} className="text-warning" />,
};

export function ToastHost() {
  const list = useSyncExternalStore(subscribe, () => entries);
  return (
    <div role="status" aria-live="polite" className="pointer-events-none fixed right-4 bottom-4 z-50 flex w-80 flex-col items-end gap-2">
      <AnimatePresence initial={false}>
        {list.map((t) => (
          <motion.div
            key={t.id}
            layout
            initial={{ opacity: 0, y: 12, scale: 0.97 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, x: 24 }}
            transition={spring}
            className="pointer-events-auto flex w-full items-start gap-2 rounded-lg border border-border bg-bg p-3 text-sm text-fg shadow-popover"
          >
            <span className="mt-0.5 shrink-0">{icons[t.tone]}</span>
            <span className="flex-1">{typeof t.message === 'string' ? <Refs text={t.message} /> : t.message}</span>
            <IconButton size="sm" label="Dismiss" onClick={() => dismiss(t.id)}>
              <X size={12} />
            </IconButton>
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
}

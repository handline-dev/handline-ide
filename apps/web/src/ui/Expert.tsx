// The one fold for what only a developer sets: closed until opened, its rows under it.
import { ChevronRight, SlidersHorizontal } from 'lucide-react';
import { useState, type ReactNode } from 'react';

export function Expert({
  children,
  line = true,
}: {
  children: ReactNode;
  /** A border line above it, as at the end of a settings block. */ line?: boolean;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className={`flex w-full items-center gap-2 text-sm text-muted hover:text-fg ${line ? 'border-t border-border pt-3' : ''}`}
      >
        <ChevronRight size={14} className={`transition-transform ${open ? 'rotate-90' : ''}`} />
        <SlidersHorizontal size={14} />
        Expert
      </button>
      {open && children}
    </>
  );
}

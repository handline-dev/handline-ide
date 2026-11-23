// A command the app runs for the person, shown as one line that says it is going; its terminal opens below only when asked.
import { ChevronDown, ChevronRight, LoaderCircle } from 'lucide-react';
import { useState } from 'react';
import { PaneTab } from '#src/tools/TerminalTab.tsx';

/** `label` says what runs ("Installing git…"); `terminal` is its window's handle, null until the daemon answers. */
export function Working({ label, terminal }: { label: string; terminal: string | null }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="flex w-full flex-col gap-2">
      <div role="status" className="flex items-center gap-2 text-xs text-muted">
        <LoaderCircle size={14} className="shrink-0 animate-spin" />
        <span>{label}</span>
        <button
          type="button"
          aria-expanded={open}
          disabled={!terminal}
          onClick={() => setOpen(!open)}
          className="inline-flex items-center gap-0.5 hover:text-fg disabled:opacity-50"
        >
          {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          Details
        </button>
      </div>
      {open && terminal && (
        <div className="flex h-60 min-h-0 overflow-hidden rounded-md">
          <PaneTab handle={terminal} label={label} />
        </div>
      )}
    </div>
  );
}

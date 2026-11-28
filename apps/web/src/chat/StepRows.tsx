import type { TranscriptItem } from '@handline/client';
import { Check, ChevronRight } from 'lucide-react';
import { useState } from 'react';
import { Step } from '#src/chat/TurnRecord.tsx';
import { Markdown } from '#src/ui/Markdown.tsx';

/** A Chat agent's turn, which has no terminal to look at: each step on its own line as it lands, and a click opens what it printed. */
export function StepRows({ items }: { items: TranscriptItem[] }) {
  return (
    <div className="mb-2 flex w-full max-w-150 min-w-0 flex-col gap-0.5 text-sm">
      {items.map((i) => (i.kind === 'reply' ? <Said key={i.id} text={i.text} /> : <StepRow key={i.id} i={i} />))}
    </div>
  );
}

function Said({ text }: { text?: string }) {
  return text ? <div className="min-w-0 py-1 text-fg">{<Markdown text={text} />}</div> : null;
}

function StepRow({ i }: { i: TranscriptItem }) {
  const [open, setOpen] = useState(false);
  const out = i.output ?? '';
  if (i.kind !== 'tool' && i.kind !== 'bash') return <Step i={i} />;
  return (
    <div className="flex min-w-0 flex-col">
      <button
        type="button"
        aria-expanded={open}
        disabled={!out}
        onClick={() => setOpen(!open)}
        className="flex min-w-0 items-baseline gap-2 py-0.5 text-left text-muted enabled:hover:text-fg"
      >
        <Check size={12} aria-label="done" className="shrink-0 self-center text-success" />
        {i.tool && <span className="shrink-0 font-medium text-fg">{i.tool}</span>}
        {i.text && <code className="min-w-0 truncate font-mono text-xs text-fg">{i.text}</code>}
        {out && <ChevronRight size={14} className={`shrink-0 self-center transition-transform ${open ? 'rotate-90' : ''}`} />}
      </button>
      {open && (
        <div className="mt-1 mb-1.5 ml-1.5 border-l border-border pl-3">
          <pre className="overflow-x-auto rounded bg-surface px-2.5 py-2 font-mono text-xs whitespace-pre text-fg">{out}</pre>
          {!!i.dropped_lines && <p className="mt-1 text-xs text-muted">{i.dropped_lines} more lines not kept</p>}
        </div>
      )}
    </div>
  );
}

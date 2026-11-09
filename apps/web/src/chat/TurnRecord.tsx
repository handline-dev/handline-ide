import type { TurnRecord as Turn } from '@handline/app-core';
import type { TranscriptItem } from '@handline/client';
import { motion } from 'framer-motion';
import { ChevronRight } from 'lucide-react';
import { useState } from 'react';
import { clock } from '#src/format/time.ts';
import { spring } from '#src/motion.ts';
import { Markdown } from '#src/ui/Markdown.tsx';

/** `12 steps · 4m`: a turn's tool calls and the agent's own text, folded; a click opens them in order. */
export function TurnRecord({ r, fade }: { r: Turn; fade: boolean }) {
  const [open, setOpen] = useState(false); // not kept: one line of the stream, which draws lines as they scroll in
  const notes = r.items.filter((i) => i.kind === 'reply').length;
  const what = r.steps ? `${r.steps} ${r.steps === 1 ? 'step' : 'steps'}` : `${notes} ${notes === 1 ? 'note' : 'notes'}`;
  const min = Math.floor((Date.parse(r.until) - Date.parse(r.at)) / 60_000);
  return (
    <motion.div
      data-id={`record:${r.id}`}
      initial={fade ? { opacity: 0 } : false}
      animate={{ opacity: 1 }}
      transition={spring}
      className="group flex flex-col items-start"
    >
      <div className="w-full max-w-150 overflow-hidden rounded-lg border border-border text-sm">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className="flex w-full items-center gap-1.5 px-3 py-1.5 text-left text-xs text-muted hover:bg-hover/30"
        >
          <ChevronRight size={12} className={`shrink-0 transition-transform ${open ? 'rotate-90' : ''}`} />
          {what} · {min < 1 ? '<1m' : `${min}m`}
        </button>
        {open && (
          <div className="flex flex-col gap-1.5 border-t border-border px-3 py-2">
            {r.items.map((i) => (
              <Step key={i.id} i={i} />
            ))}
          </div>
        )}
      </div>
      <time
        dateTime={r.at}
        className="mb-2 flex h-6 items-center text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      >
        {clock(r.at) === clock(r.until) ? clock(r.at) : `${clock(r.at)} ~ ${clock(r.until)}`}
      </time>
    </motion.div>
  );
}

/** A tool call as `Tool text` in mono, a permission ask in words, the agent's text as it wrote it. */
function Step({ i }: { i: TranscriptItem }) {
  if (i.kind === 'reply') return <div className="min-w-0 text-sm text-muted">{i.text && <Markdown text={i.text} />}</div>;
  const text =
    i.kind === 'prompt'
      ? `Asked permission: ${[i.prompt?.tool, i.prompt?.command].filter(Boolean).join(' ')}${i.open ? ' (waiting)' : ''}`
      : [i.tool, i.text].filter(Boolean).join(' ');
  return <code className="min-w-0 truncate font-mono text-xs text-fg">{text}</code>;
}

import type { ChatMessage } from '@handline/client';
import { motion } from 'framer-motion';
import { ArrowRight } from 'lucide-react';
import { useState } from 'react';
import { clock } from '#src/format/time.ts';
import { spring } from '#src/motion.ts';
import { Markdown } from '#src/ui/Markdown.tsx';

/** A message this agent sent another agent: one quiet line, set apart from the conversation, with to whom and the start of it.
 * A click opens it in place, whole (its height from none to its own), and closes it again. */
export function AgentNote({ m }: { m: ChatMessage }) {
  const [open, setOpen] = useState(false); // not kept: one line of the stream, which draws lines as they scroll in

  const first = m.body.replace(/\s+/g, ' ').trim();
  return (
    // Like a message line, its time shows on hover in the row under it, which keeps its height so nothing moves.
    <div className="group flex max-w-150 min-w-0 flex-col items-start">
      <div className="-mx-2 flex max-w-full min-w-0 flex-col text-xs text-muted">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
          className="flex min-w-0 items-center gap-2 rounded-md px-2 py-1 text-left outline-none hover:bg-hover/30 hover:text-fg focus-visible:ring-2 focus-visible:ring-accent"
        >
          <ArrowRight size={12} className="shrink-0" />
          <span className="shrink-0 font-medium">{m.to}</span>
          {!open && <span className="min-w-0 truncate">{first}</span>}
        </button>
        <motion.div
          initial={false}
          animate={open ? { height: 'auto', opacity: 1 } : { height: 0, opacity: 0 }}
          transition={spring}
          className="overflow-hidden"
        >
          <div className="px-2 pt-1 pb-2 text-sm text-fg">
            <Markdown text={m.body} />
          </div>
        </motion.div>
      </div>
      <time
        dateTime={m.at}
        className="h-5 text-xs leading-5 text-muted opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
      >
        {clock(m.at)}
      </time>
    </div>
  );
}

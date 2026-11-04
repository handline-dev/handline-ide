import { sideOf, tickLevel, type TimelineEntry } from '@handline/app-core';
import { motion } from 'framer-motion';
import { CompactionLine } from '#src/chat/CompactionLine.tsx';
import { Actions } from '#src/chat/MessageActions.tsx';
import { Ticks } from '#src/chat/Ticks.tsx';
import { clock } from '#src/format/time.ts';
import { spring } from '#src/motion.ts';

/** One line of the chat: a compaction, or a message or reply with copy and quote on hover. A line with no text draws nothing.
 * It fades in when it arrives; lines already there when the chat opens just show (AnimatePresence initial off, in the stream). */
export function ChatLine({ e, agent, me, onQuote }: { e: TimelineEntry; agent: string; me: string; onQuote: (text: string) => void }) {
  const key = `${e.kind}:${e.id}`;
  const fade = { initial: { opacity: 0 }, animate: { opacity: 1 }, transition: spring };
  if (e.kind === 'item' && e.item.session)
    return (
      <motion.div data-id={key} {...fade}>
        <CompactionLine at={e.at} cause={e.item.session.cause} handoff={e.item.session.handoff} />
      </motion.div>
    );
  const text = e.kind === 'item' ? (e.item.text ?? '') : e.kind === 'proposal' ? '' : e.message.body;
  if (!text) return null;
  const mine = e.kind === 'message' && sideOf(e.message, agent, me) === 'person';
  return (
    // The hover row keeps its height when hidden: that row is the space between lines. Mine sit on the right, on the selection colour.
    <motion.div data-id={key} {...fade} className={`group flex flex-col ${mine ? 'items-end' : 'items-start'}`}>
      <p
        className={`max-w-150 min-w-0 text-base wrap-break-word whitespace-pre-wrap text-fg ${mine ? 'rounded-xl bg-selected px-4 py-2' : ''}`}
      >
        {text}
      </p>
      {/* On hover: the agent's [buttons] time; mine: read or not, time, [buttons]. */}
      <div className="flex items-center gap-1.5 text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
        {mine && e.kind === 'message' ? (
          <>
            <Ticks level={tickLevel(e.message)} />
            <time dateTime={e.at}>{clock(e.at)}</time>
            <Actions text={text} onQuote={onQuote} />
          </>
        ) : (
          <>
            <Actions text={text} onQuote={onQuote} />
            <time dateTime={e.at}>{clock(e.at)}</time>
          </>
        )}
      </div>
    </motion.div>
  );
}

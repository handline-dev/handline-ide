import { isPerson, sideOf, tickLevel, type TimelineEntry } from '@handline/app-core';
import { motion } from 'framer-motion';
import { AgentNote } from '#src/chat/AgentNote.tsx';
import { CompactionLine } from '#src/chat/CompactionLine.tsx';
import { Actions } from '#src/chat/MessageActions.tsx';
import { MessageImages } from '#src/chat/MessageImages.tsx';
import { Ticks } from '#src/chat/Ticks.tsx';
import { clock } from '#src/format/time.ts';
import { spring } from '#src/motion.ts';

/** One line of the chat: a compaction, or a message or reply (its pictures above its text) with copy and quote on hover. A line with neither draws nothing.
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
  // A message that does not involve the founder is the agent's talk with another. What it said to others is a quiet line, read
  // whole on a click; what others said to it is not shown.
  if ((e.kind === 'message' || e.kind === 'note') && !isPerson(e.message.from, me) && !isPerson(e.message.to, me)) {
    if (e.message.from !== agent) return null;
    return (
      <motion.div data-id={key} {...fade} className="flex">
        <AgentNote m={e.message} />
      </motion.div>
    );
  }
  // A message with pictures reads from its parts: the text with each picture's reference left out, the pictures drawn above it.
  const parts = e.kind === 'message' ? e.message.parts : undefined;
  const text =
    e.kind === 'item'
      ? (e.item.text ?? '')
      : e.kind === 'proposal'
        ? ''
        : parts
          ? parts
              .flatMap((p) => ('text' in p ? [p.text] : []))
              .join('')
              .trim()
          : e.message.body;
  const images = e.kind === 'message' ? (e.message.attachments ?? []).filter((a) => a.mime.startsWith('image/')) : [];
  if (!text && !images.length) return null;
  const mine = e.kind === 'message' && sideOf(e.message, agent, me) === 'person';
  return (
    // The hover row keeps its height when hidden: that row is the space between lines. Mine sit on the right, on the selection colour.
    <motion.div data-id={key} {...fade} className={`group flex flex-col gap-1 ${mine ? 'items-end' : 'items-start'}`}>
      <MessageImages images={images} />
      {text && (
        <p
          className={`max-w-150 min-w-0 text-base wrap-break-word whitespace-pre-wrap text-fg ${mine ? 'rounded-xl bg-selected px-4 py-2' : ''}`}
        >
          {text}
        </p>
      )}
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

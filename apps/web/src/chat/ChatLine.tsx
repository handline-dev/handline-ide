import { isPerson, isSystem, sideOf, tickLevel, type TimelineEntry } from '@handline/app-core';
import { motion } from 'framer-motion';
import { CornerDownRight } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { AgentNote } from '#src/chat/AgentNote.tsx';
import { CompactionLine } from '#src/chat/CompactionLine.tsx';
import { Actions } from '#src/chat/MessageActions.tsx';
import { MessageImages } from '#src/chat/MessageImages.tsx';
import { Ticks } from '#src/chat/Ticks.tsx';
import { Markdown } from '#src/ui/Markdown.tsx';
import { clock } from '#src/format/time.ts';
import { spring } from '#src/motion.ts';

/** Whether a line draws nothing (what others said to the agent, an empty item): the rules ChatLine follows, so a run of task
 * cards is not cut by a line nobody sees. */
export function drawsNothing(e: TimelineEntry, agent: string, me: string): boolean {
  if (e.kind === 'item') return !e.item.session && !e.item.text;
  if (e.kind === 'proposal') return true;
  // The daemon's own bookkeeping (task-board digests and nudges) is not shown anywhere.
  if (isSystem(e.message.from)) return true;
  if (!isPerson(e.message.from, me) && !isPerson(e.message.to, me)) return e.message.from !== agent;
  const text = e.message.parts ? e.message.parts.some((p) => 'text' in p && p.text.trim()) : !!e.message.body;
  return !text && !(e.message.attachments ?? []).some((a) => a.mime.startsWith('image/'));
}

/** One line of the chat: a compaction, or a message or reply (its pictures above its text) with copy and quote on hover. A line with neither draws nothing.
 * It fades in when it arrives; lines already there when the chat opens just show (AnimatePresence initial off, in the stream). */
/** What a message is sent in reply to: a message (its parent) or one of the agent's replies (parent_item), with the words picked from it. */
export type Reply = { kind: 'message' | 'item'; id: number; text: string; quote?: string };

/** The longest quote chat.send takes (the daemon's quoteMax, in characters). */
const QUOTE_MAX = 1000;

export function ChatLine({
  e,
  agent,
  me,
  onReply,
  parentText,
  fade: fades = true,
}: {
  e: TimelineEntry;
  agent: string;
  me: string;
  onReply: (r: Reply) => void;
  /** The words of the message this one replies to, when it is loaded. */
  parentText?: string;
  fade?: boolean;
}) {
  const key = `${e.kind}:${e.id}`;
  if (drawsNothing(e, agent, me)) return null;
  const fade = { initial: fades ? { opacity: 0 } : (false as const), animate: { opacity: 1 }, transition: spring };
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
  const reply = (quote?: string) =>
    e.kind === 'item' || e.kind === 'message'
      ? onReply({ kind: e.kind, id: e.kind === 'item' ? e.item.id : e.message.id, text, quote })
      : undefined;
  const m = e.kind === 'message' ? e.message : null;
  return (
    // The hover row keeps its height when hidden: that row is the space between lines. Mine sit on the right, on the selection colour.
    <motion.div data-id={key} {...fade} className={`group flex flex-col ${mine ? 'items-end' : 'items-start'}`}>
      <MessageImages images={images} />
      {m?.parent != null && <Reference text={parentText} onClick={() => jumpTo(m.parent!)} />}
      {m?.parent_item && <Reference text={m.parent_item.text} />}
      {text && (
        <Quotable text={text} on={!mine} onQuote={reply}>
          <div
            className={`max-w-150 min-w-0 text-base text-fg ${images.length ? 'mt-2' : ''} ${mine ? 'rounded-xl bg-selected px-4 py-2' : ''}`}
          >
            <Markdown text={text} />
          </div>
        </Quotable>
      )}
      {/* On hover: the agent's [buttons] time; mine: read or not, time, [buttons]. */}
      <div className="mb-2 flex h-6 items-center gap-1.5 text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
        {mine && e.kind === 'message' ? (
          <>
            <Ticks level={tickLevel(e.message)} />
            <time dateTime={e.at}>{clock(e.at)}</time>
            <Actions text={text} onQuote={() => reply()} />
          </>
        ) : (
          <>
            <Actions text={text} onQuote={() => reply()} />
            <time dateTime={e.at}>{clock(e.at)}</time>
          </>
        )}
      </div>
    </motion.div>
  );
}

/** Above a reply, what it answers: one line of the message's words; a click goes to that message when it is loaded. */
function Reference({ text, onClick }: { text?: string; onClick?: () => void }) {
  return (
    <button
      type="button"
      disabled={!onClick}
      onClick={onClick}
      className="mb-1 flex max-w-80 min-w-0 items-center gap-1 rounded-md px-1 text-left text-xs text-muted enabled:hover:bg-hover/30"
    >
      <CornerDownRight size={12} className="shrink-0" />
      <span className="min-w-0 truncate">{text ? text.replace(/\s+/g, ' ') : 'An earlier message'}</span>
    </button>
  );
}

/** Scrolls the message with this id into the middle of the chat, when it is drawn. */
function jumpTo(id: number) {
  document.querySelector(`[data-id="message:${id}"]`)?.scrollIntoView({ block: 'center', behavior: 'smooth' });
}

/** Words selected in it offer a small Quote above them; only words that are in the text, and no more than the daemon takes. */
function Quotable({ text, on, onQuote, children }: { text: string; on: boolean; onQuote: (quote: string) => void; children: ReactNode }) {
  const [sel, setSel] = useState<{ text: string; x: number; y: number } | null>(null);
  const select = () => {
    const s = window.getSelection();
    const picked = s?.toString().replace(/\s+/g, ' ').trim();
    if (!on || !picked || !s?.rangeCount) return setSel(null);
    if ([...picked].length > QUOTE_MAX || !text.replace(/\s+/g, ' ').includes(picked)) return setSel(null);
    const r = s.getRangeAt(0).getBoundingClientRect();
    setSel({ text: picked, x: r.left + r.width / 2, y: r.top });
  };
  return (
    <div className="contents" onMouseDown={() => setSel(null)} onMouseUp={select}>
      {children}
      {sel && (
        <button
          type="button"
          className="fixed z-30 -translate-x-1/2 -translate-y-full rounded-md bg-fg px-2 py-1 text-xs text-bg"
          style={{ left: sel.x, top: sel.y - 6 }}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => {
            onQuote(sel.text);
            setSel(null);
            window.getSelection()?.removeAllRanges();
          }}
        >
          Quote
        </button>
      )}
    </div>
  );
}

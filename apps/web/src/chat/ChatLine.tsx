import { BASH_WATCH_MS, isPerson, isSystem, maybeRanNote, sideOf, tickLevel, type TimelineEntry } from '@handline/app-core';
import type { TranscriptItem } from '@handline/client';
import { motion } from 'framer-motion';
import { CornerDownRight } from 'lucide-react';
import { BashChip } from '#src/chat/Composer.tsx';
import { useState, type ReactNode } from 'react';
import { AgentNote } from '#src/chat/AgentNote.tsx';
import { CompactionLine } from '#src/chat/CompactionLine.tsx';
import { MoveLine } from '#src/chat/agent/AccountSwitch.tsx';
import { Dots } from '#src/chat/Dots.tsx';
import { Actions, CopyButton } from '#src/chat/MessageActions.tsx';
import { MessageImages } from '#src/chat/MessageImages.tsx';
import { Ticks } from '#src/chat/Ticks.tsx';
import { Markdown } from '#src/ui/Markdown.tsx';
import { clock, useNow } from '#src/format/time.ts';
import { spring } from '#src/motion.ts';
import { CommandsOf } from '#src/chat/CommandBlock.tsx';

/** Whether a line draws nothing (what others said to the agent, an empty item): the rules ChatLine follows, so a run of task
 * cards is not cut by a line nobody sees. */
export function drawsNothing(e: TimelineEntry, agent: string, me: string): boolean {
  if (e.kind === 'item') return !e.item.session && !e.item.text && e.item.kind !== 'bash' && !e.item.account_moved && !e.item.move_failed;
  if (e.kind === 'proposal') return true;
  // The daemon's own bookkeeping (sys digests and nudges) is not shown anywhere.
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
  bash,
  fade: fades = true,
}: {
  e: TimelineEntry;
  agent: string;
  me: string;
  onReply: (r: Reply) => void;
  /** The words of the message this one replies to, when it is loaded. */
  parentText?: string;
  /** What this ! line ran and printed, once the daemon has paired them. */
  bash?: TranscriptItem;
  fade?: boolean;
}) {
  const key = `${e.kind}:${e.id}`;
  if (drawsNothing(e, agent, me)) return null;
  const fade = { initial: fades ? { opacity: 0 } : (false as const), animate: { opacity: 1 }, transition: spring };
  if (e.kind === 'item' && e.item.session)
    return (
      <motion.div data-id={key} {...fade}>
        <CompactionLine at={e.at} {...e.item.session} />
      </motion.div>
    );
  if (e.kind === 'item' && (e.item.account_moved || e.item.move_failed))
    return (
      <motion.div data-id={key} {...fade}>
        <MoveLine at={e.at} moved={e.item.account_moved} failed={e.item.move_failed} />
      </motion.div>
    );
  // A command with no message to fold into: typed at the terminal, or sent before messages recorded their landing.
  if (e.kind === 'item' && e.item.kind === 'bash') {
    const command = e.item.command ?? '';
    return (
      <motion.div data-id={key} {...fade} className="group flex flex-col items-end">
        <CommandBox command={command} bash={e.item} />
        <div className="mb-2 flex h-6 items-center gap-1.5 text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
          <time dateTime={e.at}>{clock(e.at)}</time>
          <CopyButton text={command} />
        </div>
      </motion.div>
    );
  }
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
  // Its command fences run in the agent's terminal: an agent's reply item, or a message in this chat.
  const commands =
    e.kind === 'item' && e.item.kind === 'reply'
      ? { agent, owner: `r${e.item.id}`, runs: e.item.runs }
      : m
        ? { agent, owner: String(m.id), runs: m.runs }
        : null;
  return (
    // The hover row keeps its height when hidden: that row is the space between lines. Mine sit on the right, on the selection colour.
    <motion.div data-id={key} {...fade} className={`group flex flex-col ${mine ? 'items-end' : 'items-start'}`}>
      <MessageImages images={images} />
      {m?.parent != null && <Reference text={parentText} onClick={() => jumpTo(m.parent!)} />}
      {m?.parent_item && <Reference text={m.parent_item.text} />}
      {text && (
        <Quotable text={text} on={!mine} onQuote={reply}>
          {bash || (mine && text.startsWith('!')) ? (
            // A command the person ran in the agent's terminal (bash mode): the command as typed, not a message bubble.
            <CommandBox command={bash?.command ?? text.slice(1).trim()} bash={bash} className={images.length ? 'mt-2' : ''} />
          ) : (
            <div
              className={`max-w-150 min-w-0 text-base text-fg ${images.length ? 'mt-2' : ''} ${mine ? 'rounded-xl bg-selected px-4 py-2' : ''}`}
            >
              <CommandsOf.Provider value={commands}>
                <Markdown text={text} />
              </CommandsOf.Provider>
            </div>
          )}
        </Quotable>
      )}
      {mine && m?.receipt?.state === 'unknown' && (
        <p role="note" className="mt-1 max-w-150 text-xs text-danger">
          {maybeRanNote(text)}
        </p>
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

/** A command run in the agent's terminal and, under it, what it printed. No success or failure mark: the transcript records no exit code, and stderr does not tell. */
function CommandBox({ command, bash, className = '' }: { command: string; bash?: TranscriptItem; className?: string }) {
  const output = [bash?.stdout, bash?.stderr].filter(Boolean).join('\n');
  const dropped = bash?.dropped_lines ?? 0;
  return (
    <div className={`flex w-fit max-w-150 min-w-0 flex-col rounded-xl border border-border ${className}`}>
      <div className="flex min-w-0 items-start gap-2 px-3 py-2">
        <BashChip />
        <code className="min-w-0 py-0.5 font-mono text-sm break-all whitespace-pre-wrap text-fg">{command}</code>
      </div>
      {bash && !bash.done && <Running at={bash.at} />}
      {output && (
        <pre className="max-h-80 overflow-auto border-t border-border px-3 py-2 font-mono text-xs break-all whitespace-pre-wrap text-muted">
          {output}
        </pre>
      )}
      {dropped > 0 && (
        <div className="border-t border-border px-3 py-1 text-xs text-muted">
          {dropped} more {dropped === 1 ? 'line' : 'lines'} not kept
        </div>
      )}
    </div>
  );
}

/** Dots while the daemon still watches for the output; after that nothing, as a killed command may never record any. */
function Running({ at }: { at: string }) {
  const now = useNow();
  if (now - Date.parse(at) > BASH_WATCH_MS) return null;
  return (
    <div className="border-t border-border px-3 py-1">
      <Dots />
    </div>
  );
}

/** Above a reply, what it answers: one line of the message's words; a click goes to that message when it is loaded. */
export function Reference({ text, onClick }: { text?: string; onClick?: () => void }) {
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

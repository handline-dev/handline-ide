import { AnimatePresence, motion } from 'framer-motion';
import { chatView, sideOf, tickLevel } from '@handline/app-core';
import { useAgents, useChat, useChatActions } from '@handline/app-data';
import { useMe, useNearEnd } from '@handline/app-react';
import { Check, Copy, LoaderCircle, Quote } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Ticks } from '#src/Ticks.tsx';
import { useChatScroll } from '#src/useChatScroll.ts';
import { spring } from '#src/motion.ts';
import { Page } from '#src/ui/Page.tsx';
import { toast } from '#src/ui/toast.tsx';

/** An agent's chat, first cut: its messages and replies, oldest first, each with copy and reference on hover, and a box to send one. */
export function Chat({ agent }: { agent: string }) {
  const chat = useChat(agent, 'mine');
  const me = useMe();
  const { entries } = chatView(chat, 'mine', agent, me);
  const doing = useAgents().data?.agents.find((a) => a.name === agent)?.doing?.text;
  const stream = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  // Enters at the bottom, follows new lines while there, and keeps a reader above it on their line when
  // older pages or images land (the legacy chat's scroll, shared logic in app-core).
  const scroll = useChatScroll({ stream, content, ready: chat.status === 'ready', agent, reading: chat.holdPlace });
  // Lines added at the end (not an older page landing at the top): counted from the last line known.
  const last = useRef<string | null>(null);
  useEffect(() => {
    const keys = entries.map((e) => `${e.kind}:${e.id}`);
    const at = last.current === null ? -1 : keys.lastIndexOf(last.current);
    if (last.current !== null && at >= 0 && at < keys.length - 1) scroll.added(keys.length - 1 - at);
    last.current = keys.at(-1) ?? null;
  });
  const top = useNearEnd<HTMLDivElement, HTMLDivElement>(
    stream,
    chat.loadOlder,
    chat.hasOlder && !chat.loadingOlder && chat.status === 'ready',
    {
      atTop: true,
      margin: 400,
    },
  );
  const { send } = useChatActions(agent);
  const [draft, setDraft] = useState('');
  const submit = () => {
    const body = draft.trim();
    if (!body) return;
    send(body);
    scroll.sent();
    setDraft('');
  };
  const quote = (body: string) => setDraft((d) => `${body.replace(/^/gm, '> ')}\n\n${d}`);

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div
        ref={stream}
        onScroll={scroll.onScroll}
        onWheel={scroll.driven}
        onTouchStart={scroll.driven}
        onKeyDown={scroll.driven}
        onPointerDown={scroll.driven}
        className="min-h-0 flex-1 overflow-y-auto py-4"
      >
        <div ref={content}>
          <Page className="flex flex-col">
            {/* Near the top (400px out), the older page is read; useChatScroll keeps the line in view where it was. */}
            {chat.hasOlder && <div ref={top} aria-hidden className="h-px" />}
            {chat.loadingOlder && (
              <div className="flex items-center justify-center gap-2 py-3 text-xs text-muted">
                <LoaderCircle size={14} strokeWidth={3} className="animate-spin" />
                Loading earlier messages
              </div>
            )}
            {chat.status === 'loading' && <p className="text-sm text-muted">Loading…</p>}
            {chat.status === 'error' && <p className="text-sm text-danger">Chat did not load.</p>}
            {/* A new line fades in; the ones already there when the chat opens just show (initial off). */}
            <AnimatePresence initial={false}>
              {entries.map((e) => {
                const text = e.kind === 'item' ? (e.item.text ?? '') : e.kind === 'proposal' ? '' : e.message.body;
                if (!text) return null;
                const mine = e.kind === 'message' && sideOf(e.message, agent, me) === 'person';
                return (
                  // The hover row keeps its height when hidden: that row is the space between lines.
                  // Mine sit on the right, on the selection colour.
                  <motion.div
                    key={`${e.kind}:${e.id}`}
                    data-id={`${e.kind}:${e.id}`}
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    transition={spring}
                    className={`group flex flex-col ${mine ? 'items-end' : 'items-start'}`}
                  >
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
                          <Actions text={text} onQuote={quote} />
                        </>
                      ) : (
                        <>
                          <Actions text={text} onQuote={quote} />
                          <time dateTime={e.at}>{clock(e.at)}</time>
                        </>
                      )}
                    </div>
                  </motion.div>
                );
              })}
            </AnimatePresence>
            {/* The dots' row is always there, so turning them on or off does not move the chat. */}
            <div
              className={`h-5 transition-opacity ${chat.working.turn_running ? 'opacity-100' : 'opacity-0'}`}
              aria-hidden={!chat.working.turn_running}
            >
              <span className="flex items-center gap-2">
                <Dots />
                {/* What the agent is doing right now: its newest tool call (AgentView.doing). */}
                {doing && <span className="truncate text-xs text-muted">{doing}</span>}
              </span>
            </div>
          </Page>
        </div>
      </div>
      <Page>
        <form
          className="flex gap-2 py-3"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                submit();
              }
            }}
            rows={2}
            placeholder={`Message ${agent}`}
            className="flex-1 resize-none rounded-xl border border-border bg-bg px-4 py-2 text-base text-fg outline-none focus:border-accent"
          />
          <Button type="submit">Send</Button>
        </form>
      </Page>
    </div>
  );
}

const clock = (iso: string) => new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

function Actions({ text, onQuote }: { text: string; onQuote: (text: string) => void }) {
  return (
    <span className="flex gap-0.5">
      <CopyButton text={text} />
      <IconButton label="Reference" onClick={() => onQuote(text)}>
        <Quote size={14} />
      </IconButton>
    </span>
  );
}

/** Copies `text`; a check stands in for the icon for a moment to show it took. */
function CopyButton({ text }: { text: string }) {
  const [done, setDone] = useState(false);
  return (
    <IconButton
      label={done ? 'Copied' : 'Copy'}
      onClick={() => {
        navigator.clipboard.writeText(text).then(
          () => {
            setDone(true);
            setTimeout(() => setDone(false), 1500);
          },
          () => toast('Copy did not work.', { tone: 'warning' }),
        );
      }}
    >
      {done ? <Check size={14} className="text-success" /> : <Copy size={14} />}
    </IconButton>
  );
}

/** The agent is working: three dots rising in turn. */
function Dots() {
  return (
    <span aria-label="working" className="flex h-5 items-center gap-1">
      {[0, 1, 2].map((i) => (
        <motion.span
          key={i}
          className="size-1.5 rounded-full bg-muted"
          animate={{ y: [0, -3, 0], opacity: [0.4, 1, 0.4] }}
          transition={{ duration: 0.9, repeat: Infinity, delay: i * 0.15 }}
        />
      ))}
    </span>
  );
}

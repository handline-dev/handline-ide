import { chatView } from '@handline/app-core';
import { useAgents, useChatActions, useChatServed, useChatSuspense } from '@handline/app-data';
import { useMe, useNearEnd } from '@handline/app-react';
import { AnimatePresence } from 'framer-motion';
import { LoaderCircle } from 'lucide-react';
import { useEffect, useImperativeHandle, useRef, type ReactNode, type Ref } from 'react';
import { useAttachments } from '#src/chat/attachments.tsx';
import { ChatLine } from '#src/chat/ChatLine.tsx';
import { ChatSkeleton } from '#src/chat/ChatSkeleton.tsx';
import { Composer } from '#src/chat/Composer.tsx';
import { Dots } from '#src/chat/Dots.tsx';
import { useDraft } from '#src/chat/drafts.ts';
import { useChatScroll } from '#src/chat/useChatScroll.ts';
import { Boundary } from '#src/ui/Boundary.tsx';
import { Button } from '#src/ui/Button.tsx';
import { DropHint, useFileDrop } from '#src/ui/fileDrop.tsx';
import { Page } from '#src/ui/Page.tsx';
import { useScrollFade } from '#src/ui/ScrollFade.tsx';

/** An agent's chat: its lines, and the box to write in. The box is there from the first moment; the lines wait, as a skeleton,
 * until the messages and the transcript are both read (a suspense boundary), so the chat appears whole. */
export function Chat({ agent }: { agent: string }) {
  const served = useChatServed();
  const { send } = useChatActions(agent);
  const [draft, setDraft] = useDraft(agent);
  const attach = useAttachments(agent);
  const drop = useFileDrop(attach.add);
  const quote = (body: string) => setDraft((d) => `${body.replace(/^/gm, '> ')}\n\n${d}`);
  // The stream's scroll hears of a send through this, once the stream is drawn.
  const stream = useRef<{ sent: () => void }>(null);
  const loading = (
    <Shell>
      <ChatSkeleton />
    </Shell>
  );

  return (
    // Files dropped anywhere on the chat go with the next message.
    <div {...drop.handlers} className="relative flex min-h-0 min-w-0 flex-1 flex-col">
      <DropHint shown={drop.over} />
      {served ? (
        <Boundary
          loading={loading}
          failed={(retry) => (
            <Shell>
              <div className="flex items-center gap-3 text-sm text-danger">
                Chat did not load.
                <Button onClick={retry}>Retry</Button>
              </div>
            </Shell>
          )}
        >
          <Stream ref={stream} agent={agent} onQuote={quote} />
        </Boundary>
      ) : (
        loading
      )}
      <Page>
        <Composer
          agent={agent}
          draft={draft}
          setDraft={setDraft}
          attach={attach}
          onSend={(body, attachments) => {
            send(body, { attachments });
            stream.current?.sent();
          }}
        />
      </Page>
    </div>
  );
}

/** The stream's frame without its lines: what stands while they load. */
function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col justify-end overflow-hidden py-4">
      <Page>{children}</Page>
    </div>
  );
}

/** The chat's lines, oldest first. Suspends until the first pages are in; an older page is read near the top without suspending. */
function Stream({ agent, onQuote, ref }: { agent: string; onQuote: (text: string) => void; ref: Ref<{ sent: () => void }> }) {
  const chat = useChatSuspense(agent, 'mine');
  const me = useMe();
  const { entries } = chatView(chat, 'mine', agent, me);
  const doing = useAgents().data?.agents.find((a) => a.name === agent)?.doing?.text;
  const stream = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const fade = useScrollFade(stream);
  // Enters at the bottom, follows new lines while there, and keeps a reader above it on their line when
  // older pages or images land (the legacy chat's scroll, shared logic in app-core).
  const scroll = useChatScroll({ stream, content, ready: true, agent, reading: chat.holdPlace });
  useImperativeHandle(ref, () => ({ sent: scroll.sent }), [scroll.sent]);
  // Lines added at the end (not an older page landing at the top): counted from the last line known.
  const last = useRef<string | null>(null);
  useEffect(() => {
    const keys = entries.map((e) => `${e.kind}:${e.id}`);
    const at = last.current === null ? -1 : keys.lastIndexOf(last.current);
    if (last.current !== null && at >= 0 && at < keys.length - 1) scroll.added(keys.length - 1 - at);
    last.current = keys.at(-1) ?? null;
  });
  const top = useNearEnd<HTMLDivElement, HTMLDivElement>(stream, chat.loadOlder, chat.hasOlder && !chat.loadingOlder, {
    atTop: true,
    margin: 400,
  });

  return (
    <div
      ref={stream}
      onScroll={scroll.onScroll}
      onWheel={scroll.driven}
      onTouchStart={scroll.driven}
      onKeyDown={scroll.driven}
      onPointerDown={scroll.driven}
      className="min-h-0 flex-1 overflow-y-auto"
      style={fade}
    >
      {/* At least as tall as the view, its lines at the bottom: a short chat sits on the box, as a messenger's does. */}
      <div ref={content} className="flex min-h-full flex-col justify-end py-4">
        <Page className="flex flex-col">
          {/* Near the top (400px out), the older page is read; useChatScroll keeps the line in view where it was. */}
          {chat.hasOlder && <div ref={top} aria-hidden className="h-px" />}
          {chat.loadingOlder && (
            <div className="flex items-center justify-center gap-2 py-3 text-xs text-muted">
              <LoaderCircle size={14} strokeWidth={3} className="animate-spin" />
              Loading earlier messages
            </div>
          )}
          <AnimatePresence initial={false}>
            {entries.map((e) => (
              <ChatLine key={`${e.kind}:${e.id}`} e={e} agent={agent} me={me} onQuote={onQuote} />
            ))}
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
  );
}

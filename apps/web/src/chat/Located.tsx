// The app's one way to a chat message from outside its chat: a search hit and a schedule's last run both open this tab. It draws
// chat.locate's window (the page of messages around the message and the agent's transcript of the same span) as the chat draws
// them, so the message shows however far back it is, with no paging to it. It is a still window on the past, not the live chat:
// Open chat goes there.
import { chatOfHit } from '@handline/app-core';
import { useNameOf, useRead, why } from '@handline/app-data';
import type { Methods } from '@handline/client';
import { MessageSquare } from 'lucide-react';
import { useEffect, useRef, type ReactNode } from 'react';
import { ChatSkeleton } from '#src/chat/ChatSkeleton.tsx';
import { ChatRows, useChatRows } from '#src/chat/Rows.tsx';
import { clock, day } from '#src/format/time.ts';
import { chatOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Page } from '#src/ui/Page.tsx';

type Located = Methods['chat.locate']['result'];
/** Messages in the window, about half of them older than the one found. */
const WINDOW = 40;

export function LocatedTab({ request }: { request: number }) {
  // The daemon names the chat of the message's agent end; what an agent said to another agent is drawn in the sender's chat, so that
  // window is read there.
  const first = useRead('chat.locate', { request, limit: WINDOW });
  const drawnIn = first.data && chatOfHit({ agent: first.data.agent, from: first.data.message.from });
  const elsewhere = !!drawnIn && drawnIn !== first.data?.agent;
  const second = useRead('chat.locate', { request, agent: drawnIn ?? '', limit: WINDOW }, { enabled: elsewhere });
  const found = elsewhere ? second : first;
  // A window already drawn stays when reading it again fails: only a first read that fails says so.
  if (found.isError && !found.data)
    return (
      <Frame>
        <p className="text-sm text-muted">{why(found.error) || 'This message is not in a chat any more.'}</p>
      </Frame>
    );
  if (!found.data)
    return (
      <Frame>
        <ChatSkeleton />
      </Frame>
    );
  return <Window key={found.data.agent} at={found.data} request={request} />;
}

function Frame({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto py-4">
      <Page>{children}</Page>
    </div>
  );
}

function Window({ at, request }: { at: Located; request: number }) {
  const nameOf = useNameOf();
  const open = useOpenable(chatOf(at.agent));
  const lines = useChatRows(
    { messages: at.messages, items: at.transcript, moreMessages: at.more_older, moreItems: at.transcript_more_older },
    at.agent,
  );
  const box = useRef<HTMLDivElement>(null);
  const agent = at.agent;
  // The message comes to the middle and is lit once, as a line that just changed is (styles.css just-changed): once for the message
  // in this chat, as the window is first drawn. The window read again (a recent message's window grows) leaves the reader where they
  // are, so the effect follows which message and whose chat, never the data it was drawn from.
  useEffect(() => {
    const el = box.current?.querySelector(`[data-id="message:${request}"]`);
    if (!el) return;
    el.scrollIntoView({ block: 'center' });
    el.classList.add('rounded-md', 'just-changed');
    const t = setTimeout(() => el.classList.remove('rounded-md', 'just-changed'), 2500);
    return () => clearTimeout(t);
  }, [request, agent]);
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="border-b border-border py-2">
        <Page className="flex items-center gap-2 text-sm">
          <span className="grid size-4 shrink-0 place-items-center text-muted">
            <MessageSquare size={14} />
          </span>
          {/* Each is cut on its own where the bar runs out, so neither runs under the button. */}
          <span className="min-w-0 truncate font-medium text-fg">{nameOf(at.agent)} chat</span>
          <span className="min-w-0 flex-1 truncate text-muted">
            · around {day(at.message.at)}, {clock(at.message.at)}
          </span>
          <Button {...open}>Open chat</Button>
        </Page>
      </div>
      <div ref={box} className="min-h-0 flex-1 overflow-y-auto py-4">
        <Page className="flex flex-col">
          {(at.more_older || at.transcript_more_older) && <Edge>Earlier messages are in the chat.</Edge>}
          <ChatRows lines={lines} agent={at.agent} />
          {(at.more_newer || at.transcript_more_newer) && <Edge>Later messages are in the chat.</Edge>}
        </Page>
      </div>
    </div>
  );
}

function Edge({ children }: { children: ReactNode }) {
  return <p className="py-3 text-center text-xs text-muted">{children}</p>;
}

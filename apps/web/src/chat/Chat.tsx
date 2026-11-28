import { Spinner } from '#src/ui/Loading.tsx';
import { knownUpTo, turnMinutes } from '@handline/app-core';
import { useAgents, useChat, useChatActions, useChatReadMarks, useChatServed, useChatSuspense, useVerbRun } from '@handline/app-data';
import { restartingLine, VERBS } from '@handline/app-core';
import { useNearEnd } from '@handline/app-react';
import { LAST_CHAT } from '#src/chat/toChat.ts';
import { write } from '#src/persist/kv.ts';
import { useEffect, useImperativeHandle, useRef, useState, type ReactNode, type Ref } from 'react';
import { ContextRing } from '#src/chat/agent/ContextRing.tsx';
import { COMPACTING_WORDS, useCompacting, useCompactNow } from '#src/chat/agent/compaction.ts';
import { MoveSteps, PendingMove, SwitchWait, useSwitchLock } from '#src/chat/agent/AccountSwitch.tsx';
import type { Command } from '#src/chat/Suggest.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { restarting } from '#src/layout/agentList.ts';
import { ModelChip } from '#src/chat/agent/ModelChip.tsx';
import { AgentSettingsButton } from '#src/chat/agent/AgentSettingsButton.tsx';
import { StatusLine } from '#src/chat/agent/StatusLine.tsx';
import { FootRow, HoldCard, useHoldLine } from '#src/chat/agent/HoldBar.tsx';
import { HeldCard } from '#src/chat/agent/HeldCard.tsx';
import { PauseCard } from '#src/chat/PauseCard.tsx';
import { PromptCard } from '#src/chat/PromptCard.tsx';
import { ProposalCards } from '#src/chat/ProposalCard.tsx';
import { takeToChat } from '#src/chat/toChat.ts';
import { useTaskActivity } from '#src/chat/TaskActivity.tsx';
import { useAttachments } from '#src/chat/attachments.tsx';
import type { Reply } from '#src/chat/ChatLine.tsx';
import { ChatRows, useChatRows } from '#src/chat/Rows.tsx';
import { CornerDownRight, FoldVertical, Square, SquareTerminal, X } from 'lucide-react';
import { ChatSkeleton } from '#src/chat/ChatSkeleton.tsx';
import { Composer } from '#src/chat/Composer.tsx';
import { AlongsideStrip, useAlongside } from '#src/chat/agent/AlongsideStrip.tsx';
import { MarkReply } from '#src/chat/MessageImages.tsx';
import { useDraft } from '#src/chat/drafts.ts';
import { useChatScroll } from '#src/chat/useChatScroll.ts';
import { JumpDown } from '#src/ui/JumpDown.tsx';
import { Boundary } from '#src/ui/Boundary.tsx';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { FileDropArea } from '#src/ui/fileDrop.tsx';
import { Page } from '#src/ui/Page.tsx';
import { toast } from '#src/ui/toast.tsx';
import { useScrollFade } from '#src/ui/ScrollFade.tsx';
import { useNow } from '#src/format/time.ts';
import { useTabState } from '#src/persist/tabState.ts';

/** An agent's chat: its lines, and the box to write in. The box is there from the first moment; the lines wait, as a skeleton,
 * until the messages and the transcript are both read (a suspense boundary), so the chat appears whole. */
export function Chat({ agent }: { agent: string }) {
  useEffect(() => write(LAST_CHAT, agent), [agent]);
  const served = useChatServed();
  const { send } = useChatActions(agent);
  // Whether a turn is running, for Stop: the same read the stream suspends on, here without suspending.
  const running = useChat(agent, 'all').working.turn_running;
  const run = useVerbRun(agent);
  const compacting = useCompacting(agent);
  const switchLock = useSwitchLock(agent);
  const compactNow = useCompactNow(agent);
  // eslint-disable-next-line no-restricted-syntax -- the /terminal command has no click: it opens the terminal beside
  const openTab = useOpenTab();
  const stop = () =>
    run(VERBS.interrupt, undefined, {
      onError: (e) => toast(`Stop did not work: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' }),
    });
  const commands: Command[] = [
    { name: 'stop', hint: 'Stop the turn running now', icon: <Square size={14} />, run: stop },
    {
      name: 'compact',
      hint: 'Compact the context now',
      icon: <FoldVertical size={14} />,
      run: () => (compactNow ? compactNow() : toast('Compact is not open now.', { tone: 'warning' })),
    },
    { name: 'terminal', hint: 'Open the terminal beside', icon: <SquareTerminal size={14} />, run: () => openTab('shell', 'side') },
  ];
  const [draft, setDraft] = useDraft(agent);
  const attach = useAttachments(agent);
  // A screenshot marked in a Web tab goes with the next message too.
  const marked = useRef(attach.marked);
  useEffect(() => {
    marked.current = attach.marked;
  });
  useEffect(() => takeToChat(agent, (m, send) => marked.current.add(m, send)), [agent]);
  // The message the next one replies to, picked by Reference or Quote.
  const [reply, setReply] = useTabState<Reply | null>('reply', null);
  const alongside = useAlongside(agent);
  // The stream's scroll hears of a send through this, once the stream is drawn.
  const stream = useRef<{ sent: () => void }>(null);
  const loading = (
    <Shell>
      <ChatSkeleton />
    </Shell>
  );

  return (
    // Files dropped anywhere on the chat go with the next message.
    <FileDropArea take={attach.add} className="flex min-h-0 min-w-0 flex-1 flex-col">
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
          <MarkReply.Provider value={{ marked: attach.marked, label: 'Add to reply' }}>
            <Stream ref={stream} agent={agent} onReply={(r) => setReply(r)} />
          </MarkReply.Provider>
        </Boundary>
      ) : (
        loading
      )}
      <Page>
        {/* Outside the stream's loading and failure, so its button is there whatever the chat shows. */}
        <HoldCard agent={agent} />
        <SwitchWait agent={agent} />
        <Composer
          agent={agent}
          draft={draft}
          setDraft={setDraft}
          attach={attach}
          leading={
            <>
              <ModelChip agent={agent} />
              <AgentSettingsButton agent={agent} />
            </>
          }
          trailing={<ContextRing agent={agent} />}
          running={running}
          locked={!!compacting || !!switchLock}
          onStop={stop}
          commands={commands}
          focusOn={reply}
          above={
            alongside.length || reply ? (
              <>
                {alongside.length > 0 && <AlongsideStrip running={alongside} />}
                {reply && (
                  <div role="status" className="-ml-2 flex min-w-0 items-center gap-1.5 text-xs text-muted">
                    <CornerDownRight size={14} className="ml-1 shrink-0" />
                    <span className="min-w-0 flex-1 truncate">Replying to: {(reply.quote ?? reply.text).replace(/\s+/g, ' ')}</span>
                    <IconButton className="size-6!" label="Drop the reply" onClick={() => setReply(null)}>
                      <X size={14} />
                    </IconButton>
                  </div>
                )}
              </>
            ) : null
          }
          onSend={(body, attachments, task) => {
            // A command goes as typed, so the reply is not put on it.
            const to = reply && !body.startsWith('!') ? reply : null;
            send(body, {
              attachments,
              ...(task ? { task } : {}),
              ...(to?.kind === 'message' ? { parent: to.id, quote: to.quote } : to ? { parent_item: to.id } : {}),
            });
            setReply(null);
            stream.current?.sent();
          }}
        />
        <StatusLine agent={agent} />
      </Page>
    </FileDropArea>
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
function Stream({ agent, onReply, ref }: { agent: string; onReply: (r: Reply) => void; ref: Ref<{ sent: () => void }> }) {
  // Every message the agent sent or received (the all scope), drawn by the chat's own rules (mine): app-core's all view leaves out
  // what the agent says to other agents, which this chat shows as quiet lines (ChatLine drops what others say to it).
  const chat = useChatSuspense(agent, 'all');
  // Opening the chat reads what the agent said: its newest message, and what it sent the Inbox about itself.
  useChatReadMarks(agent, chat);
  // Each task once, as a row where it first came up, changed in place by what comes later.
  const lines = useChatRows(chat, agent, useTaskActivity(agent));
  const { entries, rows } = lines;
  const view = useAgents().data?.agents.find((a) => a.key === agent);
  const heldOff = !!view?.held_on?.restart_here && view.held_on.silent_since !== null;
  const doing = view?.doing?.text;
  const held = view?.task;
  // A compaction, a move or a restart under way takes the turn's row, in place of the box being locked.
  const compacting = useCompacting(agent);
  const switching = useSwitchLock(agent);
  const restart =
    (compacting ? COMPACTING_WORDS[compacting] : switching) ?? (view?.handoff && restarting(view) ? restartingLine(view.handoff) : null);
  const busy = chat.working.turn_running || !!restart;
  const hold = useHoldLine(agent);
  const now = useNow();
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
  // The index of the last line drawn before this render; what is at or above it (an older page included) just shows.
  const end = entries.at(-1);
  const endKey = end ? `${end.kind}:${end.id}` : null;
  const [tail, setTail] = useState({ end: endKey, before: endKey });
  if (tail.end !== endKey) setTail({ end: endKey, before: tail.end });
  // Rows after the last one drawn before this render are new and fade in; an older page landing above just shows.
  const known = knownUpTo(rows, entries, tail.before);
  const top = useNearEnd<HTMLDivElement, HTMLDivElement>(stream, chat.loadOlder, chat.hasOlder && !chat.loadingOlder, {
    atTop: true,
    margin: 400,
  });

  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
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
                <Spinner />
                Loading earlier messages
              </div>
            )}
            {/* Only lines after the last one known fade in: an older page lands as one still block. */}
            <ChatRows lines={lines} agent={agent} known={known} onReply={onReply} />
            {/* The dots' row is always there, so turning them on or off does not move the chat. */}
            {/* A move kept for the turn's end takes the row, at its height: the move or the dots, never both. */}
            {view?.pending_move ? (
              <PendingMove agent={agent} />
            ) : heldOff ? (
              <HeldCard agent={agent} />
            ) : (
              <FootRow
                busy={busy}
                hold={hold}
                restart={restart}
                move={!compacting && switching ? <MoveSteps agent={agent} /> : undefined}
                held={held}
                doing={doing}
                turn={turnMinutes(chat.working.since, now)}
              />
            )}
            <PauseCard agent={agent} />
            <PromptCard agent={agent} />
            <ProposalCards agent={agent} />
          </Page>
        </div>
      </div>
      <JumpDown box={stream} working={busy} onDown={scroll.bottom} />
    </div>
  );
}

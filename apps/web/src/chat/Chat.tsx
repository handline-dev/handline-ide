import {
  chatView,
  foldBash,
  foldTurns,
  unblockedStart,
  isPerson,
  turnMinutes,
  type TimelineEntry,
  type TurnRecord as Turn,
} from '@handline/app-core';
import { useAgents, useChat, useChatActions, useChatReadMarks, useChatServed, useChatSuspense, useVerbRun } from '@handline/app-data';
import { VERBS } from '@handline/app-core';
import { useMe, useNearEnd } from '@handline/app-react';
import { AnimatePresence, motion } from 'framer-motion';
import { spring } from '#src/motion.ts';
import { LoaderCircle } from 'lucide-react';
import { useEffect, useImperativeHandle, useRef, useState, type ReactNode, type Ref } from 'react';
import { ContextRing } from '#src/chat/agent/ContextRing.tsx';
import { COMPACTING_WORDS, useCompacting, useCompactNow } from '#src/chat/agent/compaction.ts';
import { SwitchWait, useSwitchLock } from '#src/chat/agent/AccountSwitch.tsx';
import type { Command } from '#src/chat/Suggest.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { ModelChip } from '#src/chat/agent/ModelChip.tsx';
import { StatusLine } from '#src/chat/agent/StatusLine.tsx';
import { HoldBar } from '#src/chat/agent/HoldBar.tsx';
import { PauseCard } from '#src/chat/PauseCard.tsx';
import { PromptCard } from '#src/chat/PromptCard.tsx';
import { takeToChat } from '#src/chat/toChat.ts';
import { TaskActivity, useTaskActivity, type TaskEvent } from '#src/chat/TaskActivity.tsx';
import { useAttachments } from '#src/chat/attachments.tsx';
import { ChatLine, drawsNothing, type Reply } from '#src/chat/ChatLine.tsx';
import { CornerDownRight, FoldVertical, Square, SquareTerminal, X } from 'lucide-react';
import { ChatSkeleton } from '#src/chat/ChatSkeleton.tsx';
import { TurnRecord } from '#src/chat/TurnRecord.tsx';
import { Composer } from '#src/chat/Composer.tsx';
import { MarkReply, markInto } from '#src/chat/MessageImages.tsx';
import { Dots } from '#src/chat/Dots.tsx';
import { useDraft } from '#src/chat/drafts.ts';
import { useChatScroll } from '#src/chat/useChatScroll.ts';
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
  const served = useChatServed();
  const { send } = useChatActions(agent);
  // Whether a turn is running, for Stop: the same read the stream suspends on, here without suspending.
  const running = useChat(agent, 'all').working.turn_running;
  const run = useVerbRun(agent);
  const compacting = useCompacting(agent);
  const switchLock = useSwitchLock(agent);
  const compactNow = useCompactNow(agent);
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
  // A picture marked up in the chat or the box goes with the next message.
  const addMarked = markInto(attach, setDraft);
  // A screenshot marked in a Web tab comes here too.
  const marked = useRef(addMarked);
  useEffect(() => {
    marked.current = addMarked;
  });
  useEffect(() => takeToChat(agent, (m) => marked.current(m)), [agent]);
  // The message the next one replies to, picked by Reference or Quote.
  const [reply, setReply] = useTabState<Reply | null>('reply', null);
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
      <HoldBar agent={agent} />
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
          <MarkReply.Provider value={{ add: addMarked, label: 'Add to reply' }}>
            <Stream ref={stream} agent={agent} onReply={(r) => setReply(r)} />
          </MarkReply.Provider>
        </Boundary>
      ) : (
        loading
      )}
      <Page>
        <SwitchWait agent={agent} />
        <Composer
          agent={agent}
          draft={draft}
          setDraft={setDraft}
          attach={attach}
          onMarked={addMarked}
          leading={<ModelChip agent={agent} />}
          trailing={<ContextRing agent={agent} />}
          running={running}
          locked={compacting ? COMPACTING_WORDS[compacting] : (switchLock ?? undefined)}
          onStop={stop}
          commands={commands}
          focusOn={reply}
          above={
            reply && (
              <div role="status" className="-ml-2 flex min-w-0 items-center gap-1.5 text-xs text-muted">
                <CornerDownRight size={14} className="ml-1 shrink-0" />
                <span className="min-w-0 flex-1 truncate">Replying to: {(reply.quote ?? reply.text).replace(/\s+/g, ' ')}</span>
                <IconButton className="size-6!" label="Drop the reply" onClick={() => setReply(null)}>
                  <X size={14} />
                </IconButton>
              </div>
            )
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

/** A task this agent gave or handed another agent, or one another agent gave or handed it, said as a message (kind task): drawn as the task, not as talk. */
const taskNotice = (e: TimelineEntry | Turn, agent: string, me: string) =>
  e.kind === 'message' &&
  e.message.kind === 'task' &&
  e.message.tasks.length > 0 &&
  (e.message.from === agent ? !isPerson(e.message.to, me) : e.message.to === agent && !isPerson(e.message.from, me));

/** Whether a notice says a task was handed on, or made and given. */
const noticeKind = (body: string) => (/^T\d+\s+handed\b/i.test(body) ? 'handoff' : 'add');

/** A run of task rows as one card. `id`, its React key, names the line before it, so the card changes in place as rows join or give way to
    their notice; `key` names its newest notice, so the reader's line is found again when an older page lands. */
type Card = { id: string; key: string; events: TaskEvent[] };

/** The entries, with each run of task notices gathered into one card's rows. */
function runs(entries: (TimelineEntry | Turn)[], agent: string, me: string, live: TaskEvent[] = []): (TimelineEntry | Turn | Card)[] {
  const out: (TimelineEntry | Turn | Card)[] = [];
  // Task changes heard while the chat is open go in by their time, joining a card next to them, so they scroll up as talk goes on.
  // A task made and given, or handed on, arrives twice, live and as the notice message: the notice's row is the one kept.
  const noticed = new Set(
    entries.flatMap((e) =>
      e.kind === 'message' && taskNotice(e, agent, me)
        ? e.message.tasks.map((t) => `${noticeKind(e.message.body)}:T${t.id}:${e.message.to}`)
        : [],
    ),
  );
  const heard = live
    .filter((ev) => !noticed.has(`${ev.kind}:${ev.id}:${ev.to}`))
    .sort((a, b) => Date.parse(a.at ?? '') - Date.parse(b.at ?? ''));
  const add = (rows: TaskEvent[], key: string) => {
    const last = out.at(-1);
    if (last && 'events' in last) {
      last.events.push(...rows);
      last.key = key;
    } else out.push({ id: `tasks:after:${last ? `${last.kind}:${last.id}` : ''}`, key, events: rows });
  };
  const until = (at: string) => {
    while (heard.length && Date.parse(heard[0]!.at ?? '') <= Date.parse(at)) {
      const ev = heard.shift()!;
      add([ev], `tasks:${ev.key}`);
    }
  };
  for (const e of entries) {
    until(e.at);
    const freed = e.kind === 'item' ? unblockedStart(e.item) : null;
    if (freed) {
      // The title is read with the task's state (TaskActivity).
      add(
        [{ key: `u${e.id}`, kind: 'start', id: `T${freed.task}`, title: '', detail: freed.detail, at: e.at, line: `item:${e.id}` }],
        `tasks:u${e.id}`,
      );
      continue;
    }
    if (!taskNotice(e, agent, me) || e.kind !== 'message') {
      // A line that draws nothing is kept out of the way: it goes before the card, so the run goes on.
      const last = out.at(-1);
      if (last && 'events' in last && e.kind !== 'record' && drawsNothing(e, agent, me)) out.splice(out.length - 1, 0, e);
      else out.push(e);
      continue;
    }
    const m = e.message;
    // A task handed on from another holder, or a new one made and given: said in words, its icon telling which.
    const handed = noticeKind(m.body) === 'handoff';
    // Given to this agent: where its work starts, from whom.
    const got = m.to === agent;
    const rows = m.tasks.map((t) => ({
      key: `${m.id}:${t.id}`,
      kind: got ? ('start' as const) : handed ? ('handoff' as const) : ('add' as const),
      id: `T${t.id}`,
      title: t.title,
      detail: got ? `${handed ? 'Handed' : 'Assigned'} by ${m.from}` : handed ? `Handed to ${m.to}` : `Created and assigned to ${m.to}`,
      at: m.at,
    }));
    add(rows, `tasks:${m.id}`);
  }
  until(new Date(8.64e15).toISOString());
  return out;
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
  const me = useMe();
  // Opening the chat reads what the agent said: its newest message, and what it sent the Inbox about itself.
  useChatReadMarks(agent, chat);
  const { entries, bash } = foldBash(chatView(chat, 'mine', agent, me, [], true).entries);
  const view = useAgents().data?.agents.find((a) => a.key === agent);
  const doing = view?.doing?.text;
  const held = view?.task;
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
  const known = (all: ReturnType<typeof runs>) => {
    const at = all.map((r) =>
      'events' in r
        ? r.events.map((e) => e.line ?? `message:${e.key.split(':')[0]}`)
        : r.kind === 'record'
          ? r.items.map((i) => `item:${i.id}`)
          : [`${r.kind}:${r.id}`],
    );
    for (let i = at.length - 1; i >= 0; i--) if (at[i]!.includes(tail.before ?? '')) return i;
    return Infinity;
  };
  // The words of each loaded message, for the line above a reply.
  const words = new Map(entries.flatMap((e) => (e.kind === 'message' ? [[e.message.id, e.message.body] as const] : [])));
  const tasks = useTaskActivity(agent);
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
            {/* Task notices in a row are one card, as the live task card is (TaskActivity). Only lines after the last one known fade in:
                an older page lands as one still block. */}
            {runs(foldTurns(entries), agent, me, tasks).map((r, i, all) =>
              'events' in r ? (
                <motion.div
                  key={r.id}
                  data-id={r.key}
                  initial={i <= known(all) ? false : { opacity: 0 }}
                  animate={{ opacity: 1 }}
                  transition={spring}
                  className="flex"
                >
                  <TaskActivity events={r.events} />
                </motion.div>
              ) : r.kind === 'record' ? (
                <TurnRecord key={`record:${r.id}`} r={r} fade={i > known(all)} />
              ) : (
                <ChatLine
                  key={`${r.kind}:${r.id}`}
                  e={r}
                  agent={agent}
                  me={me}
                  onReply={onReply}
                  parentText={r.kind === 'message' && r.message.parent != null ? words.get(r.message.parent) : undefined}
                  bash={r.kind === 'message' ? bash.get(r.message.id) : undefined}
                  fade={i > known(all)}
                />
              ),
            )}
          </AnimatePresence>
          {/* The dots' row is always there, so turning them on or off does not move the chat. */}
          <div
            className={`h-5 transition-opacity ${chat.working.turn_running ? 'opacity-100' : 'opacity-0'}`}
            aria-hidden={!chat.working.turn_running}
          >
            <span className="flex items-center gap-2">
              <Dots />
              {/* The held task, this turn's minutes, then its newest tool call (AgentView.task, .doing). */}
              <span className="min-w-0 truncate text-xs text-muted">
                {held && (
                  <span className="font-medium text-fg">
                    T{held.id} · {turnMinutes(chat.working.since, now)}
                  </span>
                )}
                {held && doing && ' · '}
                {doing}
              </span>
            </span>
          </div>
          <PauseCard agent={agent} />
          <PromptCard agent={agent} />
        </Page>
      </div>
    </div>
  );
}

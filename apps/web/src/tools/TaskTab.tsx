import { activityOf, foundersTurn, taskOutcome, isPerson, turnChain, isSystem, type ActivityItem } from '@handline/app-core';
import { hooks, useTaskHistory, useTaskSeen, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import {
  Check,
  ChevronDown,
  CircleDot,
  Ellipsis,
  Hand,
  MessageSquare,
  Pencil,
  RotateCcw,
  Send,
  Trash2,
  Undo2,
  UserRound,
} from 'lucide-react';
import { look } from '#src/ui/taskLook.tsx';
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { ago, clock } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Linked, Markdown } from '#src/ui/Markdown.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { Actions, CopyButton } from '#src/chat/MessageActions.tsx';
import { Reference } from '#src/chat/ChatLine.tsx';
import { MessageImages } from '#src/chat/MessageImages.tsx';
import { ReadBy } from '#src/tools/ReadBy.tsx';
import { Fields } from '#src/tools/TaskFields.tsx';
import { GO_TURN, goToTurn, showMemo } from '#src/tools/TaskTurn.ts';
import { showHistory } from '#src/tools/TaskDiff.tsx';
import { CommentBox, type Reply } from '#src/tools/TaskComment.tsx';
import { useAttachments } from '#src/chat/attachments.tsx';
import { FileDropArea } from '#src/ui/fileDrop.tsx';
import { WaitingOnYou, YourTurn } from '#src/tools/TaskWaiting.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { keptScroll } from '#src/ui/keptScroll.ts';
import { toast } from '#src/ui/toast.tsx';
import { useTabState } from '#src/persist/tabState.ts';

const CLOSED = ['done', 'discarded'];

/** One task, opened from a T-number: its face as in the chat, a line of facts, why it waits, the brief, sub-tasks and result, then
 * its activity (comments and changes in one list) and a box to comment in, the chat's own. */
export function TaskTab({ id }: { id: number }) {
  const q = hooks.useTaskGet({ task: id });
  const me = useMe();
  useTaskSeen(id);
  // The comment the box below answers, picked by Reply or a comment's Reference.
  const [replyTo, setReplyTo] = useTabState<Reply | null>('reply', null);
  // Files dropped anywhere on the task go with the next comment, as in the chat.
  const attach = useAttachments(`task:${id}`);
  // Going to the task's turn: the Your turn click, and a request-list click that left a mark (ui/keptScroll) before opening the task.
  const goRef = useRef<() => void>(() => {});
  const loaded = !!q.data;
  useEffect(() => {
    goRef.current = () => q.data && goToTurn(q.data);
  });
  useEffect(() => {
    if (loaded && keptScroll.take(`turn:${id}`)) setTimeout(() => goRef.current(), 100);
  }, [loaded, id]);
  useEffect(() => {
    const on = (e: Event) => {
      if ((e as CustomEvent<number>).detail !== id) return;
      keptScroll.take(`turn:${id}`);
      goRef.current();
    };
    window.addEventListener(GO_TURN, on);
    return () => window.removeEventListener(GO_TURN, on);
  }, [id]);
  const t = q.data;
  if (q.isError) return <p className="m-auto text-sm text-danger">T{id} did not load.</p>;
  if (!t) return <p className="m-auto text-sm text-muted">Loading T{id}</p>;
  const open = !CLOSED.includes(t.state);
  const s = look(t);
  // Blockers have their own rows: "blocked by T<n>" over them would name each twice.
  const why = t.blockers.length ? null : (t.waiting?.text ?? t.block_reason ?? t.hold_reason);
  const mine = open && foundersTurn(t, me);
  const chain = turnChain(t, me);
  // Waiting on the founder: what was asked, in the words of the last comment that named them.
  const asked =
    t.waiting && (t.waiting.on === 'founder' || isPerson(t.waiting.on, me))
      ? [...t.memos].reverse().find((m) => /@human\b/.test(m.text))
      : undefined;
  return (
    <FileDropArea take={attach.add} className="flex min-h-0 min-w-0 flex-1 flex-col">
      <ScrollFade keep={`task:${id}`} data-task-scroll={id} className="min-h-0 flex-1">
        <Page className="flex flex-col gap-6 py-4 text-base text-fg">
          <header className="flex flex-col gap-3">
            <div className="flex items-start gap-3">
              <h1 className="min-w-0 flex-1 text-lg font-semibold">
                {/* The state said once, in the title's first line: its mark and its word on its colour; the title wraps after it. */}
                <span
                  className={`mr-2 inline-flex h-6 items-center gap-1 rounded-md px-2 align-middle text-xs font-medium text-accent-fg ${s.tone}`}
                >
                  <s.icon size={12} strokeWidth={3} />
                  {s.word}
                </span>
                <span className="mr-1.5 font-normal text-muted">T{t.id}</span>
                <span className={t.state === 'discarded' ? 'text-muted line-through' : ''}>{t.title}</span>
              </h1>
              <Verbs id={t.id} open={open} state={t.state} />
            </div>
            <p className="text-xs text-muted">
              Reports to {isPerson(t.asker, me) ? 'you' : t.asker} · <span title={t.created}>created {ago(t.created)}</span>
              {open && chain && <> · {chain}</>}
              {t.done && (
                <>
                  {' '}
                  · <span title={t.done}>closed {ago(t.done)}</span>
                </>
              )}
              {t.parent && (
                <>
                  {' '}
                  · under <TaskLink id={t.parent} title={t.parent_title ?? undefined} />
                </>
              )}
            </p>
            <Fields t={t} open={open} />
            {/* Shown only while something holds it: what it waits on, and why. */}
            {open && (mine || why || t.blockers.length > 0) && (
              <div data-turn-slot className="flex flex-col gap-1 rounded-lg bg-warning/10 px-3 py-2 text-sm transition-colors">
                {mine ? (
                  <YourTurn t={t} reply={replyTo} onDone={() => void q.refetch()} onReply={setReplyTo} />
                ) : asked ? (
                  <WaitingOnYou t={t} me={me} asked={asked} onDone={() => void q.refetch()} onShow={() => showMemo(asked.id)} />
                ) : (
                  why && (
                    <p className="flex items-start gap-2">
                      <Hand size={14} className="mt-1 shrink-0 text-warning" />
                      <Linked text={why} />
                    </p>
                  )
                )}
                {t.blockers.map((b, i) => (
                  <p key={b.task} className={`flex min-w-0 items-center gap-2 ${i > 0 ? 'pl-6' : ''}`}>
                    {i === 0 && <Hand size={14} className="shrink-0 text-warning" />}
                    <span className="shrink-0 whitespace-nowrap text-muted">Waiting on</span>
                    <TaskLink id={b.task} title={b.title} />
                  </p>
                ))}
              </div>
            )}
          </header>
          {t.body && (
            <Flash text={t.body}>
              <Markdown text={t.body} />
            </Flash>
          )}
          {t.subtasks.length > 0 && (
            <Section title="Sub-tasks">
              {t.subtasks.map((c) => (
                <TaskLink key={c.id} id={c.id} title={c.title} />
              ))}
            </Section>
          )}
          {t.evidence && (
            <Section title="Result">
              <ResultChips evidence={t.evidence} title={t.title} />
              <Flash text={t.evidence}>
                <Markdown text={t.evidence} />
              </Flash>
            </Section>
          )}
          <Activity
            task={t.id}
            onReply={(id) => setReplyTo(t.memos.find((m) => m.id === id) ?? null)}
            items={activityOf(t.memos, t.timeline).filter((i) => !isSystem(i.who))}
          />
        </Page>
      </ScrollFade>
      <CommentBox t={t} reply={replyTo} onDrop={() => setReplyTo(null)} attach={attach} onSent={() => toBottom(id)} />
    </FileDropArea>
  );
}

/** Words that flash when they change after the task opened; drawn again for each change so each one flashes. */
function Flash({ text, children }: { text: string; children: ReactNode }) {
  const [first] = useState(text);
  return (
    <div key={text} className={`rounded-md ${text === first ? '' : 'just-changed'}`}>
      {children}
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-sm font-semibold text-muted">{title}</h2>
      {children}
    </section>
  );
}

/** A task by its number and title; a click opens it, Cmd+click beside. */
function TaskLink({ id, title }: { id: number; title?: string }) {
  return (
    <button type="button" {...useOpenable(taskOf(id))} className="min-w-0 truncate rounded-sm text-left hover:bg-hover/30">
      <span className="text-accent">T{id}</span>
      {title && <span className="ml-1 text-fg">{title}</span>}
    </button>
  );
}

/** The task's verbs: Done or Reopen, Discard. */
function Verbs({ id, open, state }: { id: number; open: boolean; state: string }) {
  const done = useWrite('task.done');
  const reopen = useWrite('task.reopen');
  const discard = useWrite('task.discard');
  const failed = (what: string) => (e: unknown) =>
    toast(`${what} did not work: ${e instanceof Error ? e.message : String(e)}`, { tone: 'warning' });
  return (
    // Every verb in the one menu: the state is read at the title, the verbs are asked for here.
    <div className="flex shrink-0 items-center">
      <Menu
        align="end"
        trigger={
          <IconButton label="More">
            <Ellipsis size={16} />
          </IconButton>
        }
      >
        {open && (
          <MenuItem icon={<Check size={14} />} onClick={() => done.mutate({ task: id }, { onError: failed('Done') })}>
            Done
          </MenuItem>
        )}
        {state === 'done' && (
          <MenuItem icon={<RotateCcw size={14} />} onClick={() => reopen.mutate({ task: id }, { onError: failed('Reopen') })}>
            Reopen
          </MenuItem>
        )}
        {open && (
          <MenuItem danger icon={<Trash2 size={14} />} onClick={() => discard.mutate({ task: id }, { onError: failed('Discard') })}>
            Discard
          </MenuItem>
        )}
      </Menu>
    </div>
  );
}

/** After the founder comments: the page goes to its end, where the new comment is, once it is drawn. */
function toBottom(id: number) {
  setTimeout(() => {
    const el = document.querySelector(`[data-task-scroll="${id}"]`);
    el?.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
  }, 50);
}

/** What a change was, from its sentence: assigned or handed, done, discarded, edited, sent back, a request, or another state. */
function eventIcon(line: string): typeof Check {
  if (/handed it|assigned/.test(line)) return UserRound;
  if (/marked it done/.test(line)) return Check;
  if (/discarded/.test(line)) return Trash2;
  if (/changed the brief/.test(line)) return Pencil;
  if (/sent it back/.test(line)) return Undo2;
  if (/sent a request/.test(line)) return Send;
  return CircleDot;
}

/** Under a line, as under a chat message: its buttons and time on hover; the row keeps its height hidden, as the space between lines. */
function HoverRow({ at, edited, onEdited, children }: { at: string; edited?: boolean; onEdited?: () => void; children?: ReactNode }) {
  return (
    <div className="mb-0.5 flex h-6 items-center gap-1.5 text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
      {children}
      <time dateTime={at} title={at}>
        {clock(at)}
        {edited && !onEdited && ' · edited'}
      </time>
      {edited && onEdited && (
        <button type="button" onClick={onEdited} className="underline decoration-dotted hover:text-fg">
          · edited
        </button>
      )}
    </div>
  );
}

/** The small round mark before each line of the activity. */
function Mark({ icon: Icon }: { icon: typeof Check }) {
  return (
    <span className="grid size-6 shrink-0 place-items-center rounded-full bg-surface text-muted">
      <Icon size={12} />
    </span>
  );
}

/** A line's key: a comment by its number and words, so an edit draws it again; an event by its time and words. */
const keyOf = (i: ActivityItem) => (i.kind === 'comment' ? `c${i.memo}:${i.note}` : `e${i.at}${i.line}`);

/** How many of the latest lines show before Earlier is opened. */
const SHOWN = 5;

/** Comments and changes in one list, oldest first: the last five, and Earlier opens the rest above them. */
function Activity({ task, items, onReply }: { task: number; items: ActivityItem[]; onReply: (memo: number) => void }) {
  const [all, setAll] = useTabState('earlier', false);
  // The texts an edited comment had: its history rows. Its 'edited' mark opens them in a window.
  const rows = useTaskHistory(task).data?.rows;
  const earlierOf = (memo?: number | null) =>
    (rows ?? []).filter((r) => r.field === 'memo' && r.memo === memo && r.was && r.now).sort((a, b) => a.at.localeCompare(b.at));
  // What was there when the task opened just shows; a comment or event that comes later, or a comment's new words, flash.
  const [first] = useState(() => new Set(items.map(keyOf)));
  const fresh = (i: ActivityItem) => (first.has(keyOf(i)) ? '' : 'just-changed');
  const shown = all ? items : items.slice(-SHOWN);
  const hidden = items.length - shown.length;
  // The line that was first when Earlier was clicked, and where it stood: the older ones come in above it, and it stays put.
  const list = useRef<HTMLDivElement>(null);
  const kept = useRef<{ el: Element; top: number } | null>(null);
  useLayoutEffect(() => {
    const k = kept.current;
    kept.current = null;
    if (!k) return;
    let box = k.el.parentElement;
    while (box && !/(auto|scroll)/.test(getComputedStyle(box).overflowY)) box = box.parentElement;
    if (box) box.scrollTop += k.el.getBoundingClientRect().top - k.top;
  }, [all]);
  if (!items.length) return null;
  return (
    <Section title="Activity">
      {hidden > 0 && (
        <button
          type="button"
          onClick={() => {
            const el = list.current?.firstElementChild;
            if (el) kept.current = { el, top: el.getBoundingClientRect().top };
            setAll(true);
          }}
          className="flex w-fit items-center gap-1 rounded-sm text-xs text-muted hover:bg-hover/30"
        >
          <ChevronDown size={12} />
          Earlier {hidden}
        </button>
      )}
      <div ref={list} className="flex flex-col">
        {shown.map((i) =>
          i.kind === 'comment' ? (
            // A comment as a chat line: who, the words, and under them the chat's hover row (copy, reference, time).
            <div key={keyOf(i)} data-memo={i.memo} className="group flex gap-2.5 rounded-md transition-colors">
              <Mark icon={MessageSquare} />
              <div className="flex min-w-0 flex-1 flex-col">
                <div className={`flex flex-col rounded-md ${fresh(i)}`}>
                  <span className="text-xs font-medium text-muted">{i.who}</span>
                  {i.replyTo != null && (
                    <Reference text={items.find((r) => r.memo === i.replyTo)?.note} onClick={() => showMemo(i.replyTo!)} />
                  )}
                  <Markdown text={i.note} />
                  {i.images && <MessageImages images={i.images} />}
                </div>
                <HoverRow
                  at={i.at}
                  edited={i.edited}
                  onEdited={
                    i.memo != null && earlierOf(i.memo).length
                      ? () =>
                          showHistory('Versions of this comment', [
                            { text: earlierOf(i.memo)[0]!.was, at: i.at, by: i.who },
                            ...earlierOf(i.memo).map((r) => ({ text: r.now, at: r.at, by: r.by })),
                          ])
                      : undefined
                  }
                >
                  <Actions text={i.note} onQuote={() => i.memo != null && onReply(i.memo)} />
                  <ReadBy readers={i.readers} />
                </HoverRow>
              </div>
            </div>
          ) : (
            <div key={keyOf(i)} className="group flex gap-2.5 text-xs text-muted">
              <Mark icon={eventIcon(i.line)} />
              <div className="flex min-w-0 flex-1 flex-col pt-0.5">
                <div className={`flex flex-col rounded-md ${fresh(i)}`}>
                  <span>{i.line}</span>
                  {i.note && (
                    <div className="mt-1 border-l-2 border-border pl-3 text-sm text-fg">
                      <Markdown text={i.note} />
                    </div>
                  )}
                </div>
                <HoverRow at={i.at}>{i.note && <CopyButton text={i.note} />}</HoverRow>
              </div>
            </div>
          ),
        )}
      </div>
    </Section>
  );
}

/** The chat's message box, writing a comment on the task; its draft and files are kept per task. */

/** What the work came to: the pull request when the evidence names one, else the commit. */
function ResultChips({ evidence, title }: { evidence: string; title: string }) {
  const o = taskOutcome(evidence, title);
  const chip = o.pr !== null ? `PR #${o.pr}` : o.sha;
  if (!chip) return null;
  return <span className="mb-2 inline-block w-fit rounded-md bg-surface px-2 py-0.5 font-mono text-xs text-fg">{chip}</span>;
}

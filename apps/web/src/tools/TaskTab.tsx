import { outsideBlockers, subtaskCount } from '#src/tools/subtasks.ts';
import { useLeaseWaits } from '#src/tools/leaseWait.ts';
import { LeaseIcon } from '#src/tools/LeaseIcon.tsx';
import { activityOf, foundersTurn, taskOutcome, isPerson, turnChain, isSystem, type ActivityItem } from '@handline/app-core';
import { hooks, useNameOf, useTaskHistory, useTaskSeen, useWrite } from '@handline/app-data';
import { CommentRow, EventRow, type Turn } from '#src/tools/TaskEvents.tsx';
import { useMe } from '@handline/app-react';
import { Check, Ellipsis, Hourglass, RotateCcw, Trash2 } from 'lucide-react';
import { look, TaskState, type Lookable } from '#src/ui/taskLook.tsx';
import { useEffect, useState, type ReactNode } from 'react';
import { ago } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { Linked, Markdown } from '#src/ui/Markdown.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { MarkReply, markInto } from '#src/chat/MessageImages.tsx';
import { useDraft } from '#src/chat/drafts.ts';
import { Fields } from '#src/tools/TaskFields.tsx';
import { GO_TURN, goToTurn, openTurnRow, takeTyping, type OpenTurn } from '#src/tools/TaskTurn.ts';
import { showHistory } from '#src/tools/TaskDiff.tsx';
import { CommentBox, type Reply } from '#src/tools/TaskComment.tsx';
import { useAttachments } from '#src/chat/attachments.tsx';
import { FileDropArea } from '#src/ui/fileDrop.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { keptScroll } from '#src/ui/keptScroll.ts';
import { sayRefusal } from '#src/ui/refusal.tsx';
import { toast } from '#src/ui/toast.tsx';
import { useTabState } from '#src/persist/tabState.ts';

const CLOSED = ['done', 'discarded'];

/** One task, opened from a T-number: its face as in the chat, a line of facts, why it waits, the brief, sub-tasks and result, then
 * its activity (comments and changes in one list) and a box to comment in, the chat's own. */
export function TaskTab({ id }: { id: number }) {
  const q = hooks.useTaskGet({ task: id });
  const leases = useLeaseWaits();
  const me = useMe();
  const nameOf = useNameOf();
  useTaskSeen(id);
  // The comment the box below answers, picked by Reply or a comment's Reference.
  const [replyTo, setReplyTo] = useTabState<Reply | null>('reply', null);
  // Files dropped anywhere on the task go with the next comment, as in the chat.
  const attach = useAttachments(`task:${id}`);
  // The box's draft is held here so a picture marked up in the task's viewer lands in it.
  const [draft, setDraft] = useDraft(`task:${id}`);
  const addMarked = markInto(attach, setDraft);
  // Opens at the founder's turn (a first open, or a request-list click's mark); a GO_TURN to an open tab goes there again.
  const loaded = !!q.data;
  const onTurn = !!q.data && !CLOSED.includes(q.data.state) && foundersTurn(q.data, me);
  // Bumped when Answer in task asks the comment box to take the keys: as the tab opens, or later through GO_TURN.
  const [typing, setTyping] = useState(() => (takeTyping(id) ? 1 : 0));
  useEffect(() => {
    if (!loaded) return;
    const marked = keptScroll.take(`turn:${id}`);
    if (!marked && !(onTurn && keptScroll.get(`task:${id}`) === undefined)) return;
    const at = setTimeout(() => goToTurn(id), 100);
    return () => clearTimeout(at);
  }, [loaded, onTurn, id]);
  useEffect(() => {
    const on = (e: Event) => {
      if ((e as CustomEvent<number>).detail !== id) return;
      keptScroll.take(`turn:${id}`);
      if (takeTyping(id)) setTyping((n) => n + 1);
      goToTurn(id);
    };
    window.addEventListener(GO_TURN, on);
    return () => window.removeEventListener(GO_TURN, on);
  }, [id]);
  const t = q.data;
  if (q.isError) return <p className="m-auto text-sm text-danger">T{id} did not load.</p>;
  if (!t) return <TaskLoading id={id} />;
  // In line for a machine resource: the line says which and who has it, in place of the daemon's gate task it waits on.
  const lease = leases.get(t.id) ?? null;
  const blockers = outsideBlockers(t).filter((b) => !(lease && b.owner === 'gate'));
  const open = !CLOSED.includes(t.state);
  const s = look(t);
  // Blockers have their own rows: "blocked by T<n>" over them would name each twice.
  const why = lease?.text ?? (t.blockers.length ? null : (t.waiting?.text ?? t.block_reason ?? t.hold_reason));
  const mine = open && foundersTurn(t, me);
  const chain = turnChain(t, me, nameOf);
  const turn = t.turns?.at(-1);
  return (
    <FileDropArea take={attach.add} className="flex min-h-0 min-w-0 flex-1 flex-col">
      <MarkReply.Provider value={{ add: addMarked, label: 'Add to comment' }}>
        <ScrollFade keep={`task:${id}`} fromEnd data-task-scroll={id} className="min-h-0 flex-1">
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
              {/* The founder's turn is its row in Activity; up here only why the task waits on someone else. */}
              {open && !mine && why && (
                <p className="flex items-start gap-2 text-sm text-muted">
                  {lease ? <LeaseIcon kind={lease.kind} size={14} /> : <Hourglass size={14} className="mt-0.5 shrink-0" />}
                  <Linked text={why} />
                </p>
              )}
              {open &&
                blockers.map((b, i) => (
                  <p key={b.task} className={`flex min-w-0 items-center gap-2 text-sm ${i > 0 ? 'pl-6' : ''}`}>
                    {i === 0 && <Hourglass size={14} className="shrink-0 text-muted" />}
                    <span className="shrink-0 whitespace-nowrap text-muted">Waiting on</span>
                    <TaskLink id={b.task} title={b.title} state={b} />
                  </p>
                ))}
            </header>
            {t.body && (
              <Flash text={t.body}>
                <Markdown text={t.body} />
              </Flash>
            )}
            {t.subtasks.length > 0 && (
              <Section title={['Sub-tasks', subtaskCount(t.subtasks)].filter(Boolean).join(' · ')}>
                {t.subtasks.map((c) => (
                  <TaskLink key={c.id} id={c.id} title={c.title} state={c} />
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
              open={mine ? openTurnRow(t, me) : undefined}
              turn={turn}
              holder={t.to}
              onReply={(id) => setReplyTo(t.memos.find((m) => m.id === id) ?? null)}
              items={activityOf(t.memos, t.timeline, me, nameOf).filter((i) => !isSystem(i.who))}
            />
          </Page>
        </ScrollFade>
        <CommentBox
          t={t}
          reply={replyTo}
          typing={typing}
          onDrop={() => setReplyTo(null)}
          attach={attach}
          draft={draft}
          setDraft={setDraft}
          onMarked={addMarked}
          onSent={() => toBottom(id)}
        />
      </MarkReply.Provider>
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

/** A task by its number and title, its state pill first when given; a click opens it. */
function TaskLink({ id, title, state }: { id: number; title?: string; state?: Lookable }) {
  return (
    <button type="button" {...useOpenable(taskOf(id))} className="min-w-0 truncate rounded-sm text-left hover:bg-hover/30">
      {state && <TaskState t={state} />}
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
          <MenuItem icon={<Check size={14} />} onClick={() => done.mutate({ task: id }, { onError: sayRefusal })}>
            Done
          </MenuItem>
        )}
        {state === 'done' && (
          <MenuItem icon={<RotateCcw size={14} />} onClick={() => reopen.mutate({ task: id }, { onError: failed('Reopen') })}>
            Reopen
          </MenuItem>
        )}
        {open && (
          <MenuItem danger icon={<Trash2 size={14} />} onClick={() => discard.mutate({ task: id }, { onError: sayRefusal })}>
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

/** A line's key: a comment by its number and words, so an edit draws it again; an event by its time and words. */
const keyOf = (i: ActivityItem) => (i.kind === 'comment' ? `c${i.memo}:${i.note}` : `e${i.at}${i.line}`);

/** Comments and changes in one list, oldest first, all shown; the page opens at the newest. */
function Activity({
  task,
  items,
  open,
  turn,
  onReply,
  holder,
}: {
  holder?: string;
  task: number;
  items: ActivityItem[];
  open?: OpenTurn;
  turn?: Turn;
  onReply: (memo: number) => void;
}) {
  // The texts an edited comment had: its history rows. Its 'edited' mark opens them in a window.
  const rows = useTaskHistory(task).data?.rows;
  const earlierOf = (memo?: number | null) =>
    (rows ?? []).filter((r) => r.field === 'memo' && r.memo === memo && r.was && r.now).sort((a, b) => a.at.localeCompare(b.at));
  // What was there when the task opened just shows; a comment or event that comes later, or a comment's new words, flash.
  const [first] = useState(() => new Set(items.map(keyOf)));
  const fresh = (i: ActivityItem) => (first.has(keyOf(i)) ? '' : 'just-changed');
  const me = useMe();
  const isOpen = (i: ActivityItem) =>
    !!open && i.turn === 'push' && ('memo' in open ? i.memo === open.memo : i.at === open.at) && isPerson(i.turnTo ?? '', me);
  if (!items.length) return null;
  return (
    <Section title="Activity">
      <div className="flex flex-col">
        {items.map((i) =>
          i.kind === 'comment' ? (
            <CommentRow
              key={keyOf(i)}
              i={i}
              fresh={fresh(i)}
              turn={isOpen(i)}
              quoted={i.replyTo != null ? items.find((r) => r.memo === i.replyTo)?.note : undefined}
              onReply={onReply}
              holder={holder}
              onEdited={
                i.memo != null && earlierOf(i.memo).length
                  ? () =>
                      showHistory('Versions of this comment', [
                        { text: earlierOf(i.memo)[0]!.was, at: i.at, by: i.who },
                        ...earlierOf(i.memo).map((r) => ({ text: r.now, at: r.at, by: r.by })),
                      ])
                  : undefined
              }
            />
          ) : (
            <EventRow key={keyOf(i)} i={i} fresh={fresh(i)} turn={isOpen(i) ? turn : undefined} />
          ),
        )}
      </div>
    </Section>
  );
}

/** What the work came to: the pull request when the evidence names one, else the commit. */
function ResultChips({ evidence, title }: { evidence: string; title: string }) {
  const o = taskOutcome(evidence, title);
  const chip = o.pr !== null ? `PR #${o.pr}` : o.sha;
  if (!chip) return null;
  return <span className="mb-2 inline-block w-fit rounded-md bg-surface px-2 py-0.5 font-mono text-xs text-fg">{chip}</span>;
}

/** The task page drawn empty while it loads, in its own layout: the state and title, the line under it, the field rows, the body and the
 * comment box. */
function TaskLoading({ id }: { id: number }) {
  const bar = 'rounded-sm bg-border';
  return (
    <div aria-busy="true" aria-label={`Loading T${id}`} className="flex min-h-0 flex-1 animate-pulse flex-col motion-reduce:animate-none">
      <Page className="flex flex-1 flex-col gap-6 py-4">
        <header className="flex flex-col gap-3">
          <div className="flex h-7 items-center gap-2">
            <span className="h-6 w-16 rounded-md bg-border" />
            <span className="text-lg text-muted">T{id}</span>
            <span className={`h-5 w-1/3 ${bar}`} />
          </div>
          <span className={`h-3 w-44 ${bar}`} />
          {[28, 12, 8].map((w) => (
            <div key={w} className="flex h-6 items-center gap-2.5">
              <span className="size-4 rounded-sm bg-border" />
              <span className={`h-3 w-20 ${bar}`} />
              <span className={`ml-8 h-3 ${bar}`} style={{ width: `${w * 4}px` }} />
            </div>
          ))}
        </header>
        <div className="flex flex-col gap-2.5">
          {[90, 80, 45].map((w) => (
            <span key={w} className={`h-3 ${bar}`} style={{ width: `${w}%` }} />
          ))}
        </div>
      </Page>
      <Page className="pb-4">
        <div className="h-18 rounded-lg border border-border" />
      </Page>
    </div>
  );
}

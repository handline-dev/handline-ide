import { Ago } from '#src/ui/Ago.tsx';
import { Face, Name } from '#src/ui/Face.tsx';
// An Activity line's parts: its round mark, its hover row, a comment (the founder's open turn among them), and a change in words.
import type { ActivityItem } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import { addressText, isPerson, noteText, toldOf } from '@handline/app-core';
import { useMe } from '@handline/app-react';
import { useNameOf, useWrite } from '@handline/app-data';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { confirm } from '#src/ui/modal.tsx';
import { Check, CircleDot, Hand, Pencil, Send, Trash2, Undo2, UserRound } from 'lucide-react';
import { createContext, useContext, useState, type ReactNode } from 'react';
import { Reference } from '#src/chat/ChatLine.tsx';
import { Actions, CopyButton } from '#src/chat/MessageActions.tsx';
import { MessageImages } from '#src/chat/MessageImages.tsx';
import { ReadBy } from '#src/tools/ReadBy.tsx';
import { showMemo } from '#src/tools/TaskTurn.ts';
import { clock } from '#src/format/time.ts';
import { showHistory } from '#src/tools/TaskDiff.tsx';
import { Markdown } from '#src/ui/Markdown.tsx';

export type Turn = NonNullable<TaskDetail['turns']>[number];

/** What a change was: a turn passed, or from its sentence assigned or handed, done, discarded, edited, sent back, a request, or another state. */
function eventIcon({ line, turn }: ActivityItem): typeof Check {
  if (turn) return Hand;
  if (/handed it|assigned/.test(line)) return UserRound;
  if (/marked it done/.test(line)) return Check;
  if (/discarded/.test(line)) return Trash2;
  if (/changed the brief/.test(line)) return Pencil;
  if (/sent it back/.test(line)) return Undo2;
  if (/sent a request|from the queue|went again/.test(line)) return Send;
  return CircleDot;
}

/** Under a line, as under a chat message: its buttons and time on hover; the row keeps its height hidden, as the space between lines. */
export function HoverRow({
  at,
  edited,
  onEdited,
  children,
}: {
  at: string;
  edited?: boolean;
  onEdited?: () => void;
  children?: ReactNode;
}) {
  return (
    <div className="mb-0.5 flex h-6 items-center gap-1.5 text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
      {children}
      <time dateTime={at} title={clock(at)}>
        <Ago at={at} />
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
export function Mark({ icon: Icon }: { icon: typeof Check }) {
  return (
    <span className="grid size-6 shrink-0 place-items-center rounded-full bg-surface text-muted">
      <Icon size={12} />
    </span>
  );
}

/** True on a story's sub-task that waits for the founder's verdict: its open turn reads Ready for you, its pictures small. */
export const ReadyTurn = createContext(false);

/** A change in words, and the text behind it. The founder's open turn (`turn`) sits on the warm ground marked Waiting for you, as an open comment does, and reads the question
 * as it now is, with its older texts behind "edited"; answered, it stays as a plain row. */
export function EventRow({ i, fresh, turn }: { i: ActivityItem; fresh: string; turn?: Turn }) {
  const ready = useContext(ReadyTurn);
  const word = ready ? 'Ready for you' : 'Waiting for you';
  const note = turn?.ask || i.note;
  const earlier = turn?.earlier ?? [];
  return (
    <div
      data-turn-open={turn ? '' : undefined}
      className={`group flex gap-2.5 text-xs text-muted ${turn ? '-mx-3 rounded-lg bg-warning/10 px-3 pt-2' : ''}`}
    >
      <Mark icon={eventIcon(i)} />
      <div className="flex min-w-0 flex-1 flex-col pt-0.5">
        <div className={`flex flex-col rounded-md ${fresh}`}>
          {turn ? (
            <span className="font-medium text-fg">
              <span className="font-normal text-muted">{i.line}</span> <span className="text-warning">{word}</span>
            </span>
          ) : (
            <span>{i.line}</span>
          )}
          {note && (
            <div className="mt-1 border-l-2 border-border pl-3 text-sm text-fg">
              <Markdown text={note} />
            </div>
          )}
        </div>
        <HoverRow
          at={i.at}
          edited={!!turn?.edited}
          onEdited={
            turn?.edited && earlier.length
              ? () =>
                  showHistory('Versions of the question', [
                    ...earlier.map((e) => ({ text: e.ask, at: e.at, by: turn.by })),
                    { text: turn.ask ?? '', at: turn.edited ?? '', by: turn.by },
                  ])
              : undefined
          }
        >
          {note && <CopyButton text={note} />}
        </HoverRow>
      </div>
    </div>
  );
}

/** The founder's own comment on a task can be edited, and deleted unless it passed a turn (the daemon's rule); null for any other. */
export const ownComment = (i: Pick<ActivityItem, 'memo' | 'turn'>, mine: boolean, task?: number) =>
  mine && task != null && i.memo != null ? { task, memo: i.memo, deletable: !i.turn } : null;

/** A comment as a chat line, headed by its writer and time; whom it went to is an @mention before the text. While the founder's turn
 * passed with it is open, it sits on a soft warm ground marked Waiting for you. */
export function CommentRow({
  i,
  fresh,
  turn,
  quoted,
  onReply,
  onEdited,
  task,
}: {
  /** The task it is on; with it, the founder's own comment can be edited and deleted. */
  task?: number;
  i: ActivityItem;
  fresh: string;
  turn: boolean;
  quoted?: string;
  onReply: (memo: number) => void;
  onEdited?: () => void;
}) {
  const me = useMe();
  const nameOf = useNameOf();
  const ready = useContext(ReadyTurn);
  const word = ready ? 'Ready for you' : 'Waiting for you';
  const mine = isPerson(i.who, me);
  const toYou = !mine && (i.toFounder || (!!i.turn && isPerson(i.turnTo ?? '', me)));
  // Whom the comment went to: whom the daemon told it to (its readers), never worked out here.
  const told = mine ? toldOf(i, me) : [];
  const to = toYou ? ['you'] : told;
  const [editing, setEditing] = useState(false);
  const edit = useWrite('task.comment.edit');
  const del = useWrite('task.comment.delete');
  const own = ownComment(i, mine, task);
  const remove = () =>
    own &&
    void confirm({
      title: 'Delete this comment?',
      message: told.length
        ? `${told.map(nameOf).join(', ')} ${told.length > 1 ? 'were' : 'was'} told; ${told.length > 1 ? 'they' : 'it'} will be told it was deleted.`
        : undefined,
      confirmText: 'Delete',
      icon: <Trash2 size={14} />,
      danger: true,
      run: () => del.mutateAsync({ task: own.task, memo: own.memo }),
    });
  const save = (text: string) => {
    setEditing(false);
    if (own && text.trim() && text !== i.note) edit.mutate({ task: own.task, memo: own.memo, text });
  };
  return (
    <div
      data-memo={i.memo}
      data-turn-open={turn ? '' : undefined}
      className={`group relative isolate flex gap-2.5 rounded-md pb-3 ring-warning ring-offset-4 ring-offset-bg transition-shadow duration-500 ${turn ? '-mx-3 mb-3 rounded-lg bg-warning/10 px-3 pt-2 pb-2' : ''}`}
    >
      {/* A new comment lights up over the same box a Waiting for you comment sits on: the face too, 12px out at the sides, 8px above
          and below. Behind the words, over a turn's own ground. */}
      {fresh && (
        <span
          aria-hidden
          className={`pointer-events-none absolute -z-10 rounded-lg ${fresh} ${turn ? 'inset-0' : '-inset-x-3 -top-2 bottom-1'}`}
        />
      )}
      <Face who={i.who} mine={mine} />
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex flex-col rounded-md">
          <span className="flex items-baseline gap-2 text-sm">
            <Name who={i.who} mine={mine} className="font-medium" />
            <time dateTime={i.at} title={clock(i.at)} className="text-xs text-muted">
              <Ago at={i.at} />
            </time>
            {i.edited &&
              (onEdited ? (
                <button type="button" onClick={onEdited} className="text-xs text-muted underline decoration-dotted hover:text-fg">
                  edited
                </button>
              ) : (
                <span className="text-xs text-muted">edited</span>
              ))}
            {turn && <span className="text-xs font-medium text-warning">{word}</span>}
          </span>
          {i.replyTo != null && <Reference text={quoted} onClick={() => showMemo(i.replyTo!)} />}
          {editing ? (
            <EditBox text={i.note} onSave={save} onCancel={() => setEditing(false)} />
          ) : (
            <div className="min-w-0">
              <Markdown
                text={
                  i.toFounder
                    ? noteText(i.note)
                    : told.length
                      ? addressText(
                          i.note,
                          told.flatMap((a) => [a, nameOf(a)]),
                        )
                      : i.note
                }
                lead={to.map((a) => (
                  <span key={a} className="mr-1.5 rounded bg-accent/10 px-1 text-accent">
                    @{a === 'you' ? 'You' : nameOf(a)}
                  </span>
                ))}
              />
            </div>
          )}
          {i.images && i.images.length > 0 && (
            <div className="mt-2">
              <MessageImages images={i.images} small={turn && ready} />
            </div>
          )}
          <ReadBy readers={i.readers} />
        </div>
        <div
          className={`absolute ${turn ? 'top-1.5 right-3' : 'top-0 right-0'} flex items-center gap-1.5 text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100`}
        >
          <Actions text={i.note} onQuote={() => i.memo != null && onReply(i.memo)} />
          {own && (
            <IconButton className="size-6!" label="Edit" onClick={() => setEditing(true)}>
              <Pencil size={14} />
            </IconButton>
          )}
          {own?.deletable && (
            <IconButton className="size-6!" label="Delete" onClick={remove}>
              <Trash2 size={14} />
            </IconButton>
          )}
        </div>
      </div>
    </div>
  );
}

/** A comment's text, edited in place: Enter saves, Shift+Enter breaks the line, Esc cancels. */
function EditBox({ text, onSave, onCancel }: { text: string; onSave: (text: string) => void; onCancel: () => void }) {
  const [draft, setDraft] = useState(text);
  // The buttons keep the textarea's focus on press, so its blur does not cancel first.
  const keep = (e: React.MouseEvent) => e.preventDefault();
  return (
    <div className="relative mt-1">
      <textarea
        autoFocus
        aria-label="Edit comment"
        value={draft}
        rows={Math.min(8, draft.split('\n').length + 1)}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={onCancel}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onCancel();
          else if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
            e.preventDefault();
            onSave(draft);
          }
        }}
        className="w-full resize-none rounded-md border border-border bg-bg px-2 pt-1 pb-10 text-sm outline-none focus:border-accent"
      />
      <div className="absolute right-2 bottom-2.5 flex gap-2">
        <Button size="sm" onMouseDown={keep} onClick={onCancel}>
          Cancel
        </Button>
        <Button size="sm" variant="primary" onMouseDown={keep} onClick={() => onSave(draft)}>
          Save
        </Button>
      </div>
    </div>
  );
}

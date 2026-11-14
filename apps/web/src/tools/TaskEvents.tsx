// An Activity line's parts: its round mark, its hover row, a comment (the founder's open turn among them), and a change in words.
import type { ActivityItem } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import { isPerson, noteText } from '@handline/app-core';
import { useMe } from '@handline/app-react';
import { useNameOf } from '@handline/app-data';
import { Check, CircleDot, Hand, Pencil, Send, Trash2, Undo2, UserRound } from 'lucide-react';
import type { ReactNode } from 'react';
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
export function Mark({ icon: Icon }: { icon: typeof Check }) {
  return (
    <span className="grid size-6 shrink-0 place-items-center rounded-full bg-surface text-muted">
      <Icon size={12} />
    </span>
  );
}

/** A change in words, and the text behind it. The founder's open turn (`turn`) sits on the warm ground marked Waiting for you, as an open comment does, and reads the question
 * as it now is, with its older texts behind "edited"; answered, it stays as a plain row. */
export function EventRow({ i, fresh, turn }: { i: ActivityItem; fresh: string; turn?: Turn }) {
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
              <span className="font-normal text-muted">{i.line}</span> <span className="text-warning">Waiting for you</span>
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

/** A writer's initial in a circle: the founder's blue, an agent's gray. */
function Avatar({ name, mine }: { name: string; mine: boolean }) {
  return (
    <span
      className={`grid size-6 shrink-0 place-items-center rounded-full text-xs font-semibold text-accent-fg ${mine ? 'bg-accent' : 'bg-muted'}`}
    >
      {(name[0] ?? '?').toUpperCase()}
    </span>
  );
}

/** A comment as a chat line, headed by its writer and time; whom it went to is an @mention before the text. While the founder's turn
 * passed with it is open, it sits on a soft warm ground marked Waiting for you. */
export function CommentRow({
  i,
  fresh,
  turn,
  quoted,
  onReply,
  onEdited,
  holder,
}: {
  holder?: string;
  i: ActivityItem;
  fresh: string;
  turn: boolean;
  quoted?: string;
  onReply: (memo: number) => void;
  onEdited?: () => void;
}) {
  const me = useMe();
  const nameOf = useNameOf();
  const mine = isPerson(i.who, me);
  const toYou = !mine && (i.toFounder || (!!i.turn && isPerson(i.turnTo ?? '', me)));
  // Whom the comment went to: the turn passed with it, or the task's holder.
  const to = toYou ? 'you' : mine ? i.turnTo || holder : undefined;
  const name = mine ? 'You' : nameOf(i.who);
  return (
    <div
      data-memo={i.memo}
      data-turn-open={turn ? '' : undefined}
      className={`group relative flex gap-2.5 rounded-md pb-3 ring-warning ring-offset-4 ring-offset-bg transition-shadow duration-500 ${turn ? '-mx-3 mb-3 rounded-lg bg-warning/10 px-3 pt-2 pb-2' : ''}`}
    >
      <Avatar name={name} mine={mine} />
      <div className="flex min-w-0 flex-1 flex-col">
        <div className={`flex flex-col rounded-md ${fresh}`}>
          <span className="flex items-baseline gap-2 text-sm">
            <span className="font-medium">{name}</span>
            <time dateTime={i.at} title={i.at} className="text-xs text-muted">
              {clock(i.at)}
            </time>
            {i.edited &&
              (onEdited ? (
                <button type="button" onClick={onEdited} className="text-xs text-muted underline decoration-dotted hover:text-fg">
                  edited
                </button>
              ) : (
                <span className="text-xs text-muted">edited</span>
              ))}
            {turn && <span className="text-xs font-medium text-warning">Waiting for you</span>}
          </span>
          {i.replyTo != null && <Reference text={quoted} onClick={() => showMemo(i.replyTo!)} />}
          <div className="min-w-0">
            {to && <span className="float-left mr-1.5 rounded bg-accent/10 px-1 text-sm text-accent">@{to === 'you' ? 'You' : nameOf(to)}</span>}
            <Markdown text={i.toFounder ? noteText(i.note) : i.note} />
          </div>
          {i.images && <MessageImages images={i.images} />}
          <ReadBy readers={i.readers} />
        </div>
        {/* As in Slack: the buttons float at the comment's top right on hover, over it, so nothing moves. */}
        <div className={`absolute ${turn ? 'top-1.5 right-3' : 'top-0 right-0'} flex items-center gap-1.5 text-xs text-muted opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100`}>
          <Actions text={i.note} onQuote={() => i.memo != null && onReply(i.memo)} />
        </div>
      </div>
    </div>
  );
}

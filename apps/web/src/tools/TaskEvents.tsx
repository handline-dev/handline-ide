// An Activity line's parts: its round mark, its hover row, and a change in words (the founder's open turn among them).
import type { ActivityItem } from '@handline/app-core';
import type { TaskDetail } from '@handline/client';
import { Check, CircleDot, Hand, Pencil, Send, Trash2, Undo2, UserRound } from 'lucide-react';
import type { ReactNode } from 'react';
import { CopyButton } from '#src/chat/MessageActions.tsx';
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

/** A change in words, and the text behind it. The founder's open turn (`turn`) is outlined, headed Your turn, and reads the question
 * as it now is, with its older texts behind "edited"; answered, it stays as a plain row. */
export function EventRow({ i, fresh, turn }: { i: ActivityItem; fresh: string; turn?: Turn }) {
  const note = turn?.ask || i.note;
  const earlier = turn?.earlier ?? [];
  return (
    <div
      data-turn-open={turn ? '' : undefined}
      className={`group flex gap-2.5 text-xs text-muted ${turn ? '-mx-3 rounded-lg border border-warning px-3 pt-2' : ''}`}
    >
      <Mark icon={eventIcon(i)} />
      <div className="flex min-w-0 flex-1 flex-col pt-0.5">
        <div className={`flex flex-col rounded-md ${fresh}`}>
          {turn ? (
            <span className="font-medium text-fg">
              Your turn · <span className="font-normal text-muted">{i.line}</span>
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

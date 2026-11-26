// The lines of one file's diff, each with its number, and the comments a person writes on them: a + at a line's left (always there on a
// touch screen or a narrow window, on hover elsewhere) opens a one-line box under the line, Shift-click or a drag takes a range, and Enter
// keeps the comment as a draft under its line. Nothing is sent from here.
import { anchorOf, lineLabel, rowOf, rowRef, type DiffFile, type DiffRow, type LineComment } from '@handline/app-core';
import { Plus, X } from 'lucide-react';
import { useRef, useState, type PointerEvent as ReactPointerEvent } from 'react';
import type { LineDrafts } from '#src/changes/lineDrafts.ts';

/** Where a comment is being written: the rows `a` to `b` of the file, or the file as a whole. */
export type Picked = { a: number; b: number } | 'file';

/** The drafts of the place a diff belongs to, and which of its diffs is shown (LineComment.view, LineComment.commit). */
export interface Commenting {
  drafts: LineDrafts;
  view: string;
  commit?: string;
}

const TONE: Record<DiffRow['kind'], string> = {
  hunk: 'bg-surface text-muted',
  add: 'bg-success/10 text-fg',
  del: 'bg-danger/10 text-fg',
  ctx: 'text-fg',
  meta: 'text-muted',
};
const SIGN: Partial<Record<DiffRow['kind'], string>> = { add: '+', del: '-', ctx: ' ' };
const GUTTER = 'mr-3 inline-block w-12 bg-bg pr-2 text-right text-muted tabular-nums select-none';
/** A block among the lines that is not code: a comment, its box, a note. It is as wide as the scroll area, not as its longest line. */
const ACROSS = 'font-sans text-sm whitespace-normal';

/** The one-line box: Enter keeps the comment, Escape drops it. */
export function Box({
  text,
  setText,
  hint,
  onKeep,
  onDrop,
}: {
  text: string;
  setText: (t: string) => void;
  hint: string;
  onKeep: () => void;
  onDrop: () => void;
}) {
  return (
    <div className={ACROSS}>
      <div className="flex items-center gap-2 px-3 py-1.5">
        <input
          autoFocus
          value={text}
          placeholder={hint}
          aria-label={hint}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.nativeEvent.isComposing) onKeep();
            else if (e.key === 'Escape') onDrop();
            else return;
            e.preventDefault();
          }}
          className="h-7 min-w-0 flex-1 rounded-md border border-border bg-bg px-2 text-fg outline-none placeholder:text-muted focus:border-accent"
        />
        <button type="button" disabled={!text.trim()} onClick={onKeep} className="shrink-0 text-xs text-accent disabled:opacity-40">
          Add
        </button>
        <button type="button" onClick={onDrop} className="shrink-0 text-xs text-muted hover:text-fg">
          Cancel
        </button>
      </div>
    </div>
  );
}

/** A comment kept as a draft: its lines, its words (a click edits them), and a way to drop it. */
export function Draft({ c, drafts }: { c: LineComment; drafts: LineDrafts }) {
  const [edit, setEdit] = useState<string | null>(null);
  if (edit !== null)
    return (
      <Box
        text={edit}
        setText={setEdit}
        hint="Comment"
        onKeep={() => {
          if (edit.trim()) drafts.edit(c.id, edit.trim());
          setEdit(null);
        }}
        onDrop={() => setEdit(null)}
      />
    );
  return (
    <div className={ACROSS} data-draft={c.id}>
      <div className="mx-3 my-1 flex items-start gap-2 rounded-md border border-accent/40 bg-bg px-2 py-1">
        <span className="mt-0.5 shrink-0 rounded-sm bg-surface px-1 text-xs text-muted tabular-nums">{lineLabel(c)}</span>
        <button type="button" title="Edit" onClick={() => setEdit(c.text)} className="min-w-0 flex-1 text-left break-words text-fg">
          {c.text}
        </button>
        <button
          type="button"
          aria-label="Remove comment"
          title="Remove comment"
          onClick={() => drafts.remove(c.id)}
          className="mt-0.5 shrink-0 rounded-sm text-muted hover:bg-hover hover:text-fg"
        >
          <X size={14} />
        </button>
      </div>
    </div>
  );
}

/** A line's number; where lines can be commented on it is the button that starts one, with the + beside it. */
function Gutter({ row, at, onPick, onDown }: { row: DiffRow; at: number; onPick?: (i: number, more: boolean) => void; onDown?: Down }) {
  const no = row.kind === 'del' ? row.oldNo : row.newNo;
  if (!onPick || !rowRef(row)) return <span className={GUTTER}>{no ?? ''}</span>;
  return (
    <button
      type="button"
      aria-label={`Comment on line ${no}`}
      title="Comment on this line · Shift-click or drag for more lines"
      onClick={(e) => onPick(at, e.shiftKey)}
      onPointerDown={(e) => onDown?.(at, e)}
      className={`${GUTTER} relative cursor-pointer align-top outline-none hover:text-accent focus-visible:text-accent`}
    >
      <Plus
        size={12}
        strokeWidth={3}
        className="absolute top-1 left-1 text-accent opacity-0 group-hover/row:opacity-100 max-sm:opacity-70 pointer-coarse:opacity-70"
      />
      {no}
    </button>
  );
}

type Down = (i: number, e: ReactPointerEvent) => void;

/** The row a draft is drawn under in the diff shown; -1 when it is not on a line of this one (a comment on the file, one written on
 *  another of the place's diffs, a line that is gone), and it is listed above the rows. */
export const rowUnder = (file: DiffFile, lines: boolean, view: string, c: LineComment): number =>
  lines && c.path === file.path && c.view === view && c.at ? rowOf(file.rows, c.at) : -1;

/** The rows `a` to `b` are taken while the mouse, pressed on row `from`'s number, moves over other rows of `box`. `during` is told
 *  while it lasts: the box that opens under the last row waits for the release, or it would push the rows from under the mouse. */
function useDragRange(box: React.RefObject<HTMLElement | null>, take: (p: Picked) => void, during: (on: boolean) => void): Down {
  return (from, e) => {
    if (e.pointerType !== 'mouse' || e.button !== 0 || e.shiftKey) return;
    e.preventDefault(); // no text is selected by the drag; the click that ends it still comes
    let moved = false;
    const move = (m: PointerEvent) => {
      const el = document.elementFromPoint(m.clientX, m.clientY)?.closest('[data-row]');
      if (!el || !box.current?.contains(el)) return;
      const to = Number(el.getAttribute('data-row'));
      if (to === from && !moved) return;
      if (!moved) during(true);
      moved = true;
      take({ a: from, b: to });
    };
    const up = () => {
      removeEventListener('pointermove', move);
      removeEventListener('pointerup', up);
      during(false);
    };
    addEventListener('pointermove', move);
    addEventListener('pointerup', up);
  };
}

/** The file's rows. `lines`: its numbers hold, so each numbered line can take a comment; without it the rows are shown bare. */
export function Rows({
  file,
  lines,
  commenting,
  picked,
  setPicked,
  box,
}: {
  file: DiffFile;
  lines: boolean;
  commenting?: Commenting;
  picked: Picked | null;
  setPicked: (p: Picked | null) => void;
  /** The box a comment is typed in, drawn under the last picked row. */
  box: React.ReactNode;
}) {
  const area = useRef<HTMLDivElement>(null);
  const [dragging, setDragging] = useState(false);
  const onDown = useDragRange(area, setPicked, setDragging);
  const range = picked && picked !== 'file' ? [Math.min(picked.a, picked.b), Math.max(picked.a, picked.b)] : null;
  const onPick = (i: number, more: boolean) => setPicked(more && picked && picked !== 'file' ? { a: picked.a, b: i } : { a: i, b: i });
  const under = new Map<number, LineComment[]>();
  for (const c of commenting?.drafts.list ?? []) {
    const at = rowUnder(file, lines, commenting!.view, c);
    if (at >= 0) under.set(at, [...(under.get(at) ?? []), c]);
  }
  return (
    <div ref={area}>
      {file.rows.map((row, i) => (
        <div key={i}>
          {/* The line's text is the row's own, not an element of its own: a long line then scrolls sideways without a box wider than the window. */}
          <div
            data-row={i}
            className={`group/row pr-3 whitespace-pre ${lines ? '' : 'pl-3'} ${range && i >= range[0] && i <= range[1] && rowRef(row) ? 'bg-accent/20 text-fg' : TONE[row.kind]}`}
          >
            {lines && <Gutter row={row} at={i} onPick={commenting && onPick} onDown={commenting && onDown} />}
            {(SIGN[row.kind] ?? '') + row.text || ' '}
          </div>
          {under.get(i)?.map((c) => (
            <Draft key={c.id} c={c} drafts={commenting!.drafts} />
          ))}
          {range?.[1] === i && !dragging && box}
        </div>
      ))}
    </div>
  );
}

/** The words that say which lines a box is for: `Comment on line 42`, `Comment on lines 40-42`. */
export function boxHint(file: DiffFile, picked: Picked): string {
  if (picked === 'file') return 'Comment on this file';
  const label = lineLabel(anchorOf(file.rows, picked.a, picked.b) ?? {});
  return label === 'File' ? 'Comment' : `Comment on ${/^\d+$|^old \d+$/.test(label) ? 'line' : 'lines'} ${label}`;
}

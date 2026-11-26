// Comments a person writes on the lines of a diff: kept as drafts, then sent to the agent once, as one text. A task comment is told to
// its reader on one line and cut short, so the text opens with a line that is whole on its own and says where the rest is read. Pure.
import type { DiffRow } from '#src/diff-lines.ts';
import { foundersTurn, type Turns } from '#src/turns.ts';

/** A numbered line of a diff: an added or unchanged line by its number after the change, a deleted one by its number before it. */
export interface RowRef {
  side: 'new' | 'old';
  no: number;
}

export interface LineComment {
  id: string;
  path: string;
  /** The commit whose diff it was written on; none for all of a task's changes or a worktree's. */
  commit?: string;
  /** Which of the diffs a screen can show it was written on (a commit, all of a task's changes, a worktree against its head or its
   *  base): the same line has another number in another of them, so it is drawn under its line in this one only. */
  view: string;
  /** The lines it is on; none for a comment on the file as a whole. */
  lines?: { side: 'new' | 'old'; from: number; to: number };
  /** The line it is drawn under: the last of its lines. */
  at?: RowRef;
  /** The lines as they stood, each with its +, - or space. */
  quote: string[];
  text: string;
}

const SIGN = { add: '+', del: '-', ctx: ' ' } as const;
type Numbered = DiffRow & { kind: keyof typeof SIGN };
const numbered = (r: DiffRow): r is Numbered => r.kind in SIGN;

export function rowRef(r: DiffRow): RowRef | null {
  if (r.kind === 'del' && r.oldNo != null) return { side: 'old', no: r.oldNo };
  if ((r.kind === 'add' || r.kind === 'ctx') && r.newNo != null) return { side: 'new', no: r.newNo };
  return null;
}

/** Where the line `ref` names is among `rows`; -1 when the diff no longer has it. */
export const rowOf = (rows: readonly DiffRow[], ref: RowRef): number =>
  rows.findIndex((r) => {
    const own = rowRef(r);
    return own?.side === ref.side && own.no === ref.no;
  });

/** Lines kept with a comment; the text sent shows fewer (QUOTED). */
const KEPT = 40;

/** What a comment on the rows `a` to `b` (either order) points at: the lines by their numbers after the change, or before it when every
 *  one of them was deleted. Null when no row between them is a numbered line. */
export function anchorOf(rows: readonly DiffRow[], a: number, b: number): Pick<LineComment, 'lines' | 'at' | 'quote'> | null {
  const picked = rows.slice(Math.min(a, b), Math.max(a, b) + 1).filter(numbered);
  const at = picked.length ? rowRef(picked.at(-1)!) : null;
  if (!at) return null;
  const news = picked.flatMap((r) => (r.kind !== 'del' && r.newNo != null ? [r.newNo] : []));
  const olds = picked.flatMap((r) => (r.oldNo != null ? [r.oldNo] : []));
  const nos = news.length ? news : olds;
  return {
    lines: { side: news.length ? 'new' : 'old', from: nos[0], to: nos.at(-1)! },
    at,
    quote: picked.slice(0, KEPT).map((r) => SIGN[r.kind] + r.text),
  };
}

const span = (l: NonNullable<LineComment['lines']>) => (l.from === l.to ? `${l.from}` : `${l.from}-${l.to}`);

/** The lines a comment is on, as a short label beside it: `42`, `40-42`, `old 7`; `File` for the whole file. */
export const lineLabel = (c: Pick<LineComment, 'lines'>): string =>
  !c.lines ? 'File' : c.lines.side === 'old' ? `old ${span(c.lines)}` : span(c.lines);

const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`;
export const commentCount = (n: number) => plural(n, 'comment');

/** Lines of a quote the text shows, and how wide. */
const QUOTED = 6;
const WIDE = 160;
/** Paths the first line names before "and N more". */
const NAMED = 3;

const paths = (comments: readonly LineComment[]) => [...new Set(comments.map((c) => c.path))];

/** Text that is not the person's own words (a path, a quoted line) with no @name in it for the daemon to read as a mention: a comment
 *  that names an agent is told to that agent only, and `'@scope/pkg'` in a quoted import must not pick the reader. A zero-width space
 *  after the @ keeps the text looking the same. */
const unnamed = (s: string) => s.replace(/@(?=[A-Za-z0-9])/g, '@\u200b');

/** The first line of the text, whole on its own: how many comments, where to read them when the reader is told only this line, and
 *  on which files. The reading hint comes before the paths, so a line cut short loses paths and never the hint. */
export function lineCommentsSummary(comments: readonly LineComment[], readWith?: string): string {
  const what = plural(comments.length, comments.some((c) => c.lines) ? 'line comment' : 'file comment');
  const all = paths(comments);
  const named = unnamed(all.slice(0, NAMED).join(', ')) + (all.length > NAMED ? ` and ${all.length - NAMED} more` : '');
  return readWith ? `${what}, read ${comments.length === 1 ? 'it' : 'them'} with ${readWith}: ${named}` : `${what} on ${named}`;
}

/** The quoted lines in a fence no line of theirs can close. */
function fenced(quote: readonly string[]): string[] {
  if (!quote.length) return [];
  const shown = quote.slice(0, QUOTED).map((l) => unnamed(l.length > WIDE ? `${l.slice(0, WIDE)}…` : l));
  if (quote.length > QUOTED) shown.push(`… ${plural(quote.length - QUOTED, 'more line')}`);
  const ticks = Math.max(3, ...shown.map((l) => Math.max(0, ...[...l.matchAll(/`+/g)].map((m) => m[0].length)) + 1));
  return ['`'.repeat(ticks) + 'diff', ...shown, '`'.repeat(ticks)];
}

function block(c: LineComment): string {
  const path = unnamed(c.path);
  const where = c.lines ? `\`${path}:${span(c.lines)}\`` : `\`${path}\` (the whole file)`;
  const old = c.lines?.side === 'old' ? ' (deleted lines, numbered as before the change)' : '';
  const commit = c.commit ? ` in commit ${c.commit.slice(0, 7)}` : '';
  return [where + old + commit, ...fenced(c.quote), c.text.trim()].join('\n');
}

/** The comments in the order they are read: file by file as first written, a file's own comment first, then down its lines. */
function ordered(comments: readonly LineComment[]): LineComment[] {
  const file = paths(comments);
  const line = (c: LineComment) => c.lines?.from ?? 0;
  return [...comments].sort((x, y) => file.indexOf(x.path) - file.indexOf(y.path) || line(x) - line(y));
}

/** The one text the drafts are sent as: the summary line, `lead` when the person added words of their own, then a block per comment
 *  with its path and lines, the lines quoted, and the comment. */
export function lineCommentsText(comments: readonly LineComment[], opts: { readWith?: string; lead?: string } = {}): string {
  const lead = opts.lead?.trim();
  return [lineCommentsSummary(comments, opts.readWith), ...(lead ? [lead] : []), ...ordered(comments).map(block)].join('\n\n');
}

/** How a task's line comments go out, and what the button that sends them says. */
export interface CommentRoute {
  /** reopen: a done task is opened again with them; comment: they are a comment on an open task. */
  via: 'reopen' | 'comment';
  label: string;
  /** Set when the comment answers the person's own turn on the task: who asked, and gets the answer and the turn back. */
  answers?: string;
}

/** A done task is reopened, since a comment alone starts no work there. On an open task they are a comment; while the person's own
 *  turn is on top that comment is their answer and passes the turn, as the task page's Answer does, and the label says so. A task
 *  Ready for them keeps its turn: the comment is a question, and Redo stays its own button (redoLabel). */
export function taskCommentRoute(t: Turns & { review?: { ready: boolean } | null }, me: string, n: number): CommentRoute {
  if (t.state === 'done') return { via: 'reopen', label: `Reopen with ${commentCount(n)}` };
  if (foundersTurn(t, me) && !t.review?.ready)
    return { via: 'comment', label: `Answer with ${commentCount(n)}`, answers: t.turns?.at(-1)?.by || undefined };
  return { via: 'comment', label: 'Comment' };
}

export const redoLabel = (n: number) => `Redo with ${commentCount(n)}`;

// Comments a person writes on the lines of a diff: kept as drafts, then sent to the agent once, as one text. A task comment is told to
// its reader on one line and cut short, so the text opens with a line that is whole on its own and says where the rest is read. Pure.
import { lineNumbersHold, type DiffFile, type DiffRow } from '#src/diff-lines.ts';
import { foundersTurn, type Turns } from '#src/turns.ts';

/** A numbered line of a diff: an added or unchanged line by its number after the change, a deleted one by its number before it. */
export interface RowRef {
  side: 'new' | 'old';
  no: number;
}

/** The line a comment is drawn under, and how it read, with its sign, when the comment was written: a line of that number that reads
 *  otherwise is another line. */
export interface Anchor extends RowRef {
  line: string;
}

/** The name a place's drafts are kept under. `place` is a task or an agent (`task:12`, `agent:name`); `cluster` is the joined cluster
 *  it is on, '' for this Mac's own: two clusters each have a task 12, and may each have an agent of one name, and their drafts stay
 *  apart. */
export const draftsKey = (cluster: string, place: string): string => `linecomments/${cluster ? `@${cluster}/` : ''}${place}`;

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
  at?: Anchor;
  /** Set once the diff it was written on no longer has its line as it was (the line moved, changed or left the diff). From then on
   *  it is a comment on the file that carries the lines as they stood, and no line number is said for it again. */
  moved?: true;
  /** The lines as they stood, each with its +, - or space. */
  quote: string[];
  text: string;
}

const SIGN = { add: '+', del: '-', ctx: ' ' } as const;
type Numbered = DiffRow & { kind: keyof typeof SIGN };
const numbered = (r: DiffRow): r is Numbered => r.kind in SIGN;
const signed = (r: Numbered) => SIGN[r.kind] + r.text;

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

/** Where a comment's line is among `rows` as it read when the comment was written; -1 when no line of that number reads the same. */
export function placedRow(rows: readonly DiffRow[], at: Anchor): number {
  const i = rowOf(rows, at);
  const row = rows[i];
  return row && numbered(row) && signed(row) === at.line ? i : -1;
}

/** The drafts written on the diff `view` whose line that diff, as it is now, no longer has as written: its file left the diff, the
 *  file's numbers do not hold any more, or the line of that number reads otherwise. Their ids, to be marked moved. */
export function movedDrafts(drafts: readonly LineComment[], files: readonly DiffFile[], view: string): string[] {
  const by = new Map(files.map((f) => [f.path, f]));
  const gone = (c: LineComment) => {
    const f = by.get(c.path);
    return !f || !lineNumbersHold(f) || placedRow(f.rows, c.at!) < 0;
  };
  return drafts.filter((c) => c.view === view && c.at && !c.moved && gone(c)).map((c) => c.id);
}

/** Lines kept with a comment; the text sent shows fewer (QUOTED). */
const KEPT = 40;

/** What a comment on the rows `a` to `b` (either order) points at: the lines by their numbers after the change, or before it when every
 *  one of them was deleted. Null when no row between them is a numbered line. */
export function anchorOf(rows: readonly DiffRow[], a: number, b: number): Pick<LineComment, 'lines' | 'at' | 'quote'> | null {
  const picked = rows.slice(Math.min(a, b), Math.max(a, b) + 1).filter(numbered);
  const last = picked.at(-1);
  const ref = last && rowRef(last);
  if (!ref) return null;
  const news = picked.flatMap((r) => (r.kind !== 'del' && r.newNo != null ? [r.newNo] : []));
  const olds = picked.flatMap((r) => (r.oldNo != null ? [r.oldNo] : []));
  const nos = news.length ? news : olds;
  return {
    lines: { side: news.length ? 'new' : 'old', from: nos[0], to: nos.at(-1)! },
    at: { ...ref, line: signed(last) },
    quote: picked.slice(0, KEPT).map(signed),
  };
}

const span = (l: NonNullable<LineComment['lines']>) => (l.from === l.to ? `${l.from}` : `${l.from}-${l.to}`);

/** The lines a comment is on, as a short label beside it: `42`, `40-42`, `old 7`; `File` for the whole file; `was 42` once its
 *  line has moved. */
export function lineLabel(c: Pick<LineComment, 'lines' | 'moved'>): string {
  if (!c.lines) return 'File';
  const at = c.lines.side === 'old' ? `old ${span(c.lines)}` : span(c.lines);
  return c.moved ? `was ${at}` : at;
}

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

/** A path as the text says it: on one line and inside one code span whatever the file is called. A name may hold a line break or a
 *  backtick, and either would end the first line early or let the rest of the name out of its span. */
const said = (path: string) => unnamed(path.replace(/[\p{Cc}\u2028\u2029`]/gu, ' '));

/** The first line of the text, whole on its own: how many comments, where to read them when the reader is told only this line, and
 *  on which files. The reading hint comes before the paths, so a line cut short loses paths and never the hint. */
export function lineCommentsSummary(comments: readonly LineComment[], readWith?: string): string {
  const what = plural(comments.length, comments.some((c) => c.lines) ? 'line comment' : 'file comment');
  const all = paths(comments);
  const named = all.slice(0, NAMED).map(said).join(', ') + (all.length > NAMED ? ` and ${all.length - NAMED} more` : '');
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

/** Where a comment is, as its block's first line. One whose line has moved names the file only and says so: its number would point
 *  at another line now, and the lines quoted under it are how the agent finds the place. */
function where(c: LineComment): string {
  const path = said(c.path);
  const commit = c.commit ? ` in commit ${c.commit.slice(0, 7)}` : '';
  if (c.moved) return `\`${path}\`${commit} (on lines that have moved or changed since; they are quoted as they stood)`;
  if (!c.lines) return `\`${path}\` (the whole file)${commit}`;
  const old = c.lines.side === 'old' ? ' (deleted lines, numbered as before the change)' : '';
  return `\`${path}:${span(c.lines)}\`${old}${commit}`;
}

const block = (c: LineComment): string => [where(c), ...fenced(c.quote), c.text.trim()].join('\n');

/** The comments in the order they are read: file by file as first written, a file's own comment first, then down its lines. */
function ordered(comments: readonly LineComment[]): LineComment[] {
  const file = paths(comments);
  const line = (c: LineComment) => (c.moved ? 0 : (c.lines?.from ?? 0));
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

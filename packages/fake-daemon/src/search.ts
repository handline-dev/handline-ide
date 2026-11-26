// search for the fake daemon: tasks, comments and messages by text, shaped as the daemon's read answers them.
import type { SearchHit } from '@handline/contract';
import { invalidParams } from '#src/errors.ts';

const QUERY_MAX = 200;
const HITS_DEFAULT = 10;
const HITS_MAX = 50;
const SNIPPET_MAX = 160;
const SNIPPET_LEAD = 40;

/** A task as search reads it: its title, its brief, what its end kept and its comments. */
export type SearchTask = {
  id: number;
  title: string;
  created: string;
  body: string;
  evidence?: string | null;
  memos?: { id: number; author: string; at: string; text: string }[];
};
/** A message as search reads it, with the chat that shows it. */
export type SearchMessage = { id: number; at: string; from: string; body: string; agent: string; scope: 'mine' | 'all'; task?: number };

/** A to Z lowered and nothing else, as the daemon's LIKE compares. */
const fold = (s: string) => s.replace(/[A-Z]/g, (c) => c.toLowerCase());

/** One line of `text` around the first place it holds `query`, at most 160 characters, with … where it was cut. */
export function snippetOf(text: string, query: string): string {
  const line = [...text.replace(/\s+/g, ' ').trim()];
  if (line.length <= SNIPPET_MAX) return line.join('');
  const whole = line.join('');
  const i = fold(whole).indexOf(fold(query));
  const at = i <= 0 ? 0 : [...whole.slice(0, i)].length;
  const from = Math.max(0, at - SNIPPET_LEAD);
  if (line.length - from <= SNIPPET_MAX - 1)
    return `…${line
      .slice(line.length - (SNIPPET_MAX - 1))
      .join('')
      .trim()}`;
  const room = SNIPPET_MAX - 1 - (from > 0 ? 1 : 0);
  return `${from > 0 ? '…' : ''}${line
    .slice(from, from + room)
    .join('')
    .trim()}…`;
}

function read(p: Record<string, unknown>): { query: string; limit: number } {
  const query = typeof p.query === 'string' ? p.query.trim() : '';
  if (!query) throw invalidParams('query is text and not empty', 'query');
  if ([...query].length > QUERY_MAX) throw invalidParams(`query is at most ${QUERY_MAX} characters`, 'query');
  if (p.limit !== undefined && !(Number.isInteger(p.limit) && (p.limit as number) >= 1 && (p.limit as number) <= HITS_MAX))
    throw invalidParams(`limit is an integer from 1 to ${HITS_MAX}`, 'limit');
  return { query, limit: typeof p.limit === 'number' ? p.limit : HITS_DEFAULT };
}

/** The tasks, then the comments, then the messages that hold the query, each kind newest first and at most `limit`. */
export function search(tasks: SearchTask[], messages: SearchMessage[], p: Record<string, unknown>): { hits: SearchHit[]; more: boolean } {
  const { query, limit } = read(p);
  const holds = (text: string | null | undefined) => !!text && fold(text).includes(fold(query));
  const titles = new Map(tasks.map((t) => [t.id, t.title]));
  const found: SearchHit[][] = [
    tasks
      .filter((t) => holds(t.title) || holds(t.body) || holds(t.evidence))
      .sort((a, b) => b.id - a.id)
      .map((t) => {
        const field = holds(t.title) ? 'title' : holds(t.body) ? 'brief' : 'result';
        const text = field === 'title' ? t.title : field === 'brief' ? t.body : (t.evidence ?? '');
        return { kind: 'task', at: t.created, snippet: snippetOf(text, query), field, task: t.id, title: t.title };
      }),
    tasks
      .flatMap((t) => (t.memos ?? []).map((m) => ({ t, m })))
      .filter(({ m }) => holds(m.text))
      .sort((a, b) => b.m.id - a.m.id)
      .map(({ t, m }) => ({
        kind: 'comment',
        at: m.at,
        snippet: snippetOf(m.text, query),
        task: t.id,
        title: t.title,
        memo: m.id,
        from: m.author,
      })),
    messages
      .filter((m) => holds(m.body))
      .sort((a, b) => b.id - a.id)
      .map((m) => ({
        kind: 'message',
        at: m.at,
        snippet: snippetOf(m.body, query),
        request: m.id,
        agent: m.agent,
        scope: m.scope,
        from: m.from,
        ...(m.task !== undefined && titles.has(m.task) ? { task: m.task, title: titles.get(m.task) } : {}),
      })),
  ];
  return { hits: found.flatMap((kind) => kind.slice(0, limit)), more: found.some((kind) => kind.length > limit) };
}

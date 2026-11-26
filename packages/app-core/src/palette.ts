// The command palette's decisions: what the typed text asks for, which places match it, what is kept as recent. Pure.
import type { SearchHit } from '@handline/client';
import { isPersonKey } from '#src/chat.ts';
import { letterOf } from '#src/keys.ts';
import { isSystem } from '#src/task-people.ts';

type Chord = { code?: string; key: string; metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean };

/** Cmd-K, on a Mac only: elsewhere Ctrl-K is the terminal's and the editor's kill-line, so the palette opens from its button there. */
export const isPaletteChord = (e: Chord, mac: boolean): boolean =>
  mac && e.metaKey && !e.ctrlKey && !e.altKey && !e.shiftKey && letterOf(e) === 'k';

/** The task a typed number names: the number alone, or after a `T` or a `#`; null for anything else. */
export function taskNumberOf(text: string): number | null {
  const m = /^(?:t|#)?(\d{1,9})$/i.exec(text.trim());
  const n = m ? Number(m[1]) : 0;
  return n > 0 ? n : null;
}

/** How well a label answers the typed letters: 0 it starts with them, 1 one of its words does, 2 it holds them; null it does not. */
function rankOf(label: string, q: string): number | null {
  const l = label.toLowerCase();
  if (l.startsWith(q)) return 0;
  if (l.split(/[\s\-._/]+/).some((w) => w.startsWith(q))) return 1;
  return l.includes(q) ? 2 : null;
}

/** The places whose label (or one of its other words) answers the query, the best first, in the order given among equals; at most `max`. */
export function goMatches<T extends { label: string; words?: string }>(items: readonly T[], query: string, max = 6): T[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  return items
    .flatMap((item, i) => {
      const own = rankOf(item.label, q);
      const other = item.words ? rankOf(item.words, q) : null;
      // A match on the other words (a workspace's name beside its agent) counts after any on the label itself.
      const rank = own ?? (other === null ? null : other + 3);
      return rank === null ? [] : [{ item, rank, i }];
    })
    .sort((a, b) => a.rank - b.rank || a.i - b.i)
    .slice(0, max)
    .map((x) => x.item);
}

/** The recent list with `opens` put first, once, and cut to `max`. */
export const withRecent = (list: readonly string[], opens: string, max = 12): string[] =>
  [opens, ...list.filter((o) => o !== opens)].slice(0, max);

const GROUP_TITLE: Record<SearchHit['kind'], string> = { task: 'Tasks', comment: 'Comments', message: 'Messages' };

/** The hits under a heading per kind, in the order the daemon gave the kinds. */
export function hitGroups(hits: readonly SearchHit[]): { kind: SearchHit['kind']; title: string; hits: SearchHit[] }[] {
  const groups: { kind: SearchHit['kind']; title: string; hits: SearchHit[] }[] = [];
  for (const h of hits) {
    const g = groups.find((x) => x.kind === h.kind);
    if (g) g.hits.push(h);
    else groups.push({ kind: h.kind, title: GROUP_TITLE[h.kind], hits: [h] });
  }
  return groups;
}

/** A to Z lowered and nothing else, as the daemon's search compares. */
const fold = (s: string) => s.replace(/[A-Z]/g, (c) => c.toLowerCase());

/** A snippet cut where it holds the query, so the match can be drawn marked; one unmarked part when it does not hold it. */
export function snippetParts(snippet: string, query: string): { text: string; hit: boolean }[] {
  const q = fold(query.trim());
  const at = q ? fold(snippet).indexOf(q) : -1;
  if (at < 0) return [{ text: snippet, hit: false }];
  return [
    { text: snippet.slice(0, at), hit: false },
    { text: snippet.slice(at, at + q.length), hit: true },
    { text: snippet.slice(at + q.length), hit: false },
  ].filter((p) => p.text);
}

/** The agent whose chat draws a message hit: what an agent said to another agent is a line of the sender's chat (the receiver's
 * leaves it out); anything else, a message the daemon typed on a schedule among it, is in the chat the daemon names. */
export const chatOfHit = (hit: Pick<SearchHit, 'agent' | 'from'>): string | undefined =>
  hit.from && !isPersonKey(hit.from) && !isSystem(hit.from) && hit.agent && hit.agent !== hit.from ? hit.from : hit.agent;

/** The row an arrow, Home or End moves to from `at` among `count`, round the ends; `at` when the key is none of them. */
export function stepRow(at: number, count: number, key: string): number {
  if (count < 1) return 0;
  if (key === 'ArrowDown') return (at + 1) % count;
  if (key === 'ArrowUp') return (at - 1 + count) % count;
  if (key === 'Home') return 0;
  if (key === 'End') return count - 1;
  return at;
}

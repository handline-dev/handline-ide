// The Activity of a task as one list: its comments, and its events in words (a handoff is one line, its note opens on "Read note").
// Comments come from the memos, which a write patches at once; the other entries from the daemon's timeline.
import type { TaskDetail } from '@handline/client';
import { isPerson } from '#src/chat.ts';
import { findImageRefs } from '#src/images.ts';

type Entry = TaskDetail['timeline'][number];

export interface ActivityItem {
  kind: 'comment' | 'event';
  at: string;
  who: string;
  /** What happened, as a sentence; empty for a comment. */
  line: string;
  /** The text behind an event (a handoff's note), shown on request. */
  note: string;
  memo: number | null;
  /** The comment this one answers, for a reply. */
  replyTo?: number | null;
  /** For a founder's comment: where its tell stands with each agent it was told to. */
  readers?: TaskDetail['memos'][number]['readers'];
  /** A comment's pictures, still kept; on a video's poster, `video` names the video it plays. */
  images?: { id: string; name: string; mime: string; size: number; video?: { id: string; name: string } }[];
  edited: boolean;
  /** For a turn passed: handed on (push) or given back (pop), and to whom; on a comment, the turn passed with it. */
  turn?: 'push' | 'pop';
  turnTo?: string;
  /** For a comment that opens with a person: that person, as the daemon read it (Memo.to); a word to them that passes no turn. */
  to?: string;
}

const STATE_WORD: Record<string, string> = {
  opened: 'opened it',
  doing: 'started it',
  done: 'marked it done',
  discarded: 'discarded it',
  queued: 'queued it',
  draft: 'moved it back to draft',
  review: 'reported it',
  blocked: 'blocked it',
};

/** "handline.pc marked it done" from a state entry whose text is `from -> to`. */
const stateLine = (who: string, text: string): string => {
  const to = text.split('->').pop()?.trim() ?? '';
  return `${who} ${STATE_WORD[to] ?? `moved it to ${to || 'another state'}`}`;
};

/** Writes a key out (useNameOf); the identity when none is given. */
type NameOf = (key: string) => string;
const asIs: NameOf = (k) => k;

/** The person as "you", anyone else by name. */
const named = (who: string, me: string, nameOf: NameOf, you = 'you') => (isPerson(who, me) ? you : nameOf(who));

/** A turn passed, in words: "handline.ui asked you" with its ask, "You gave the turn back to handline.ui". */
function turnLine(e: Entry, me: string, nameOf: NameOf): { line: string; to: string } {
  const [from = e.who, to = ''] = e.who.split('→').map((x) => x.trim());
  if (e.turn === 'pop') return { line: `${named(from, me, nameOf, 'You')} gave the turn back to ${named(to, me, nameOf)}`, to };
  return { line: isPerson(to, me) ? `${nameOf(from)} asked you` : `${named(from, me, nameOf, 'You')} gave the turn to ${nameOf(to)}`, to };
}

/** A request in words: one the daemon delivered on someone's behalf says why (the queue reached it, or a new holder). */
function requestLine(e: Entry, nameOf: NameOf): string {
  const to = nameOf(e.who.split('→').pop()?.trim() ?? '');
  if (e.forwards === 'queue') return `It reached ${to} from the queue`;
  if (e.forwards === 'resend') return `It went again to ${to}, its new holder`;
  return `${e.who
    .split('→')
    .map((x) => nameOf(x.trim()))
    .join(' → ')} sent a request`;
}

/** A comment's words without a leading @mention of any of `to` (id or name), which the app draws as a chip. */
export const addressText = (text: string, to: readonly string[]): string => {
  // A picked mention is stored as <@key>, a typed one as @name.
  const m = /^\s*(?:<@([\w.-]+)>|@([\w.-]+))[:,]?\s*/.exec(text);
  const who = m?.[1] ?? m?.[2];
  return m && who && to.some((n) => n.toLowerCase() === who.toLowerCase()) ? text.slice(m[0].length) : text;
};
/** Whom the founder's own comment went to: the agents the daemon told it to (its readers), never the founder. */
export const toldOf = (i: Pick<ActivityItem, 'readers'>, me: string): string[] =>
  (i.readers ?? []).map((r) => r.agent).filter((a) => !isPerson(a, me));

/** The memos as comments and the rest of the timeline as events in words, oldest first; `me` is the person, written "you", and every
 * other who in a line is written out by `nameOf`. */
export function activityOf(memos: TaskDetail['memos'], timeline: readonly Entry[], me = '', nameOf: NameOf = asIs): ActivityItem[] {
  // A turn passed or given back with a comment is drawn as that comment, headed with whom it went to.
  const passed = new Map(
    timeline.flatMap((e) =>
      e.kind === 'turn' && e.memo != null ? [[e.memo, { to: turnLine(e, me, nameOf).to, turn: e.turn }] as const] : [],
    ),
  );
  const passedWith = (id: number): Pick<ActivityItem, 'turn' | 'turnTo'> => {
    const p = passed.get(id);
    return p ? { turn: p.turn === 'pop' ? 'pop' : 'push', turnTo: p.to } : {};
  };
  const comments: ActivityItem[] = memos.map((m) => {
    const named = new Set(findImageRefs(m.text).map((r) => r.id));
    return {
      kind: 'comment',
      at: m.at,
      who: m.author,
      line: '',
      note: m.text,
      memo: m.id,
      replyTo: m.reply_to ?? null,
      readers: m.readers,
      // A file the text names is drawn there, in the text: listed again under it, it would show twice. A video is drawn as its poster.
      images: (m.files ?? [])
        .filter((f) => f.kind !== 'video' && !f.gone && !named.has(f.id))
        .map((f) => {
          const v = m.files?.find((x) => x.poster === f.id);
          return { id: f.id, name: f.name, mime: f.mime, size: f.bytes, ...(v ? { video: { id: v.id, name: v.name } } : {}) };
        }),
      edited: !!m.edited_at,
      ...(m.to ? { to: m.to } : {}),
      ...passedWith(m.id),
    };
  });
  const shown = new Set(memos.map((m) => m.id));
  const events = timeline.flatMap((e): ActivityItem[] => {
    const base = { kind: 'event' as const, at: e.at, who: e.who, memo: e.memo, edited: false };
    switch (e.kind) {
      case 'memo':
        return [];
      case 'turn': {
        if (e.memo != null && shown.has(e.memo)) return [];
        // The person giving the turn back says nothing new: their comment is the answer.
        if (e.turn === 'pop' && isPerson(e.who.split('→')[0]?.trim() ?? '', me)) return [];
        const { line, to } = turnLine(e, me, nameOf);
        return [{ ...base, line, note: e.text, turn: e.turn === 'pop' ? 'pop' : 'push', turnTo: to }];
      }
      case 'handoff': {
        const [from = e.who, to = ''] = e.who.split('→').map((x) => x.trim());
        const by = named(from, me, nameOf, 'You');
        const line = !to ? `${by} handed it over` : isPerson(to, me) ? `${by} assigned you` : `${by} handed it to ${nameOf(to)}`;
        return [{ ...base, who: from, line, note: e.text }];
      }
      case 'state':
        return [{ ...base, line: stateLine(nameOf(e.who), e.text), note: '' }];
      case 'edit':
        return [{ ...base, line: `${nameOf(e.who)} changed the brief`, note: e.text }];
      case 'reject':
        return [{ ...base, line: `${nameOf(e.who)} sent it back`, note: e.text }];
      case 'request':
        return [{ ...base, line: requestLine(e, nameOf), note: e.text }];
      case 'lesson':
        return []; // drawn by T9200
    }
  });
  return [...comments, ...events].sort((a, b) => Date.parse(a.at) - Date.parse(b.at));
}

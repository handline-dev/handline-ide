// What the palette lists for the text typed: with none, what was on screen lately; a task number, that task first; then the places
// that answer it, then what search finds, grouped by kind with the words found.
import { chatOfHit, goMatches, hitGroups, SCHEDULE_SENDER, scheduledText, snippetParts, taskNumberOf } from '@handline/app-core';
import { useNameOf, useRead, useServes, useTask } from '@handline/app-data';
import type { SearchHit } from '@handline/client';
import { MessageSquareText } from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import { day } from '#src/format/time.ts';
import { iconOf, messageOf, opened, taskOf } from '#src/layout/kinds.tsx';
import { goPage } from '#src/layout/openTab.tsx';
import { tabEntry, usePages, usePlaces, type Entry } from '#src/palette/places.tsx';
import { useRecent } from '#src/palette/store.ts';
import { showComment } from '#src/tools/TaskTurn.ts';
import { useRefs } from '#src/ui/refs.tsx';

export type Section = { title: string; entries: Entry[]; /** Said under the rows: nothing found, more to find. */ note?: string };

const RECENT_SHOWN = 8;
const HITS = 8;
const DEBOUNCE_MS = 200;

/** `value` once it has stood for a moment, so a read follows the typing and not each key. */
function useSettled<T>(value: T, ms: number): T {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setSettled(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return settled;
}

/** One letter finds too much to be worth a read, unless it is a whole word of its own (a Korean or Chinese character). */
const worthSearching = (q: string) => q.length >= 2 || /[^\x20-\x7e]/.test(q);

/** A task's number and title; the title is read when it is not given. */
function TaskLabel({ id, title }: { id: number; title?: string }) {
  const read = useTask(id, { enabled: title === undefined });
  return (
    <>
      <span className="text-muted tabular-nums">T{id}</span> {title ?? read.data?.title ?? ''}
    </>
  );
}

/** The words found, the match marked. */
function Snippet({ text, query }: { text: string; query: string }) {
  return snippetParts(text, query).map((p, i) =>
    p.hit ? (
      <mark key={i} className="rounded-sm bg-selected text-fg">
        {p.text}
      </mark>
    ) : (
      p.text
    ),
  );
}

/** Brings a comment into view once its task's page has drawn it. */
function showCommentWhenDrawn(comment: number) {
  let tries = 0;
  const look = () => {
    if (document.querySelector(`[data-comment="${comment}"]`)) return showComment(comment);
    if (++tries < 40) setTimeout(look, 100);
  };
  setTimeout(look, 100);
}

const task = (id: number, title: string | undefined): Entry => ({
  key: taskOf(id),
  icon: iconOf('tasks'),
  label: <TaskLabel id={id} title={title} />,
  opens: taskOf(id),
  here: () => goPage(taskOf(id), `T${id}`),
});

/** A recent tab as a row; a task shows its title beside its number. */
function recentEntry(opens: string, nameOf: (key: string) => string): Entry {
  const id = Number(opened(opens, 'task') ?? 0);
  return id ? task(id, undefined) : tabEntry(opens, nameOf);
}

function useHitEntry(query: string): (hit: SearchHit, i: number) => Entry {
  const nameOf = useNameOf();
  const refs = useRefs();
  return (hit, i) => {
    // A message typed on a schedule reads as the chat draws it: under the word Scheduled, without the cron it was typed with.
    const scheduled = hit.from === SCHEDULE_SENDER;
    const note: ReactNode = <Snippet text={refs(scheduled ? scheduledText(hit.snippet) : hit.snippet)} query={query} />;
    const by = scheduled ? 'Scheduled' : hit.from ? nameOf(hit.from) : '';
    const who = by ? `${by} · ${day(hit.at)}` : day(hit.at);
    if (hit.kind === 'task')
      return {
        ...task(hit.task!, hit.title),
        key: `hit:task:${hit.task}`,
        hint: hit.field === 'brief' ? 'in its brief' : hit.field === 'result' ? 'in its result' : undefined,
        note: hit.field === 'title' ? undefined : note,
      };
    if (hit.kind === 'comment')
      return {
        ...task(hit.task!, hit.title),
        key: `hit:comment:${hit.comment}`,
        icon: <MessageSquareText size={14} />,
        hint: who,
        note,
        then: () => showCommentWhenDrawn(hit.comment!),
      };
    // A message that delivers a task is that task's card in the chat: it opens the task. Any other opens with the lines around it.
    const opens = hit.task ? taskOf(hit.task) : messageOf(hit.request!);
    const chat = chatOfHit(hit);
    return {
      key: `hit:message:${hit.request ?? i}`,
      icon: iconOf('chat'),
      label: hit.task ? <TaskLabel id={hit.task} title={hit.title} /> : `${chat ? nameOf(chat) : 'A'} chat`,
      hint: who,
      note,
      opens,
      here: () => goPage(opens, hit.task ? `T${hit.task}` : 'Message'),
    };
  };
}

/** The sections for `query`, and whether a search is still on its way. */
export function useSections(query: string): { sections: Section[]; searching: boolean } {
  const q = query.trim();
  const nameOf = useNameOf();
  const recent = useRecent();
  const pages = usePages();
  const places = usePlaces();
  const settled = useSettled(q, DEBOUNCE_MS);
  const served = useServes('search').served;
  const on = served && worthSearching(settled);
  const found = useRead('search', { query: settled, limit: HITS }, { enabled: on });
  const hitEntry = useHitEntry(settled);
  // A number that names no task says so in place of its row.
  const typed = taskNumberOf(q);
  const lookedUp = taskNumberOf(settled);
  const named = useTask(lookedUp ?? 0, { enabled: lookedUp !== null });

  if (!q) {
    const lately = recent.slice(0, RECENT_SHOWN).map((o) => recentEntry(o, nameOf));
    // A page listed as recent is not listed again under it.
    const rest = pages.filter((p) => !lately.some((l) => l.opens === p.opens));
    return {
      sections: [...(lately.length ? [{ title: 'Recent', entries: lately }] : []), { title: 'Go to', entries: rest }],
      searching: false,
    };
  }
  const sections: Section[] = [];
  if (typed !== null) {
    const known = typed === lookedUp;
    if (known && named.isError) sections.push({ title: 'Task', entries: [], note: `No task T${typed}.` });
    else sections.push({ title: 'Task', entries: [task(typed, known ? (named.data?.title ?? '') : '')] });
  }
  const go = goMatches(places, q);
  if (go.length) sections.push({ title: 'Go to', entries: go });
  // The hits shown are those of the text last read: while the next read is on its way they stay, under the same headings.
  const current = on && settled === q && !found.isPlaceholderData;
  const hits = on ? (found.data?.hits ?? []) : [];
  for (const g of hitGroups(hits)) sections.push({ title: g.title, entries: g.hits.map(hitEntry) });
  const searching = worthSearching(q) && served && (!current || found.isFetching);
  const said = (note: string) => sections.push({ title: 'Search', entries: [], note });
  // What is said of an empty search is said only when the list is otherwise empty: under a task or a place that answered, it would
  // read as that one's failure.
  const alone = !sections.length;
  if (!served) said('This daemon does not serve search yet: update Handline to find tasks, comments and messages.');
  else if (!worthSearching(q)) {
    if (alone) said('Type one more letter to search tasks, comments and messages.');
  } else if (current && found.isError) said('Search did not answer.');
  else if (current && found.data && !hits.length) {
    if (alone) said(`No task, comment or message holds “${q}”.`);
  } else if (current && found.data?.more) said('There are more. Type more of it to narrow them.');
  return { sections, searching };
}

// Settings > Agents, the founder's own rules as Handline learned them from what they wrote: Screens and Work, each rule with how many
// times it was said; resting on one shows the words it came from, a click opens that comment, and each can be removed.
import { useLessons, useWrite, why } from '@handline/app-data';
import type { Lesson } from '@handline/client';
import { Trash2 } from 'lucide-react';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { Block } from '#src/settings/Meter.tsx';
import { closeSettings } from '#src/settings/open.ts';
import { goToMemoOf } from '#src/tools/TaskTurn.ts';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { toast } from '#src/ui/toast.tsx';

const AREAS: { area: Lesson['area'] | undefined; title: string }[] = [
  { area: 'screen', title: 'Screens' },
  { area: 'work', title: 'Work' },
  { area: undefined, title: 'Other' },
];

export function Rules({ person }: { person: string }) {
  const items = useLessons(person).data?.items;
  if (!items) return null;
  return (
    <Block title={`Your preferences · ${items.length}`} note="What Handline learned from what you wrote. Every agent follows these.">
      {!items.length && (
        <p className="text-sm text-muted">None yet. When you tell an agent how you want something done, it is kept here.</p>
      )}
      {AREAS.map(({ area, title }) => {
        const rules = items.filter((l) => l.area === area).sort((a, b) => (b.count ?? 1) - (a.count ?? 1));
        if (!rules.length) return null;
        return (
          <section key={title} className="-mx-2 flex flex-col">
            <h3 className="sticky top-0 z-10 border-b border-border bg-bg px-2 py-1 text-xs font-medium text-muted">{title}</h3>
            <ul className="flex flex-col">
              {rules.map((l) => (
                <Rule key={l.id} l={l} />
              ))}
            </ul>
          </section>
        );
      })}
    </Block>
  );
}

/** One rule: its words and how many times; the quote on hover, its comment on a click by the app's one scheme. */
function Rule({ l }: { l: Lesson }) {
  const remove = useWrite('lesson.remove');
  const q = l.quote;
  const task = q?.task ?? l.task;
  const open = useOpenable(task != null ? taskOf(task) : null, {
    focus: () => task != null && q?.comment != null && goToMemoOf(task, q.comment),
    // Settings covers the app: it closes so the comment shows.
    onOpen: closeSettings,
  });
  const row = (
    <li className="group flex items-center gap-3 border-b border-border px-2 py-2 hover:bg-hover/30">
      <button type="button" {...open} disabled={task == null} className="flex min-w-0 flex-1 items-baseline gap-3 text-left">
        <span className="min-w-0 flex-1 text-sm text-fg">{l.text}</span>
        <span className="shrink-0 text-xs text-muted tabular-nums">{(l.count ?? 1) > 1 ? `said ${l.count} times` : 'said once'}</span>
      </button>
      <IconButton
        label="Remove this preference"
        className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
        disabled={remove.isPending}
        onClick={() =>
          remove.mutate({ id: l.id }, { onError: (e) => toast(`Did not remove: ${why(e) || 'try again'}`, { tone: 'warning' }) })
        }
      >
        <Trash2 size={14} />
      </IconButton>
    </li>
  );
  return q ? (
    <HoverCard trigger={row}>
      <p className="max-w-sm text-sm text-fg">“{q.text}”</p>
      {task != null && <p className="text-xs text-muted">T{task}</p>}
    </HoverCard>
  ) : (
    row
  );
}

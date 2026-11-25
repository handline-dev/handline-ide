// The founder's unread view: a button in the sidebar's top bar with how many agents wrote to them unread, which swaps the sidebar's
// sections for a flat list of those agents (NavPrefs.unread_only, kept by the daemon across the founder's devices); and a folded
// workspace's marks, so what waits inside shows without unfolding it.
import { byWorkspace, foldMarks, unreadAgents, unreadCount } from '@handline/app-core';
import { useAgents, useDirectory, useNameOf, useNav } from '@handline/app-data';
import type { AgentView } from '@handline/client';
import { MessageSquareDot } from 'lucide-react';
import { useEffect, useState } from 'react';
import { activityOf, holdLine, type Activity } from '#src/layout/agentList.ts';
import { chatOf } from '#src/layout/kinds.tsx';
import { Row } from '#src/sidebar/Row.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';

/** The unread count as the row's own pill; none at zero, the button staying where it is. */
function Pill({ n }: { n: number }) {
  return n > 0 ? <span className="shrink-0 rounded-full bg-accent px-1.5 text-xs font-medium text-accent-fg tabular-nums">{n}</span> : null;
}

/** At the right of the top bar: pressed while the unread view is on; a click turns it on or off. */
export function UnreadButton() {
  const { nav, change } = useNav();
  const n = unreadCount(useAgents().data?.agents ?? []);
  const on = nav.unread_only;
  return (
    <IconButton
      label={n ? `${n} ${n === 1 ? 'agent' : 'agents'} wrote to you` : 'Unread'}
      aria-pressed={on}
      onClick={() => change({ unread_only: !on })}
      className={`relative ${on ? 'bg-selected text-fg' : ''}`}
    >
      <MessageSquareDot size={16} />
      {/* On the icon's corner, so the bar keeps its width whatever the count. */}
      {n > 0 && (
        <span className="absolute -top-1 -right-1 min-w-4 rounded-full bg-accent px-1 text-xs font-medium text-accent-fg tabular-nums">
          {n}
        </span>
      )}
    </IconButton>
  );
}

/** The unread view: the agents that wrote to the founder, newest first, as workspace/agent; one read here stays until the view is
 * left. Esc, like a second click, goes back. */
export function UnreadList({ shown, onSelect }: { shown: string[]; onSelect: (name: string) => void }) {
  const agents = useAgents().data?.agents ?? [];
  const { change } = useNav();
  const nameOf = useNameOf();
  const names = new Map(byWorkspace(useDirectory(), agents).flatMap((g) => g.agents.map((a) => [a.key, g.workspace?.name] as const)));
  // Every agent shown since the view opened, so reading one does not take its row away.
  const [kept, setKept] = useState<ReadonlySet<string>>(() => new Set());
  const list = unreadAgents(agents, kept);
  // Set while rendering (it adjusts state, no effect): a row once listed is kept.
  const fresh = list.filter((a) => !kept.has(a.key));
  if (fresh.length) setKept(new Set([...kept, ...fresh.map((a) => a.key)]));
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      // An Esc another thing took (a menu, a popover closing) is not for this view.
      if (e.defaultPrevented || e.key !== 'Escape' || t?.closest('input, textarea, [contenteditable], [role=dialog]')) return;
      change({ unread_only: false });
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [change]);
  if (!list.length) return <p className="px-2 py-1 text-sm text-muted">No unread messages</p>;
  return (
    <div className="flex flex-col gap-0.5">
      {list.map((a) => (
        <Row
          key={a.key}
          opens={chatOf(a.key)}
          title={names.get(a.key) ? `${names.get(a.key)}/${nameOf(a.key)}` : nameOf(a.key)}
          icon={<ActivityIcon act={activityOf(a)} />}
          selected={shown.includes(a.key)}
          onShow={() => onSelect(a.key)}
          note={holdLine(a) ?? noteOf(a)}
          unread={a.founder_unread ?? 0}
        />
      ))}
    </div>
  );
}

const noteOf = (a: AgentView) => a.last_to_founder?.text.replace(/\s+/g, ' ').trim();

const WORD: Record<'you' | 'error' | 'working', string> = { you: 'need you', error: 'need attention', working: 'working' };

/** A folded workspace's marks at the right of its header: needs you, error, working, each with how many, then the unread pill. */
export function FoldMarks({ agents }: { agents: AgentView[] }) {
  const marks = foldMarks(agents.map((a) => activityOf(a) as Activity));
  const unread = unreadCount(agents);
  if (!marks.length && !unread) return null;
  return (
    <span className="ml-auto flex shrink-0 items-center gap-2 text-xs text-muted tabular-nums">
      {marks.map((m) => (
        <span key={m.act} title={`${m.n} ${WORD[m.act]}`} className="flex items-center gap-0.5">
          <ActivityIcon act={m.act} />
          {m.n}
        </span>
      ))}
      <Pill n={unread} />
    </span>
  );
}

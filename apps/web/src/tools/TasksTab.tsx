import type { Task } from '@handline/client';
import { foundersTurn, turnChain } from '@handline/app-core';
import { classify, sentence, useGiveBack, useTasks } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { useState } from 'react';
import { ago } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { openProps, useOpenTab } from '#src/layout/openTab.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { failedLine, look } from '#src/ui/taskLook.tsx';
import { toast } from '#src/ui/toast.tsx';

type Show = 'open' | 'draft' | 'closed';

/** Open work first by what it is doing: doing, then held, then queued; each group newest first. */
const GROUPS: { title: string; has: (t: Task) => boolean }[] = [
  { title: 'Doing', has: (t) => t.state === 'doing' && !t.waiting && !t.block_reason && !t.hold_reason },
  { title: 'Waiting', has: (t) => !!(t.waiting || t.block_reason || t.hold_reason) },
  { title: 'Up next', has: () => true },
];

/** The tasks: open ones grouped by what they are doing, drafts, or the closed ones; in an agent's pane, that agent's by default.
 * A row opens the task in this pane, Cmd+click beside. */
export function TasksTab({ agent }: { agent: string | null }) {
  const [show, setShow] = useState<Show>('open');
  const [only, setOnly] = useState(true);
  const owner = agent && only ? agent : undefined;
  const q = useTasks(show === 'closed' ? { state: 'closed', order: 'closed', owner } : { state: show, owner });
  const items = q.data?.items ?? [];
  const groups =
    show === 'open'
      ? GROUPS.reduce<{ title: string; items: Task[] }[]>((out, g) => {
          const taken = new Set(out.flatMap((o) => o.items));
          out.push({ title: g.title, items: items.filter((t) => !taken.has(t) && g.has(t)) });
          return out;
        }, [])
      : [{ title: '', items }];
  return (
    <ScrollFade keep={`tasks:${agent ?? ''}`} className="min-h-0 flex-1">
      <Page>
        <div className="flex flex-col gap-5 py-4">
          <div className="flex flex-wrap items-center gap-2">
            {(['open', 'draft', 'closed'] as const).map((s) => (
              <Chip key={s} on={show === s} onClick={() => setShow(s)}>
                {s === 'open' ? 'Open' : s === 'draft' ? 'Drafts' : 'Closed'}
              </Chip>
            ))}
            {agent && (
              <Chip on={only} onClick={() => setOnly(!only)}>
                Assigned to {agent}
              </Chip>
            )}
          </div>
          {q.isPending && <p className="text-sm text-muted">Loading…</p>}
          {q.isError && <p className="text-sm text-danger">Tasks did not load.</p>}
          {q.data && !items.length && <p className="text-sm text-muted">No tasks here.</p>}
          {groups
            .filter((g) => g.items.length)
            .map((g) => (
              <section key={g.title} className="flex flex-col gap-1">
                {g.title && (
                  <h2 className="text-xs font-medium text-muted">
                    {g.title} {g.items.length}
                  </h2>
                )}
                <div className="flex flex-col overflow-hidden rounded-lg border border-border">
                  {g.items.map((t, i) => (
                    <Row key={t.id} t={t} ruled={i > 0} owner={owner} />
                  ))}
                </div>
              </section>
            ))}
        </div>
      </Page>
    </ScrollFade>
  );
}

function Chip({ on, onClick, children }: { on: boolean; onClick: () => void; children: string | string[] }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      className={`h-7 rounded-md px-2.5 text-sm ${on ? 'bg-selected text-fg' : 'text-muted hover:bg-hover/30'}`}
    >
      {children}
    </button>
  );
}

/** A task as the chat's task cards draw one: its state's square, number and title, and a line under it. */
/** Why it waits, in a few words: on the founder, on tasks by number, or the first clause of the daemon's sentence. */
function waitWords(t: Task): string | null {
  const w = t.waiting;
  if (w?.on === 'founder') return 'Waiting on you';
  if (w?.tasks.length) return `Waits on ${w.tasks.map((n) => `T${n}`).join(', ')}`;
  const text = w?.text ?? t.block_reason ?? t.hold_reason;
  return text ? text.split(/[,;(]/)[0]!.trim() : null;
}

function Row({ t, ruled, owner }: { t: Task; ruled: boolean; owner?: string }) {
  const openTab = useOpenTab();
  const me = useMe();
  const back = useGiveBack();
  const s = look(t);
  const why = waitWords(t);
  // The assignee is left out while the list is already that agent's.
  const chain = turnChain(t, me);
  const detail = [
    chain || (owner ? null : `Assigned to ${t.to}`),
    t.priority && `P${t.priority}`,
    why,
    failedLine(t.evidence, t.title),
    t.done ? `Closed ${ago(t.done)}` : `Created ${ago(t.created)}`,
  ];
  return (
    <div className={`flex items-center hover:bg-hover/30 ${ruled ? 'border-t border-border' : ''}`}>
      <button
        type="button"
        {...openProps(
          (how) => openTab(taskOf(t.id), how),
          () => taskOf(t.id),
        )}
        className="flex min-w-0 flex-1 gap-2.5 px-3 py-2 text-left text-sm"
      >
        <span className={`mt-0.5 grid size-5 shrink-0 place-items-center rounded-md text-accent-fg ${s.tone}`}>
          <s.icon size={12} strokeWidth={3} />
        </span>
        <div className="min-w-0">
          <div className="truncate">
            <span className="mr-1 text-muted">T{t.id}</span>
            <span className={`font-medium ${t.state === 'discarded' ? 'text-muted line-through' : 'text-fg'}`}>{t.title}</span>
          </div>
          <div className="truncate text-xs text-muted">{detail.filter(Boolean).join(' · ')}</div>
        </div>
      </button>
      {foundersTurn(t, me) && (
        <button
          type="button"
          disabled={back.pending}
          onClick={() =>
            back.give(t.id, { onError: (e) => toast(`Give back did not work: ${sentence(classify(e))}`, { tone: 'warning' }) })
          }
          className="mr-3 shrink-0 rounded-md px-2 py-1 text-sm font-medium hover:bg-hover/30 disabled:opacity-40"
        >
          Give back
        </button>
      )}
    </div>
  );
}

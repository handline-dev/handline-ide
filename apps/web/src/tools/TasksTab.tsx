import { ListLoading } from '#src/ui/Loading.tsx';
import { useLeaseWaits } from '#src/tools/leaseWait.ts';
import { LeaseIcon } from '#src/tools/LeaseIcon.tsx';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { Fragment, type ReactNode } from 'react';
import type { Task } from '@handline/client';
import { foundersTurn, turnChain } from '@handline/app-core';
import { classify, sentence, useGiveBack, useNameOf, useTasks } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { ago, useNow } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { Page } from '#src/ui/Page.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { failedLine, look } from '#src/ui/taskLook.tsx';
import { toast } from '#src/ui/toast.tsx';
import { useTabState } from '#src/persist/tabState.ts';

type Show = 'mine' | 'own' | 'draft' | 'closed';

const SHOWS: { key: Show; label: string }[] = [
  { key: 'mine', label: 'My requests' },
  { key: 'own', label: "Agents' own" },
  { key: 'draft', label: 'Drafts' },
  { key: 'closed', label: 'Closed' },
];

/** A request nothing has moved for this long is on hold: folded under the ones in progress. */
const HOLD_MS = 14 * 24 * 60 * 60 * 1000;
const onHold = (t: Task, now: number) => t.state !== 'doing' && !t.turns?.length && now - Date.parse(t.delivered ?? t.created) > HOLD_MS;

/** Open work first by what it is doing: doing, then held, then queued; each group newest first. */
const GROUPS: { title: string; has: (t: Task) => boolean }[] = [
  { title: 'Doing', has: (t) => t.state === 'doing' && !t.waiting && !t.block_reason && !t.hold_reason },
  { title: 'Waiting', has: (t) => !!(t.waiting || t.block_reason || t.hold_reason) },
  { title: 'Up next', has: () => true },
];

/** The tasks: open ones grouped by what they are doing, drafts, or the closed ones; in an agent's pane, that agent's by default.
 * A row opens the task in this pane. */
export function TasksTab({ agent }: { agent: string | null }) {
  const [kept, setShow] = useTabState<Show | 'open'>('show', 'mine');
  const show: Show = kept === 'open' ? 'mine' : kept;
  const [only, setOnly] = useTabState('only', true);
  const [held, setHeld] = useTabState('held', false);
  const owner = agent && only ? agent : undefined;
  const open = show === 'mine' || show === 'own';
  const q = useTasks(show === 'closed' ? { state: 'closed', order: 'closed', owner } : { state: open ? 'open' : show, owner });
  const all = q.data?.items ?? [];
  const items = open ? all.filter((t) => !!t.my_request === (show === 'mine')) : all;
  const mine = all.filter((t) => t.my_request).length;
  const now = useNow(60_000 * 10);
  const groups =
    show === 'mine'
      ? [{ title: 'In progress', items: items.filter((t) => !onHold(t, now)) }]
      : show === 'own'
        ? GROUPS.reduce<{ title: string; items: Task[] }[]>((out, g) => {
            const taken = new Set(out.flatMap((o) => o.items));
            out.push({ title: g.title, items: items.filter((t) => !taken.has(t) && g.has(t)) });
            return out;
          }, [])
        : [{ title: '', items }];
  const hold = show === 'mine' ? items.filter((t) => onHold(t, now)) : [];
  return (
    <ScrollFade keep={`tasks:${agent ?? ''}`} className="min-h-0 flex-1">
      <Page>
        <div className="flex flex-col gap-5 py-4">
          <div className="flex flex-wrap items-center gap-2">
            {SHOWS.map((x) => (
              <Chip key={x.key} on={show === x.key} onClick={() => setShow(x.key)}>
                {x.label}
                {x.key === 'mine' && q.data ? <span className="ml-1.5 text-accent">{mine}</span> : null}
              </Chip>
            ))}
            {agent && (
              <Chip on={only} onClick={() => setOnly(!only)}>
                Assigned to {agent}
              </Chip>
            )}
          </div>
          {q.isPending && <ListLoading label="Loading tasks" />}
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
                    <Row
                      key={t.id}
                      t={t}
                      ruled={i > 0}
                      owner={owner}
                      dot={show === 'mine'}
                      kids={all.filter((c) => c.parent === t.id).map((c) => c.id)}
                    />
                  ))}
                </div>
              </section>
            ))}
          {hold.length > 0 && (
            <section className="flex flex-col gap-1">
              <button
                type="button"
                aria-expanded={held}
                onClick={() => setHeld(!held)}
                className="flex w-fit items-center gap-1 rounded-sm text-xs font-medium text-muted hover:bg-hover/30"
              >
                {held ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                On hold {hold.length}
              </button>
              {held ? (
                <div className="flex flex-col overflow-hidden rounded-lg border border-border">
                  {hold.map((t, i) => (
                    <Row key={t.id} t={t} ruled={i > 0} owner={owner} dot kids={[]} />
                  ))}
                </div>
              ) : (
                <p className="truncate text-xs text-muted">{hold.map((t) => `T${t.id} ${t.title}`).join(' · ')} · nothing for two weeks</p>
              )}
            </section>
          )}
        </div>
      </Page>
    </ScrollFade>
  );
}

function Chip({ on, onClick, children }: { on: boolean; onClick: () => void; children: ReactNode }) {
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
/** Why it waits, in a few words: on the founder, on tasks by number, on its own open sub-tasks (`kids`, those in the list), or the
 * first clause of the daemon's sentence. */
function waitWords(t: Task, kids: number[]): string | null {
  const w = t.waiting;
  if (w?.on === 'founder') return 'Waiting on you';
  const others = w?.tasks.filter((n) => !kids.includes(n)) ?? [];
  if (others.length) return `Waits on ${others.map((n) => `T${n}`).join(', ')}`;
  if (w?.tasks.length) return `${w.tasks.length} sub-task${w.tasks.length > 1 ? 's' : ''} open`;
  const text = w?.text ?? t.block_reason ?? t.hold_reason;
  return text ? text.split(/[,;(]/)[0]!.trim() : null;
}

function Row({ t, ruled, owner, kids, dot }: { t: Task; ruled: boolean; owner?: string; kids: number[]; dot?: boolean }) {
  const nameOf = useNameOf();
  const me = useMe();
  const back = useGiveBack();
  const s = look(t);
  const lease = useLeaseWaits().get(t.id);
  const why: ReactNode = lease ? (
    <>
      <LeaseIcon kind={lease.kind} /> {lease.text}
    </>
  ) : (
    waitWords(t, kids)
  );
  const yours = foundersTurn(t, me);
  // The assignee is left out while the list is already that agent's.
  const chain = turnChain(t, me, nameOf);
  const ask = yours
    ? t.turns
        ?.at(-1)
        ?.ask?.split('\n')
        .find((l) => l.trim())
    : undefined;
  const detail = ask
    ? [`Waits on you: ${ask}`]
    : [
        chain || (owner ? null : `Assigned to ${nameOf(t.to)}`),
        t.priority && `P${t.priority}`,
        why,
        failedLine(t.evidence, t.title),
        t.done ? `Closed ${ago(t.done)}` : `Created ${ago(t.created)}`,
      ];
  return (
    <div className={`flex items-center hover:bg-hover/30 ${ruled ? 'border-t border-border' : ''}`}>
      <button type="button" {...useOpenable(taskOf(t.id))} className="flex min-w-0 flex-1 gap-2.5 px-3 py-2 text-left text-sm">
        {dot ? (
          // Orange while it waits on the founder, blue while an agent is on it.
          <span
            className={`mt-1.5 size-2 shrink-0 rounded-full ${yours ? 'bg-warning' : 'bg-accent'}`}
            aria-label={yours ? 'Waits on you' : 'In progress'}
          />
        ) : (
          <span className={`mt-0.5 grid size-5 shrink-0 place-items-center rounded-md text-accent-fg ${s.tone}`}>
            <s.icon size={12} strokeWidth={3} />
          </span>
        )}
        <div className="min-w-0">
          <div className="truncate">
            <span className="mr-1 text-muted">T{t.id}</span>
            <span className={`font-medium ${t.state === 'discarded' ? 'text-muted line-through' : 'text-fg'}`}>{t.title}</span>
          </div>
          <div className="truncate text-xs text-muted">
            {detail.filter(Boolean).map((d, i) => (
              <Fragment key={i}>
                {i > 0 && ' · '}
                {d}
              </Fragment>
            ))}
          </div>
        </div>
      </button>
      {yours && (
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

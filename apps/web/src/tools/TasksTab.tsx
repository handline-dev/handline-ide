import { count } from '#src/format/count.ts';
import { ListLoading } from '#src/ui/Loading.tsx';
import { useLeaseWaits } from '#src/tools/leaseWait.ts';
import { LeaseIcon } from '#src/tools/LeaseIcon.tsx';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { Fragment, type ReactNode } from 'react';
import type { Task } from '@handline/client';
import { foundersTurn, isPersonKey, turnChain } from '@handline/app-core';
import { useGiveBack, useNameOf, useTasks, why } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { ago, useNow } from '#src/format/time.ts';
import { taskOf } from '#src/layout/kinds.tsx';
import { Across, InCluster, type Of } from '#src/connection/clusters.tsx';
import { HereWord } from '#src/connection/ClusterWord.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { Page } from '#src/ui/Page.tsx';
import { useRefs } from '#src/ui/refs.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { failedLine, look } from '#src/ui/taskLook.tsx';
import { toast } from '#src/ui/toast.tsx';
import { useTabState } from '#src/persist/tabState.ts';

type Show = 'mine' | 'own' | 'holds' | 'draft' | 'closed';

const SHOWS: { key: Show; label: string }[] = [
  { key: 'mine', label: 'My requests' },
  { key: 'own', label: "Agents' own" },
  { key: 'draft', label: 'Drafts' },
  { key: 'closed', label: 'Closed' },
];
/** A tab naming its agent lists all it holds in one, the founder's requests among them, as the papers on its desk count them. */
const NAMED: typeof SHOWS = [{ key: 'holds', label: 'Open' }, ...SHOWS.slice(2)];

/** A request nothing has moved for this long is on hold: folded under the ones in progress. */
const HOLD_MS = 14 * 24 * 60 * 60 * 1000;
const onHold = (t: Task, now: number) => t.state !== 'doing' && !t.turns?.length && now - Date.parse(t.delivered ?? t.created) > HOLD_MS;

/** A person's turn is on top: the task waits on them, whatever its owner's state. */
const personsTurn = (t: Task) => !!t.turns?.length && isPersonKey(t.acting);

/** Open work first by what it is doing: doing, then held, then queued; each group newest first. */
const GROUPS: { title: string; has: (t: Task) => boolean }[] = [
  { title: 'Doing', has: (t) => t.state === 'doing' && !t.waiting && !t.block_reason && !t.hold_reason && !personsTurn(t) },
  { title: 'Waiting', has: (t) => !!(t.waiting || t.block_reason || t.hold_reason) || personsTurn(t) },
  { title: 'Up next', has: () => true },
];

/** The tasks: open ones grouped by what they are doing, drafts, or the closed ones; in an agent's pane, that agent's by default.
 * A row opens the task in this pane. */
/** `named`: the tab names its agent (one's desk in the town opened it), so it lists what that agent holds. */
export function TasksTab({ agent, named = false }: { agent: string | null; named?: boolean }) {
  const [kept, setShow] = useTabState<Show | 'open'>('show', named ? 'holds' : 'mine');
  const show: Show = kept === 'open' ? 'mine' : kept;
  const [only, setOnly] = useTabState('only', true);
  const [held, setHeld] = useTabState('held', false);
  const owner = agent && only ? agent : undefined;
  const open = show === 'mine' || show === 'own' || show === 'holds';
  const params = show === 'closed' ? { state: 'closed', order: 'closed' as const, owner } : { state: open ? 'open' : show, owner };
  const useListed = () => useTasks(params);
  // The Tasks page lists every cluster's; an agent's pane, its own cluster's.
  return (
    <Across hook={useListed} alone={!!agent} sig={(q) => JSON.stringify([q.isPending, q.isError, q.data])}>
      {(all) => (
        <Tasks
          agent={agent}
          named={named}
          all={all}
          show={show}
          setShow={setShow}
          only={only}
          setOnly={setOnly}
          held={held}
          setHeld={setHeld}
        />
      )}
    </Across>
  );
}

type Q = ReturnType<typeof useTasks>;
/** A task and the cluster it is from ('' for this one). */
type At = Task & { cluster: string };

function Tasks({
  agent,
  named,
  all: got,
  show,
  setShow,
  only,
  setOnly,
  held,
  setHeld,
}: {
  agent: string | null;
  named: boolean;
  all: Of<Q>[];
  show: Show;
  setShow: (s: Show) => void;
  only: boolean;
  setOnly: (b: boolean) => void;
  held: boolean;
  setHeld: (b: boolean) => void;
}) {
  const owner = agent && only ? agent : undefined;
  const open = show === 'mine' || show === 'own' || show === 'holds';
  const q = got.find((g) => g.cluster === '')?.data;
  const several = got.length > 1;
  const merged: At[] = got.flatMap((g) => (g.data.data?.items ?? []).map((t) => ({ ...t, cluster: g.cluster })));
  const all = several ? merged.sort((a, b) => (b.done ?? b.created).localeCompare(a.done ?? a.created)) : merged;
  const items = open && show !== 'holds' ? all.filter((t) => !!t.my_request === (show === 'mine')) : all;
  const mine = all.filter((t) => t.my_request).length;
  const now = useNow(60_000 * 10);
  const groups =
    show === 'mine'
      ? [{ title: 'In progress', items: items.filter((t) => !onHold(t, now)) }]
      : show === 'own' || show === 'holds'
        ? GROUPS.reduce<{ title: string; items: At[] }[]>((out, g) => {
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
            {(named ? NAMED : SHOWS).map((x) => (
              <Chip key={x.key} on={show === x.key} onClick={() => setShow(x.key)}>
                {x.label}
                {x.key === 'mine' && q?.data ? <span className="ml-1.5 text-accent">{mine}</span> : null}
              </Chip>
            ))}
            {agent && (
              <Chip on={only} onClick={() => setOnly(!only)}>
                Assigned to {agent}
              </Chip>
            )}
          </div>
          {(!q || q.isPending) && <ListLoading label="Loading tasks" />}
          {q?.isError && <p className="text-sm text-danger">Tasks did not load.</p>}
          {q?.data && !items.length && <p className="text-sm text-muted">No tasks here.</p>}
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
                    <InCluster key={`${t.cluster}/${t.id}`} id={t.cluster} dim>
                      <Row
                        t={t}
                        ruled={i > 0}
                        owner={owner}
                        word={several && <HereWord />}
                        kids={all.filter((c) => c.cluster === t.cluster && c.parent === t.id).map((c) => c.id)}
                      />
                    </InCluster>
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
                    <InCluster key={`${t.cluster}/${t.id}`} id={t.cluster} dim>
                      <Row t={t} ruled={i > 0} owner={owner} kids={[]} word={several && <HereWord />} />
                    </InCluster>
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
  if (w?.tasks.length) return `${count(w.tasks.length, 'sub-task')} open`;
  const text = w?.text ?? t.block_reason ?? t.hold_reason;
  return text ? text.split(/[,;(]/)[0].trim() : null;
}

function Row({ t, ruled, owner, kids, word }: { t: Task; ruled: boolean; owner?: string; kids: number[]; word?: ReactNode }) {
  const nameOf = useNameOf();
  const me = useMe();
  const back = useGiveBack();
  const s = look(t);
  const lease = useLeaseWaits().get(t.id);
  const refs = useRefs();
  const words = waitWords(t, kids);
  const held: ReactNode = lease ? (
    <>
      <LeaseIcon kind={lease.kind} /> {lease.text}
    </>
  ) : (
    words && refs(words)
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
        held,
        failedLine(t.evidence, t.title),
        t.done ? `Closed ${ago(t.done)}` : `Created ${ago(t.created)}`,
      ];
  return (
    <div className={`flex items-center hover:bg-hover/30 ${ruled ? 'border-t border-border' : ''}`}>
      <button type="button" {...useOpenable(taskOf(t.id))} className="flex min-w-0 flex-1 gap-2.5 px-3 py-2 text-left text-sm">
        <span className={`mt-0.5 grid size-5 shrink-0 place-items-center rounded-md text-accent-fg ${s.tone}`}>
          <s.icon size={12} strokeWidth={3} />
        </span>
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
      {word && <span className="mr-3 flex min-w-0 shrink items-center">{word}</span>}
      {yours && (
        <button
          type="button"
          disabled={back.pending}
          onClick={() => back.give(t.id, { onError: (e) => toast(`Give back did not work: ${why(e)}`, { tone: 'warning' }) })}
          className="mr-3 shrink-0 rounded-md px-2 py-1 text-sm font-medium hover:bg-hover/30 disabled:opacity-40"
        >
          Give back
        </button>
      )}
    </div>
  );
}

import { Ago } from '#src/ui/Ago.tsx';
// A story's sub-task opened from its node: back to the Plan, the next one for you, and its Plan, Code and Result with the founder's verdicts.
import type { TaskDetail } from '@handline/client';
import { Hooks, useNameOf, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { Check, ChevronLeft, ChevronRight } from 'lucide-react';
import type { ReactNode } from 'react';
import { TaskCode } from '#src/tools/TaskCode.tsx';
import { taskPlace, useLineDrafts } from '#src/changes/lineDrafts.ts';
import { useTabState } from '#src/persist/tabState.ts';
import { FaceName } from '#src/ui/Face.tsx';
import { startsAfter, wordOf, type Sub } from '#src/tools/storyWords.ts';
import { Button } from '#src/ui/Button.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { note, Result, VerdictButtons } from '#src/tools/Review.tsx';

type Tab = 'plan' | 'code' | 'result';
const CLOSED = ['done', 'discarded'];

export function SubTask({ sub, story, onPick }: { sub: Sub; story: TaskDetail; onPick: (id: number | null) => void }) {
  const t = Hooks.useTaskGet({ task: sub.id }).data;
  const me = useMe();
  const [tab, setTab] = useTabState<Tab | null>(`subtab:${sub.id}`, null);
  // Every hook before the early return: a render after the load must call the same ones.
  const code = !!Hooks.useTaskChanges({ task: sub.id }).data?.commits.length;
  const waiting = useLineDrafts(taskPlace(sub.id)).list.length > 0;
  if (!t) return <div className="flex-1" />;
  const open = new Set(story.subtasks.filter((x) => !CLOSED.includes(x.state)).map((x) => x.id));
  const word = wordOf(t, sub, me, open);
  // The next sub-task for you after this one, else the next one.
  const rest = story.subtasks.filter((x) => x.id !== sub.id);
  const after = [...rest.filter((x) => x.id > sub.id), ...rest.filter((x) => x.id < sub.id)];
  const next = after.find((x) => x.review?.ready || (x.state === 'draft' && x.plan_state === 'written')) ?? after[0];
  const first: Tab = t.review ? 'result' : 'plan';
  const shown: Tab = tab === 'code' && !code ? first : (tab ?? first);
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4 text-sm">
        <button
          type="button"
          onClick={() => onPick(null)}
          className="flex items-center gap-1 rounded-md px-2 py-1 text-muted hover:bg-hover/30 hover:text-fg"
        >
          <ChevronLeft size={14} />
          Plan
        </button>
        {next && (
          <button
            type="button"
            onClick={() => onPick(next.id)}
            className="flex min-w-0 items-center gap-1 rounded-md px-2 py-1 text-muted hover:bg-hover/30 hover:text-fg"
          >
            <span className="truncate">
              Next: T{next.id} {next.title}
            </span>
            <ChevronRight size={14} className="shrink-0" />
          </button>
        )}
      </div>
      <div className="flex shrink-0 flex-col gap-2 px-6 pt-5">
        <h2 className="text-lg font-semibold text-fg">{t.title}</h2>
        <p className="flex items-center gap-2 text-sm text-muted">
          <FaceName who={t.to} className="text-fg" />
          <span>· T{t.id}</span>
          <span className={word.kind === 'you' ? 'text-warning' : ''}>· {word.text}</span>
        </p>
        <div role="tablist" className="mt-2 flex gap-4 border-b border-border text-sm">
          {(
            [
              ['plan', 'Plan', true],
              ['code', 'Code', code],
              ['result', 'Result', !!t.review],
            ] as const
          ).map(([k, label, live]) => (
            <button
              key={k}
              type="button"
              role="tab"
              aria-selected={shown === k}
              disabled={!live}
              onClick={() => setTab(k)}
              className={`-mb-px border-b-2 py-2 disabled:opacity-40 ${shown === k ? 'border-accent font-medium text-fg' : 'border-transparent text-muted hover:text-fg'}`}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
      {shown === 'plan' && <PlanTab t={t} open={open} />}
      {shown === 'code' && (
        <ScrollFade keep={`subcode:${t.id}`} fadeTop={!waiting} className="min-h-0 flex-1">
          <TaskCode task={t.id} />
        </ScrollFade>
      )}
      {shown === 'result' && t.review && (
        <ScrollFade className="min-h-0 flex-1">
          <div className="px-6 py-5">
            <Result t={t} review={t.review} />
          </div>
        </ScrollFade>
      )}
      {t.review?.ready ? <Verdict t={t} /> : t.state === 'draft' && <DraftButtons t={t} />}
    </div>
  );
}

function Footer({ children }: { children: ReactNode }) {
  return <div className="flex shrink-0 items-center justify-end gap-2 border-t border-border px-6 py-3">{children}</div>;
}

/** A Ready task's buttons, on every tab: Redo with a note, or OK. */
function Verdict({ t }: { t: TaskDetail }) {
  return (
    <Footer>
      <VerdictButtons t={t} />
    </Footer>
  );
}

/** A draft's buttons, before Run: ask for a plan or its core logic, and Plan OK; `children` goes last (a draft's own page puts Run
 * there). */
export function DraftButtons({ t, children }: { t: TaskDetail; children?: ReactNode }) {
  const ask = useWrite('task.plan_ask');
  const core = useWrite('task.core_ask');
  const plan = t.plan;
  return (
    <Footer>
      {!plan && t.plan_state === 'none' && (
        <Button variant="primary" disabled={ask.isPending} onClick={() => ask.mutate({ task: t.id }, { onError: sayRefusal })}>
          Ask for a plan
        </Button>
      )}
      {plan && (
        <Button
          disabled={core.isPending || (!!t.core_ask && !t.core_ask.sent_at)}
          onClick={() =>
            void note({ title: 'Ask for core logic', hint: 'Which part, if any (optional)', go: 'Ask', required: false }).then(
              (n) => n !== undefined && core.mutate({ task: t.id, ...(n ? { note: n } : {}) }, { onError: sayRefusal }),
            )
          }
        >
          Ask for core logic
        </Button>
      )}
      <PlanOk t={t} />
      {children}
    </Footer>
  );
}

/** Plan OK, or, once given, the OK and a way to take it back. */
function PlanOk({ t }: { t: TaskDetail }) {
  const ok = useWrite('task.plan_ok');
  if (!t.plan) return null;
  if (t.plan.ok)
    return (
      <>
        <Button disabled={ok.isPending} onClick={() => ok.mutate({ task: t.id, ok: false }, { onError: sayRefusal })}>
          Undo Plan OK
        </Button>
        <span className="flex items-center gap-1 text-sm text-success">
          <Check size={14} />
          Plan OK
        </span>
      </>
    );
  return (
    <Button
      variant="primary"
      disabled={ok.isPending}
      onClick={() => ok.mutate({ task: t.id, version: t.plan!.version }, { onError: sayRefusal })}
    >
      Plan OK
    </Button>
  );
}

function PlanTab({ t, open }: { t: TaskDetail; open: Set<number> }) {
  const nameOf = useNameOf();
  const plan = t.plan;
  const before = new Set(plan && plan.versions.length > 1 ? plan.versions.at(-2)!.text.split('\n') : []);
  const changed = plan && plan.versions.length > 1;
  const after = startsAfter(t, open);
  return (
    <ScrollFade className="min-h-0 flex-1">
      <div className="flex flex-col gap-4 px-6 py-5 text-sm">
        {!plan ? (
          <p className="text-muted">
            {t.plan_state === 'asked' || t.plan_state === 'writing' ? `${nameOf(t.to)} is writing the plan…` : 'No plan yet.'}
          </p>
        ) : (
          <>
            <p className="text-xs text-muted">
              {changed ? `Version ${plan.version} · updated ` : 'Written '}
              <Ago at={plan.at} />
              {plan.ok && (
                <>
                  {' · OK '}
                  <Ago at={plan.ok.at} />
                </>
              )}
            </p>
            <div className="flex flex-col">
              {plan.text.split('\n').map((line, i) => {
                const fresh = changed && line.trim() !== '' && !before.has(line);
                return (
                  <p
                    key={i}
                    className={`min-h-5 border-l-2 py-0.5 pl-3 whitespace-pre-wrap ${fresh ? 'border-accent bg-accent/10' : 'border-transparent'}`}
                  >
                    {line}
                  </p>
                );
              })}
            </div>
            {changed && <p className="text-xs text-muted">Lines marked on the left changed since version {plan.version - 1}.</p>}
          </>
        )}
        <p className="text-muted">
          Starts after: <span className="text-fg">{after.length ? after.map((n) => `T${n}`).join(', ') : 'nothing, starts at once'}</span>
        </p>
        {t.core_ask && (
          <p className="text-muted">
            Core logic asked <Ago at={t.core_ask.at} />
            {t.core_ask.sent_at ? '' : ', sent once the agent is free'}
            {t.core_ask.note && `: ${t.core_ask.note}`}
          </p>
        )}
      </div>
    </ScrollFade>
  );
}

import { Ago } from '#src/ui/Ago.tsx';
// A story's sub-task opened beside the graph: its place among the others, what it starts after and what starts then, and its Chat,
// Plan, Code and Result with the founder's verdicts.
import type { Task, TaskDetail } from '@handline/client';
import { useNameOf, useRead, useTasks, useWrite } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import { Check, ChevronLeft, ChevronRight, X } from 'lucide-react';
import type { ReactNode } from 'react';
import { TaskCode } from '#src/tools/TaskCode.tsx';
import { taskPlace, useLineDrafts } from '#src/changes/lineDrafts.ts';
import { useTabState } from '#src/persist/tabState.ts';
import { FaceName } from '#src/ui/Face.tsx';
import { wordOf, type Sub } from '#src/tools/storyWords.ts';
import { Button } from '#src/ui/Button.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { look } from '#src/ui/taskLook.tsx';
import { TaskChat } from '#src/tools/TaskTab.tsx';
import { sayRefusal } from '#src/ui/refusal.tsx';
import { ScrollFade } from '#src/ui/ScrollFade.tsx';
import { note, Result, VerdictButtons } from '#src/tools/Review.tsx';

type Tab = 'chat' | 'plan' | 'code' | 'result';
const CLOSED = ['done', 'discarded'];

export function SubTask({
  sub,
  story,
  onPick,
  at,
  onStep,
}: {
  sub: Sub;
  story: TaskDetail;
  onPick: (id: number | null) => void;
  /** Its place among the story's sub-tasks, 1-based, and how many there are. */
  at: [number, number];
  onStep: (d: -1 | 1) => void;
}) {
  const t = useRead('task.get', { task: sub.id }).data;
  const me = useMe();
  const [tab, setTab] = useTabState<Tab | null>(`subtab:${sub.id}`, null);
  const kids = useTasks({ parent: story.id, all: true }).data?.items ?? [];
  // Every hook before the early return: a render after the load must call the same ones.
  const code = !!useRead('task.changes', { task: sub.id }).data?.commits.length;
  const waiting = useLineDrafts(taskPlace(sub.id)).list.length > 0;
  if (!t) return <div className="flex-1" />;
  const open = new Set(story.subtasks.filter((x) => !CLOSED.includes(x.state)).map((x) => x.id));
  const word = wordOf(t, sub, me, open);
  const first: Tab = t.review ? 'result' : 'plan';
  const shown: Tab = tab === 'code' && !code ? first : (tab ?? first);
  const before = kids.filter((x) => (t.blocked_by ?? []).includes(x.id));
  const then = kids.filter((x) => (x.blocked_by ?? []).includes(t.id));
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex h-12 shrink-0 items-center gap-1 border-b border-border px-3 text-sm text-muted">
        <IconButton label="Previous" disabled={at[0] <= 1} onClick={() => onStep(-1)}>
          <ChevronLeft size={16} />
        </IconButton>
        <span className="tabular-nums">
          {at[0]} / {at[1]}
        </span>
        <IconButton label="Next" disabled={at[0] >= at[1]} onClick={() => onStep(1)}>
          <ChevronRight size={16} />
        </IconButton>
        <span className="ml-auto" />
        <IconButton label="Close" onClick={() => onPick(null)}>
          <X size={16} />
        </IconButton>
      </div>
      <div className="flex shrink-0 flex-col gap-2 px-6 pt-5">
        <h2 className="text-lg font-semibold text-fg">{t.title}</h2>
        <p className="flex items-center gap-2 text-sm text-muted">
          <FaceName who={t.to} className="text-fg" />
          <span>· T{t.id}</span>
          <span className={word.kind === 'you' ? 'text-warning' : ''}>· {word.text}</span>
        </p>
        <Links label="Starts after" tasks={before} onPick={onPick} />
        <Links label="Then starts" tasks={then} onPick={onPick} />
        <div role="tablist" className="mt-2 flex gap-4 border-b border-border text-sm">
          {(
            [
              ['chat', 'Chat', true],
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
      {shown === 'chat' && <TaskChat id={t.id} />}
      {shown === 'plan' && <PlanTab t={t} />}
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

function PlanTab({ t }: { t: TaskDetail }) {
  const nameOf = useNameOf();
  const plan = t.plan;
  const before = new Set(plan && plan.versions.length > 1 ? plan.versions.at(-2)!.text.split('\n') : []);
  const changed = plan && plan.versions.length > 1;
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

/** A row of linked sub-tasks, each with its state mark; a click opens it here. */
function Links({ label, tasks, onPick }: { label: string; tasks: Task[]; onPick: (id: number) => void }) {
  if (!tasks.length) return null;
  return (
    <div className="flex items-start gap-3 text-sm">
      <span className="w-24 shrink-0 text-muted">{label}</span>
      <span className="flex min-w-0 flex-col items-start gap-1">
        {tasks.map((x) => {
          const s = look(x);
          return (
            <button
              key={x.id}
              type="button"
              onClick={() => onPick(x.id)}
              title={`${s.word} · ${x.title}`}
              className="flex max-w-full min-w-0 items-center gap-1 rounded-sm px-1 text-fg hover:bg-hover/30"
            >
              <s.icon size={12} className="shrink-0 text-muted" />
              <span className="text-muted">T{x.id}</span>
              <span className="truncate">{x.title}</span>
            </button>
          );
        })}
      </span>
    </div>
  );
}

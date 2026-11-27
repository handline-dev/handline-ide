// The Plan strip on a task page: every sub-task as a flat row in the order the plan runs, the graph opened in place above them.
import { useTasks } from '@handline/app-data';
import { useMe } from '@handline/app-react';
import type { Task, TaskDetail } from '@handline/client';
import { Check, ChevronRight, GripVertical, Network, Play } from 'lucide-react';
import type { KeyboardEvent } from 'react';
import { taskOf } from '#src/layout/kinds.tsx';
import { useOpenable } from '#src/layout/openable.tsx';
import { useOpenTab } from '#src/layout/openTab.tsx';
import { useTabState } from '#src/persist/tabState.ts';
import { planHead, planRows, OPEN_UP_TO, type PlanRow } from '#src/tools/planRows.ts';
import { useRun } from '#src/tools/RunPlan.tsx';
import { PlanGraph } from '#src/tools/PlanGraph.tsx';
import { wordOf, type Word } from '#src/tools/storyWords.ts';
import { Button } from '#src/ui/Button.tsx';
import { Face, FaceName } from '#src/ui/Face.tsx';
import { useNarrow } from '#src/ui/narrow.ts';
import { look } from '#src/ui/taskLook.tsx';

const TONE = { merged: 'text-success', you: 'text-warning', working: 'text-accent', waiting: 'text-muted', draft: 'text-muted' };
const BAR = [
  ['merged', 'bg-success'],
  ['you', 'bg-warning'],
  ['working', 'bg-accent'],
  ['waiting', 'bg-border'],
  ['draft', 'bg-border'],
] as const;

export function PlanStrip({ t }: { t: TaskDetail }) {
  const me = useMe();
  const kids = useTasks({ parent: t.id, all: true }).data?.items ?? [];
  const subOf = new Map(t.subtasks.map((x) => [x.id, x]));
  const live = kids.filter((x) => x.state !== 'discarded');
  const open = new Set(live.filter((x) => x.state !== 'done').map((x) => x.id));
  const word = (x: Task) => wordOf(x, subOf.get(x.id), me, open);
  const rows = planRows(kids, word);
  const head = planHead(rows);
  const [shown, setShown] = useTabState<boolean | null>('plan-open', null);
  const [graph, setGraph] = useTabState('plan-graph', false);
  // A graph node opens its step here; its own rows below carry the drag and the new-tab clicks.
  // eslint-disable-next-line no-restricted-syntax -- a node is picked, not a tab opener of its own
  const openTab = useOpenTab();
  const { run } = useRun();
  if (!live.length) return null;
  const isOpen = shown ?? head.total <= OPEN_UP_TO;
  const drafts = live.filter((x) => x.state === 'draft');
  const count = (g: string) => rows.reduce((n, r) => n + (r.kind === 'merged' ? (g === 'merged' ? r.n : 0) : r.group === g ? 1 : 0), 0);
  return (
    <section className="flex flex-col border-t border-border">
      <div className="flex h-11 items-center gap-2 text-sm">
        <button
          type="button"
          aria-expanded={isOpen}
          onClick={() => setShown(!isOpen)}
          className="flex min-w-0 flex-1 items-center gap-2 self-stretch text-left"
        >
          <ChevronRight size={14} className={`shrink-0 text-muted transition-transform ${isOpen ? 'rotate-90' : ''}`} />
          <span className="font-semibold">Plan</span>
          <span className="flex h-1.5 w-28 min-w-6 shrink gap-px overflow-hidden rounded-full bg-border/40">
            {BAR.map(([g, tone]) => {
              const n = count(g);
              return n > 0 && <span key={g} className={tone} style={{ flex: n }} />;
            })}
          </span>
          <span className="shrink-0 text-muted tabular-nums">
            {head.merged}/{head.total}
          </span>
          {!isOpen && head.now && (
            <span className="min-w-0 truncate text-muted">
              · <span className="text-fg">T{head.now.task.id}</span> {head.now.task.title}
              {head.more > 0 && ` · +${head.more}`}
            </span>
          )}
        </button>
        <Button
          size="sm"
          variant="ghost"
          aria-pressed={graph}
          className="aria-pressed:bg-hover aria-pressed:text-fg"
          onClick={() => (setGraph(!graph), setShown(true))}
        >
          <Network size={12} />
          Graph
        </Button>
        {drafts.length > 0 && (
          <Button size="sm" variant="primary" onClick={() => run(drafts, open)}>
            <Play size={12} />
            Run
          </Button>
        )}
      </div>
      <div className="grid" style={{ gridTemplateRows: isOpen ? '1fr' : '0fr', transition: 'grid-template-rows 200ms ease' }}>
        <div className="min-h-0 overflow-hidden" inert={!isOpen}>
          {graph && (
            <div className="flex h-72 flex-col border-t border-border">
              <PlanGraph parent={t.id} subs={t.subtasks} open={null} bare onPick={(id) => openTab(taskOf(id))} />
            </div>
          )}
          <Rows rows={rows} />
        </div>
      </div>
    </section>
  );
}

/** Up and Down move through the rows, as in a list; Enter opens the one in focus (useOpenable). */
function step(e: KeyboardEvent<HTMLDivElement>) {
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
  const all = [...e.currentTarget.querySelectorAll<HTMLElement>('[data-plan-row]')];
  const i = all.indexOf(document.activeElement as HTMLElement);
  const next = all[e.key === 'ArrowDown' ? Math.min(all.length - 1, i + 1) : Math.max(0, i - 1)];
  if (!next) return;
  e.preventDefault();
  next.focus();
}

function Rows({ rows }: { rows: PlanRow[] }) {
  return (
    <div role="list" onKeyDown={step} className="max-h-150 overflow-y-auto">
      {rows.map((r) =>
        r.kind === 'merged' ? (
          <div key="merged" role="listitem" className="flex h-10 items-center gap-2 border-t border-border pl-6 text-sm text-success">
            <Check size={14} />
            {r.n} merged
          </div>
        ) : (
          <Row key={r.task.id} t={r.task} word={r.word} tone={TONE[r.group]} />
        ),
      )}
    </div>
  );
}

function Row({ t, word, tone }: { t: Task; word: Word; tone: string }) {
  const narrow = useNarrow();
  const s = look(t);
  const mark = t.state === 'done' ? <Check size={14} /> : <s.icon size={14} />;
  const state = (
    <span className={`flex shrink-0 items-center gap-1.5 text-sm ${tone} ${narrow ? 'text-xs' : 'w-52'}`}>
      {mark}
      <span className="truncate">{t.state === 'done' ? 'Merged' : word.text}</span>
    </span>
  );
  return (
    <button
      type="button"
      role="listitem"
      data-plan-row
      {...useOpenable(taskOf(t.id))}
      className={`group flex w-full items-center gap-2 border-t border-border text-left hover:bg-hover/30 ${narrow ? 'flex-col items-start py-2' : 'h-10'}`}
    >
      {!narrow && <GripVertical size={14} className="shrink-0 cursor-grab text-muted" aria-hidden />}
      {state}
      <span className="flex min-w-0 flex-1 items-center gap-2 self-stretch">
        <span className="min-w-0 flex-1 truncate text-sm">
          <span className="text-muted">T{t.id}</span> {t.title}
        </span>
        {narrow ? <Face who={t.to} /> : <FaceName who={t.to} className="shrink-0 text-sm text-muted" />}
      </span>
    </button>
  );
}

// The last step: what the first agent does, which AI tool it runs on, and starting it.
import { classify, hooks, sentence, useCreateProgress, useRepos, useWrite } from '@handline/app-data';
import { LoaderCircle, Sparkles } from 'lucide-react';
import { useEffect, useState } from 'react';
import { usePicked } from '#src/onboarding/data/picked.ts';
import { closeOnboarding, useAiTools } from '#src/onboarding/data/progress.ts';
import type { AiRow } from '#src/onboarding/data/steps.ts';
import { agentNameOf, factCards, LOOK_AROUND } from '#src/onboarding/firstTask.ts';
import { StepView } from '#src/onboarding/StepView.tsx';
import { Mark, vendorOf } from '#src/onboarding/Steps.tsx';
import { toast } from '#src/ui/toast.tsx';

export function AgentStep() {
  const ai = useAiTools();
  const repos = useRepos().data?.repos ?? [];
  const chosen = usePicked();
  const folder = chosen ?? (repos[0] ? { path: repos[0].path, name: repos[0].name } : null);
  const ready = ai.rows.filter((r): r is AiRow & { account: string } => r.state === 'ready' && !!r.account);
  const [vendor, setVendor] = useState<string | null>(null);
  const on = ready.find((r) => r.vendor === vendor) ?? ready[ready.length - 1];
  const facts = hooks.useRepoFacts({ path: folder?.path ?? '' }, { enabled: !!folder });
  const cards = [
    { title: 'Look around and propose 3 tasks', note: 'Reads only; nothing is changed.', task: LOOK_AROUND },
    ...factCards(facts.data).slice(0, 1),
  ];
  const [card, setCard] = useState(0);
  const [own, setOwn] = useState('');
  const task = own.trim() || (cards[card] ?? cards[0]!).task;
  const create = useWrite('agent.create');
  const [operation, setOperation] = useState<string | null>(null);
  const [startFailed, setStartFailed] = useState(false);
  const start = () =>
    folder &&
    on &&
    create.mutate(
      { name: agentNameOf(folder.name), role: 'developer', repo: folder.path, account: on.account },
      { onSuccess: (r) => setOperation((r as { operation: string }).operation) },
    );
  const line = folder ? 'It works on its own branch, so your checkout stays as you left it.' : 'Pick a project first: go back one step.';
  return (
    <StepView
      title="Give your first agent a task"
      line={line}
      primary={{
        label: operation ? 'Starting…' : `Start ${on ? vendorOf(on.vendor).name : 'agent'}`,
        disabled: !folder || !on || create.isPending || !!operation,
        onClick: start,
      }}
    >
      {folder && (
        <>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {cards.map((c, i) => (
              <button
                key={c.title}
                type="button"
                aria-pressed={!own.trim() && card === i}
                onClick={() => (setCard(i), setOwn(''))}
                className={`flex items-start gap-2.5 rounded-md border p-3 text-left ${!own.trim() && card === i ? 'border-accent' : 'border-border hover:bg-hover/30'}`}
              >
                <Sparkles size={14} className="mt-0.5 shrink-0 text-accent" />
                <span className="flex min-w-0 flex-col">
                  <span className="text-xs font-medium text-fg">{c.title}</span>
                  <span className="truncate text-xs text-muted">{c.note}</span>
                </span>
              </button>
            ))}
          </div>
          <input
            value={own}
            onChange={(e) => setOwn(e.target.value)}
            placeholder="Or describe it, e.g. Add a /health endpoint"
            className="h-9 rounded-md border border-border bg-bg px-3 text-sm text-fg outline-none placeholder:text-muted focus:border-accent"
          />
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted">
            Runs on
            <span className="flex rounded-md border border-border">
              {ready.map((r) => (
                <button
                  key={r.vendor}
                  type="button"
                  aria-pressed={on?.vendor === r.vendor}
                  onClick={() => setVendor(r.vendor)}
                  className={`flex items-center gap-1.5 px-2 py-1 text-fg ${on?.vendor === r.vendor ? 'bg-selected' : ''}`}
                >
                  <Mark vendor={r.vendor} small />
                  {vendorOf(r.vendor).name}
                </button>
              ))}
            </span>
            in {folder.name}, on its own branch
          </div>
          {!ready.length && <p className="text-xs text-warning">Log in to an AI tool first (step 1).</p>}
        </>
      )}
      {!!create.error && <p className="text-xs text-danger">{sentence(classify(create.error))}</p>}
      {startFailed && !operation && <p className="text-xs text-danger">The agent did not start. Try again.</p>}
      {operation && <Starting operation={operation} task={task} onFailed={() => (setOperation(null), setStartFailed(true))} />}
    </StepView>
  );
}

function Starting({ operation, task, onFailed }: { operation: string; task: string; onFailed: () => void }) {
  const p = useCreateProgress(operation, { task, box: '2h' }, () => {
    closeOnboarding();
    toast("You're set up. Change any of it, in more detail, in Settings.");
  });
  const failed = p.op?.state === 'failed';
  useEffect(() => {
    if (failed) onFailed();
  }, [failed, onFailed]);
  return (
    <p className="flex items-center gap-2 text-xs text-muted">
      <LoaderCircle size={12} className="animate-spin" />
      Starting the agent…
      {!!p.ask.error && <span className="text-danger">The first task was not sent: {sentence(classify(p.ask.error))}</span>}
    </p>
  );
}

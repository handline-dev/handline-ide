// Settings: the shared runs of This Mac or of one workspace. A shared run is a set of commands only N agents run at once; the others
// wait their turn (hl use). Agents set them; the founder sees them here and may add, change or remove any.
import { classify, hooks, sentence, useNameOf, useWrite } from '@handline/app-data';
import { Clock, Bot } from 'lucide-react';
import { useState } from 'react';
import { Block } from '#src/settings/Meter.tsx';
import { Button } from '#src/ui/Button.tsx';
import { Input, Select } from '#src/ui/Field.tsx';
import { ModalFrame, confirm, openModal, type Close } from '#src/ui/modal.tsx';
import { toast } from '#src/ui/toast.tsx';

type Run = NonNullable<ReturnType<typeof hooks.useRunList>['data']>['runs'][number];
type Draft = { name: string; scope: string; cmds: string; at_once: number; why: string };

const MAC = 'mac';

/** Every shared run, This Mac's first and then each workspace's, under its scope's name. */
export function SharedRuns() {
  const nameOf = useNameOf();
  // No event carries an agent's hl use add or a turn taken: read again every 10 s, as This Mac's leases are.
  const list = hooks.useRunList({}, { refetchMs: 10_000 });
  const set = useWrite('run.set');
  const remove = useWrite('run.remove');
  // A run's scope is mac or a workspace's key (w3); the key is sent back, the name shown.
  const named = new Map((hooks.useWorkspacesList().data?.workspaces ?? []).map((w) => [w.key, w.name]));
  const scopeWord = (scope: string) => (scope === MAC ? 'This Mac' : (named.get(scope) ?? scope));
  const runs = list.data?.runs ?? [];
  const scopes = [MAC, ...new Set([...named.keys(), ...runs.map((r) => r.scope).filter((x) => x !== MAC)])];
  const target = (scope: string) => (scope === MAC ? { scope: 'mac' as const } : { scope: 'workspace' as const, workspace: scope });
  const fail = (what: string) => (e: unknown) => toast(`Did not ${what}: ${sentence(classify(e)) || 'try again'}`, { tone: 'warning' });
  const edit = async (run?: Run) => {
    const d = await openModal<Draft>((close) => (
      <RunForm run={run} scopes={scopes} scopeWord={scopeWord} presets={list.data?.presets ?? []} close={close} />
    ));
    if (!d) return;
    set.mutate(
      {
        ...target(d.scope),
        name: d.name,
        cmds: d.cmds
          .split(',')
          .map((c) => c.trim())
          .filter(Boolean),
        at_once: d.at_once,
        why: d.why,
      },
      { onError: fail('save') },
    );
  };
  const drop = async (run: Run) => {
    const sure = await confirm({
      title: `Remove ${run.name}?`,
      message: 'Turns held now run to their end; nothing waits for it again.',
      confirmText: 'Remove',
      danger: true,
    });
    if (sure) remove.mutate({ ...target(run.scope), name: run.name }, { onError: fail('remove') });
  };
  return (
    <Block title="Shared runs">
      {list.data && !runs.length && <p className="text-sm text-muted">None.</p>}
      {scopes
        .filter((sc) => runs.some((r) => r.scope === sc))
        .map((sc) => (
          <div key={sc} className="flex flex-col">
            <h3 className="text-xs font-medium text-muted">{scopeWord(sc)}</h3>
            <ul className="-mx-2 flex flex-col">
              {runs
                .filter((r) => r.scope === sc)
                .map((r) => (
                  <li key={r.name} className="flex flex-col gap-1 border-t border-border px-2 py-2 first:border-t-0">
                    <div className="flex items-center gap-3">
                      <span className="min-w-0 flex-1">
                        <span className="text-sm font-medium text-fg">{r.name}</span>
                        <span className="ml-2 font-mono text-xs text-muted">{r.cmds.join(', ')}</span>
                      </span>
                      <span className="text-xs text-muted tabular-nums">{r.at_once} at once</span>
                      <Button variant="ghost" onClick={() => void edit(r)}>
                        Edit
                      </Button>
                      <Button variant="ghost" onClick={() => void drop(r)}>
                        Remove
                      </Button>
                    </div>
                    <span className="text-xs text-muted">{[`Set by ${nameOf(r.set_by)}`, r.why].filter(Boolean).join(' · ')}</span>
                    {r.held.map((h) => (
                      <span key={`h${h.agent}`} className="flex items-center gap-1.5 text-xs text-fg">
                        <Bot size={12} className="text-muted" />
                        {nameOf(h.agent)} has a turn
                      </span>
                    ))}
                    {r.waiting.map((w) => (
                      <span key={`w${w.agent}`} className="flex items-center gap-1.5 text-xs text-muted">
                        <Clock size={12} />
                        {nameOf(w.agent)} waits, {w.position} in line{w.task ? ` (T${w.task})` : ''}
                      </span>
                    ))}
                  </li>
                ))}
            </ul>
          </div>
        ))}
      <Button className="self-start" onClick={() => void edit()}>
        Add
      </Button>
    </Block>
  );
}

function RunForm({
  run,
  scopes,
  scopeWord,
  presets,
  close,
}: {
  run?: Run;
  scopes: string[];
  scopeWord: (scope: string) => string;
  presets: string[];
  close: Close<Draft>;
}) {
  const [d, setD] = useState<Draft>({
    name: run?.name ?? '',
    scope: run?.scope ?? MAC,
    cmds: run?.cmds.join(', ') ?? '',
    at_once: run?.at_once ?? 1,
    why: run?.why ?? '',
  });
  const ok = /^[a-z0-9][a-z0-9-]*$/.test(d.name) && d.cmds.trim() !== '' && Number.isInteger(d.at_once) && d.at_once >= 1;
  return (
    <ModalFrame
      title={run ? `Edit ${run.name}` : 'Add a shared run'}
      onClose={() => close()}
      footer={
        <>
          <Button onClick={() => close()}>Cancel</Button>
          <Button variant="primary" disabled={!ok} onClick={() => close(d)}>
            Save
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3 text-sm">
        <label className="flex flex-col gap-1">
          Name
          <Input value={d.name} disabled={!!run} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="ios-build" />
        </label>
        <label className="flex flex-col gap-1">
          For
          <Select value={d.scope} disabled={!!run} onChange={(e) => setD({ ...d, scope: e.target.value })}>
            {scopes.map((sc) => (
              <option key={sc} value={sc}>
                {scopeWord(sc)}
              </option>
            ))}
          </Select>
        </label>
        <label className="flex flex-col gap-1">
          Commands
          <Input
            value={d.cmds}
            onChange={(e) => setD({ ...d, cmds: e.target.value })}
            placeholder={presets.join(', ') || 'xcodebuild, tools/check.sh'}
          />
        </label>
        <label className="flex flex-col gap-1">
          At once
          <Input type="number" min={1} value={d.at_once} onChange={(e) => setD({ ...d, at_once: Number(e.target.value) })} />
        </label>
        <label className="flex flex-col gap-1">
          Why
          <Input value={d.why} onChange={(e) => setD({ ...d, why: e.target.value })} />
        </label>
      </div>
    </ModalFrame>
  );
}

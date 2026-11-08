import { cacheRewriteUsd, MODELS, VERBS } from '@handline/app-core';
import { EFFORTS, useAgentDetail, useAgentStrip, useServes, useVerbRun, useWrite } from '@handline/app-data';
import { ChevronDown } from 'lucide-react';
import { Button } from '#src/ui/Button.tsx';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { ModalFrame, modalText, openModal } from '#src/ui/modal.tsx';
import { MenuHeading, MenuRule } from '#src/chat/agent/menuParts.tsx';

type Restart = 'resume' | 'fresh';

/** Asks how a live session takes a new model: the same conversation (its context written once into the new model's cache), a
 * handoff then a new conversation, or no change. Resolves the restart mode, or undefined for no change. */
function askRestart(model: string, used: number | null) {
  const cost =
    used == null
      ? "Its context is written into the new model's cache once."
      : `Its ${Math.round(used / 1000)}k tokens of context are written into the new model's cache once, about $${cacheRewriteUsd(used, model).toFixed(2)} at list price.`;
  return openModal<Restart>((close) => (
    <ModalFrame
      title={`Switch to ${model}`}
      description="The session is live. It switches at its next idle moment."
      onClose={() => close()}
      footer={
        <>
          <Button onClick={() => close()}>{modalText.cancel}</Button>
          <Button onClick={() => close('fresh')}>Compact, then switch</Button>
          <Button variant="primary" autoFocus onClick={() => close('resume')}>
            Keep conversation
          </Button>
        </>
      }
    >
      <dl className="flex flex-col gap-1">
        <dt className="font-medium">Keep conversation</dt>
        <dd className="mb-2 text-muted">
          The same conversation on {model}. {cost}
        </dd>
        <dt className="font-medium">Compact, then switch</dt>
        <dd className="text-muted">The agent writes a handoff note, then starts a new conversation on {model}.</dd>
      </dl>
    </ModalFrame>
  ));
}

/** The agent's model and effort, in the message box: a menu changes either. A new model for a live session first asks how it
 * restarts (askRestart); any other change applies from its next session, and "Apply now" restarts it to take it at once. */
export function ModelChip({ agent }: { agent: string }) {
  const detail = useAgentDetail(agent).data;
  const used = useAgentStrip(agent).data?.ctx_used ?? null;
  const set = useWrite('agent.set', agent);
  const { served } = useServes('agent.set');
  const restart = useServes(VERBS.restart.method).served;
  const run = useVerbRun(agent);
  if (!detail) return null;
  const model = detail.settings?.model ?? detail.model;
  const effort = detail.settings?.effort ?? detail.effort;
  const waits =
    (!!detail.settings?.model && detail.settings.model !== detail.model) ||
    (!!detail.settings?.effort && detail.settings.effort !== detail.effort);
  const models = [...new Set([...MODELS, ...(model ? [model] : [])])];
  const change = (values: { model?: string; effort?: (typeof EFFORTS)[number] }, restart?: Restart) =>
    set.mutate({ agent, values, ...(restart && { restart }) });
  const pickModel = async (m: string) => {
    if (!detail.session_open) return change({ model: m });
    const restart = await askRestart(m, used);
    if (restart) change({ model: m }, restart);
  };
  return (
    <Menu
      trigger={
        <button
          type="button"
          disabled={!served}
          title={waits ? 'Applies from the next session' : 'Model and effort'}
          className="flex h-7 items-center gap-1 rounded-md px-2 text-xs text-muted outline-none hover:bg-hover/30 hover:text-fg focus-visible:ring-2 focus-visible:ring-accent data-[popup-open]:text-fg"
        >
          {model ?? 'Model'}
          {effort && <span>· {effort}</span>}
          {waits && <span className="size-1.5 rounded-full bg-warning" aria-label="applies from the next session" />}
          <ChevronDown size={12} />
        </button>
      }
    >
      <MenuHeading>Model</MenuHeading>
      {models.map((m) => (
        <MenuItem key={m} checked={m === model} onClick={() => m !== model && void pickModel(m)}>
          {m}
        </MenuItem>
      ))}
      <MenuRule />
      <MenuHeading>Effort</MenuHeading>
      {EFFORTS.map((e) => (
        <MenuItem key={e} checked={e === effort} onClick={() => e !== effort && change({ effort: e })}>
          {e}
        </MenuItem>
      ))}
      {waits && (
        <>
          <MenuRule />
          <MenuHeading>Applies from the next session</MenuHeading>
          {restart && detail.session_open && <MenuItem onClick={() => run(VERBS.restart)}>Apply now (restart)</MenuItem>}
        </>
      )}
    </Menu>
  );
}

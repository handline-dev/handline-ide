import { cacheRewriteUsd, MODELS, VERBS } from '@handline/app-core';
import { EFFORTS, useAgentDetail, useAgentStrip, useServes, useVerbRun, useWrite } from '@handline/app-data';
import { ChevronDown, MessagesSquare, Shrink, X } from 'lucide-react';
import type { ReactNode } from 'react';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { ModalFrame, modalText, openModal } from '#src/ui/modal.tsx';
import { MenuHeading, MenuRule } from '#src/chat/agent/menuParts.tsx';

type Restart = 'resume' | 'fresh';

/** One way to take the new model: a full-width button, its icon, its name and what it does inside it. */
function Choice({
  icon,
  title,
  text,
  primary,
  onClick,
}: {
  icon: ReactNode;
  title: string;
  text: ReactNode;
  primary?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      autoFocus={primary}
      onClick={onClick}
      className={`flex w-full items-start gap-3 rounded-md border px-3 py-2.5 text-left outline-none hover:bg-hover focus-visible:ring-2 focus-visible:ring-accent ${primary ? 'border-accent' : 'border-border'}`}
    >
      <span className={`mt-0.5 shrink-0 ${primary ? 'text-accent' : 'text-muted'}`}>{icon}</span>
      <span className="flex min-w-0 flex-col gap-0.5">
        <span className="font-medium">{title}</span>
        <span className="text-sm text-muted">{text}</span>
      </span>
    </button>
  );
}

/** Asks how a live session takes a new model, at its next idle moment: the same conversation (its context written once into the new
 * model's cache), a handoff then a new conversation, or no change. Resolves the restart mode, or undefined for no change. */
function askRestart(model: string, used: number | null) {
  const cost =
    used == null
      ? "Its context is written into the new model's cache once."
      : `${Math.round(used / 1000)}k tokens of context are written into the new model's cache once, about $${cacheRewriteUsd(used, model).toFixed(2)} at list price.`;
  return openModal<Restart>((close) => (
    <ModalFrame title={`Switch to ${model}`} onClose={() => close()}>
      <div className="flex flex-col gap-2 pb-1">
        <Choice
          primary
          icon={<MessagesSquare size={16} />}
          title="Keep conversation"
          text={`The same conversation on ${model}, from its next idle moment. ${cost}`}
          onClick={() => close('resume')}
        />
        <Choice
          icon={<Shrink size={16} />}
          title="Compact, then switch"
          text={`The agent writes a handoff note, then starts a new conversation on ${model}.`}
          onClick={() => close('fresh')}
        />
        <Choice icon={<X size={16} />} title={modalText.cancel} text="Keep the current model." onClick={() => close()} />
      </div>
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

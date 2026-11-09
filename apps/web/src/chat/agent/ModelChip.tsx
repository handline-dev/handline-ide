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

type Change = { model?: string; effort?: string };

/** The question's words for a change of model, effort or both. The cost is the context written once into the cache of the model the
 * conversation goes on in (a new effort clears the cached messages too). */
export function restartWords(c: Change, current: string, used: number | null) {
  const on = c.model ?? current;
  const cost =
    used == null
      ? 'Its context is written into the cache once.'
      : `${Math.round(used / 1000)}k tokens of context are written into the cache once, about $${cacheRewriteUsd(used, on).toFixed(2)} at list price.`;
  const what = c.model && c.effort ? `${c.model} at ${c.effort} effort` : (c.model ?? `${c.effort} effort`);
  return {
    title: c.model ? `Switch to ${what}` : `Change effort to ${c.effort}`,
    keep: `The same conversation on ${what}, from its next idle moment. ${cost}`,
    compactTitle: c.model ? 'Compact, then switch' : 'Compact, then change',
    compact: `The agent writes a handoff note, then starts a new conversation on ${what}.`,
    cancel: c.model ? 'Keep the current model.' : 'Keep the current effort.',
  };
}

/** Asks how a live session takes a new model or effort, at its next idle moment: the same conversation, a handoff then a new
 * conversation, or no change. Resolves the restart mode, or undefined for no change. */
function askRestart(c: Change, current: string, used: number | null) {
  const w = restartWords(c, current, used);
  return openModal<Restart>((close) => (
    <ModalFrame title={w.title} onClose={() => close()}>
      <div className="flex flex-col gap-2 pb-1">
        <Choice primary icon={<MessagesSquare size={16} />} title="Keep conversation" text={w.keep} onClick={() => close('resume')} />
        <Choice icon={<Shrink size={16} />} title={w.compactTitle} text={w.compact} onClick={() => close('fresh')} />
        <Choice icon={<X size={16} />} title={modalText.cancel} text={w.cancel} onClick={() => close()} />
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
  // A live session takes a new model or effort by restarting: it asks how first.
  const pick = async (values: { model?: string; effort?: (typeof EFFORTS)[number] }) => {
    if (!detail.session_open) return change(values);
    const restart = await askRestart(values, detail.model ?? model ?? '', used);
    if (restart) change(values, restart);
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
        <MenuItem key={m} checked={m === model} onClick={() => m !== model && void pick({ model: m })}>
          {m}
        </MenuItem>
      ))}
      <MenuRule />
      <MenuHeading>Effort</MenuHeading>
      {EFFORTS.map((e) => (
        <MenuItem key={e} checked={e === effort} onClick={() => e !== effort && void pick({ effort: e })}>
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

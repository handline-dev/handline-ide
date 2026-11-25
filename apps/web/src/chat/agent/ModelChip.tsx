import { MODELS, VERBS } from '@handline/app-core';
import { EFFORTS, useAgentDetail, useServes, useVerbRun, useWrite } from '@handline/app-data';
import { ChevronDown } from 'lucide-react';
import { Menu, MenuItem } from '#src/ui/Menu.tsx';
import { confirm } from '#src/ui/modal.tsx';
import { MenuHeading, MenuRule } from '#src/chat/agent/menuParts.tsx';

type Change = { model?: string; effort?: string };

/** The confirm's words for a change of model, effort or both: a live session takes it at its next idle moment by writing a handoff note
 *  and starting a new conversation on it. */
export function restartWords(c: Change) {
  const what = c.model && c.effort ? `${c.model} at ${c.effort} effort` : (c.model ?? `${c.effort} effort`);
  return {
    title: c.model ? `Switch to ${what}?` : `Change effort to ${c.effort}?`,
    message: `When it is next idle, it hands off and starts a new conversation on ${what}, without the cache it has now.`,
    confirm: c.model ? 'Switch' : 'Change',
  };
}

/** The agent's model and effort, in the message box: a menu changes either. A new model for a live session first asks how it
 * says it will hand off and start anew (a confirm); any other change applies from its next session, and "Apply now" restarts it to take it at once. */
export function ModelChip({ agent }: { agent: string }) {
  const detail = useAgentDetail(agent).data;
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
  const change = (values: { model?: string; effort?: (typeof EFFORTS)[number] }) => set.mutate({ agent, values });
  // A live session takes a new model or effort with a handoff and a new conversation: it says so first.
  const pick = async (values: { model?: string; effort?: (typeof EFFORTS)[number] }) => {
    if (!detail.session_open) return change(values);
    const w = restartWords(values);
    if (await confirm({ title: w.title, message: w.message, confirmText: w.confirm })) change(values);
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

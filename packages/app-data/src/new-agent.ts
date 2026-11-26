// The new-agent form, for the PC and the phone: its fields, the efforts to pick from, and the agent.create params the fields make.
import { useState } from 'react';
import type { AgentOptions } from '@handline/client';
import { useWrite } from '#src/write.ts';

export const EFFORTS = ['low', 'medium', 'high', 'xhigh', 'max'] as const;

/** What the form offers to pick from. Without `agent.options` the efforts are today's list and the rest is free text (models) or absent. */
export interface NewAgentChoices {
  /** Whether the daemon sent its options; false falls back to the fixed efforts and free-text model. */
  fromDaemon: boolean;
  efforts: readonly string[];
  /** The models to pick from, or null for free text. */
  models: string[] | null;
  /** Every account with its login state; `offered` is false for one that is not logged in. Empty when the daemon sent none. */
  accounts: { name: string; login_state: string; offered: boolean }[];
  roles: AgentOptions['roles'];
  /** By workspace key: the name an agent made there with no name typed gets (the name field's placeholder). */
  defaultNames: Record<string, string>;
}
export function shapeChoices(o: AgentOptions | undefined): NewAgentChoices {
  if (!o) return { fromDaemon: false, efforts: EFFORTS, models: null, accounts: [], roles: [], defaultNames: {} };
  return {
    fromDaemon: true,
    efforts: o.efforts,
    models: o.models,
    accounts: o.accounts.map((a) => ({ ...a, offered: a.login_state === 'ok' })),
    roles: o.roles,
    defaultNames: o.default_names,
  };
}

export interface NewAgentFields {
  name: string;
  role: string;
  /** The workspace folder it works in, as workspaces.list gives its path; "" for the default. */
  folder: string;
}
export const EMPTY_AGENT_FIELDS: NewAgentFields = { name: '', role: '', folder: '' };

/** The params of agent.create for a new agent in workspace: its plain name, the folder only when it is not the default. */
export function createParams(f: NewAgentFields, workspace: string) {
  return {
    name: f.name.trim(),
    role: f.role.trim(),
    workspace,
    ...(f.folder ? { folder: f.folder } : {}),
  };
}

/** The form for a new agent in workspace and its Create: `onCreated` hears of the operation the daemon started and the name it makes. */
export function useNewAgentForm(workspace: string, onCreated: (operation: string, name: string) => void) {
  const create = useWrite('agent.create');
  const [f, setF] = useState<NewAgentFields>(EMPTY_AGENT_FIELDS);
  const set = (k: keyof NewAgentFields, v: string) => setF((was) => ({ ...was, [k]: v }));
  const ready = !!f.name.trim() && !!f.role.trim();
  const send = () => {
    if (!ready || create.isPending) return;
    const params = createParams(f, workspace);
    create.mutate(params, {
      onSuccess: (r) => onCreated((r as { operation: string }).operation, params.name),
    });
  };
  return { create, f, set, ready, send };
}

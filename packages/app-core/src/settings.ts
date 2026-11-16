// An agent's settings fields (the bridge's agent_settings.py): what each takes, and who may change it.
import type { AgentSettingValues } from '@handline/client';

export type FieldKey = keyof AgentSettingValues;
export type FieldKind = 'text' | 'number' | 'select' | 'switch' | 'sites' | 'model';

export interface Field {
  key: FieldKey;
  label: string;
  section: 'model' | 'work' | 'session' | 'browser';
  kind: FieldKind;
  options?: string[];
  /** Shown beside the control in muted text. */
  note?: string;
  unit?: string;
}

export const MODELS = ['opus', 'sonnet', 'haiku'];

/** The keys an agent may change on itself; every other one is the founder's. */
export const OWN_KEYS: ReadonlySet<FieldKey> = new Set<FieldKey>(['model', 'effort', 'context_k', 'handoff_note']);

/** Whether this viewer may change the field: the founder any, anyone else only the agent's own keys. */
export const mayChange = (key: FieldKey, role: string | undefined): boolean => role === 'owner' || OWN_KEYS.has(key);

/** What the person typed for a field, or the problem in words; null is fine. Nothing invalid is sent. */
export function problemWith(f: Field, raw: string): string | null {
  if (f.kind === 'number') {
    if (!/^[0-9]+$/.test(raw.trim()))
      return f.key === 'idle_close' ? 'whole minutes, 0 for never' : 'a whole number of thousands of tokens';
    if (f.key === 'context_k' && Number(raw) < 1) return 'at least 1';
  }
  if (f.kind === 'model' && !raw.trim()) return 'a model name';
  return null;
}

/** The switch that asks first, and the words it asks in (the bridge's). Turning it the safe way asks nothing. */
export const DANGER: Partial<Record<FieldKey, { to: string; title: string; text: string; go: string }>> = {
  sandbox: {
    to: 'off',
    title: 'Run this agent without its sandbox',
    text: 'Its commands will run with nothing holding them back, until its session ends.',
    go: 'Turn off',
  },
};

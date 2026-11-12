// The fake daemon's kept rules (docs/design/people-workspaces.md "Workspace rules"): workspace.rules and the owner's workspace.rule.set.
// A workspace's row beats the machine default's; a key set on neither is not listed.
import type { Actor, Workspace, WorkspaceRule } from '@handline/contract';
import { Failure, invalidParams } from '#src/errors.ts';

type Key = WorkspaceRule['key'];
interface Row {
  value: unknown;
  set_by: string;
  set_at: number;
}
interface Host {
  now: () => Date;
  spaces: { list(): Workspace[] };
  accounts: { list(): { accounts: { name: string }[] } };
}
type Ok = (said: string, extra?: object) => object;

const MODES = ['plan', 'default', 'acceptEdits', 'auto'];
const names = (v: unknown) => Array.isArray(v) && v.every((x) => typeof x === 'string' && x !== '');
const whole = (v: unknown) => Number.isInteger(v) && (v as number) >= 0;
/** Each key's kind, and whether a value of it is a rule (not 0, empty or false). */
const KINDS: Record<Key, { valid: (v: unknown) => boolean; kind: string; on: (v: unknown) => boolean }> = {
  accounts: { valid: names, kind: 'a list of account names', on: (v) => (v as string[]).length > 0 },
  agent_cap: { valid: whole, kind: 'a whole number', on: (v) => v !== 0 },
  'lease_max.heavy': { valid: whole, kind: 'a whole number', on: (v) => v !== 0 },
  'lease_max.simulator': { valid: whole, kind: 'a whole number', on: (v) => v !== 0 },
  'lease_max.browser': { valid: whole, kind: 'a whole number', on: (v) => v !== 0 },
  'lease_max.port': { valid: whole, kind: 'a whole number', on: (v) => v !== 0 },
  tools_deny: { valid: names, kind: 'a list of tool names', on: (v) => (v as string[]).length > 0 },
  mode_max: { valid: (v) => v === '' || MODES.includes(v as string), kind: `empty or ${MODES.join(', ')}`, on: (v) => v !== '' },
  review_required: { valid: (v) => typeof v === 'boolean', kind: 'true or false', on: (v) => v === true },
  land_by: { valid: (v) => typeof v === 'string', kind: 'an agent name', on: (v) => v !== '' },
  land_check: { valid: (v) => typeof v === 'string', kind: 'a shell command', on: (v) => v !== '' },
};
const isKey = (k: unknown): k is Key => typeof k === 'string' && Object.hasOwn(KINDS, k);
/** The most bytes a workspace's rules text should hold (team.RulesTextLimit). */
const TEXT_LIMIT = 2048;
/** The machine default's rows are kept under this key. */
const MACHINE = '';

export class FakeRules {
  private rows = new Map<string, Map<Key, Row>>();
  /** Each workspace's rules text size in bytes, by key; 0 when unset, as for a workspace with no base.md. */
  readonly textBytes = new Map<string, number>();
  private host: Host;

  constructor(host: Host) {
    this.host = host;
  }

  owns = (method: string): boolean => method === 'workspace.rules' || method === 'workspace.rule.set';

  call(method: string, p: Record<string, unknown>, ok: Ok, actor: Actor): unknown {
    const ws = this.workspace(p.workspace);
    if (method !== 'workspace.rules') return this.set(ws, p, ok, actor);
    const rules = this.inForce(ws);
    return ws === MACHINE ? { rules } : { rules, text_bytes: this.textBytes.get(ws) ?? 0, limit_bytes: TEXT_LIMIT };
  }

  /** A workspace's key, by key or name; MACHINE when absent. */
  private workspace(ref: unknown): string {
    if (ref === undefined) return MACHINE;
    const ws = this.host.spaces.list().find((w) => w.key === ref || w.name === ref);
    if (!ws) throw new Failure('not_found', `no workspace ${String(ref)}`, { what: 'workspace' });
    return ws.key;
  }

  private inForce(ws: string): WorkspaceRule[] {
    const own = this.rows.get(ws);
    const machine = this.rows.get(MACHINE);
    return (Object.keys(KINDS) as Key[]).flatMap((key) => {
      const row = own?.get(key) ?? machine?.get(key);
      if (!row) return [];
      const source = own?.has(key) && ws !== MACHINE ? 'workspace' : 'machine';
      return [{ key, value: row.value, on: KINDS[key].on(row.value), source, set_by: row.set_by, set_at: row.set_at }];
    });
  }

  private set(ws: string, p: Record<string, unknown>, ok: Ok, actor: Actor) {
    if (!isKey(p.key)) throw invalidParams(`key is one of ${Object.keys(KINDS).join(', ')}`, 'key');
    const key = p.key;
    if (p.value === null) {
      this.rows.get(ws)?.delete(key);
      return ok(`${key} is cleared.`);
    }
    if (!KINDS[key].valid(p.value)) throw invalidParams(`${key} is ${KINDS[key].kind}`, 'value');
    if (key === 'accounts') {
      const known = new Set(this.host.accounts.list().accounts.map((a) => a.name));
      const missing = (p.value as string[]).filter((a) => !known.has(a));
      if (missing.length) throw invalidParams(`no account ${missing.join(', ')}`, 'value');
    }
    const rows = this.rows.get(ws) ?? new Map<Key, Row>();
    this.rows.set(ws, rows);
    rows.set(key, { value: p.value, set_by: actor.name, set_at: Math.floor(this.host.now().getTime() / 1000) });
    return ok(`${key} is set.`);
  }
}

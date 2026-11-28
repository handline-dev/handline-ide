// The fake daemon's agent context: an agent's role and standing context (agent.context, the owner's agent.context.set) and its memory
// notes (agent.memory, the owner's agent.memory.remove). The role gate is daemon.ts's, from the contract.
import type { Actor } from '@handline/contract';
import { Failure, invalidParams } from '#src/errors.ts';

const PARTS = ['role', 'base'] as const;
type Part = (typeof PARTS)[number];
interface Version {
  text: string;
  by: string;
  at: string;
}
/** One memory note, as agent.memory gives it. */
export interface MemoryNote {
  name: string;
  file: string;
  description: string;
  type: string;
  body: string;
}
/** An agent's memory folder: its notes in the order agent.memory gives them, and its MEMORY.md text. */
export interface MemoryFolder {
  index: string;
  notes: MemoryNote[];
}
type Ok = (said: string, extra?: object) => object;

const agentOf = (p: Record<string, unknown>): string => {
  if (typeof p.agent !== 'string' || !p.agent) throw invalidParams("agent is an agent's key", 'agent');
  return p.agent;
};
// eslint-disable-next-line no-control-regex -- the daemon refuses a control character other than tab and newline
const CONTROL = /[\u0000-\u0008\u000b-\u001f\u007f-\u009f\u2028\u2029]/;
/** Why s cannot be a new version of part, '' when it can (the daemon's db.InstructionRefusal). */
export const textRefusal = (part: string, s: string): string =>
  !s.trim() ? `${part} is the whole new text, not empty` : CONTROL.test(s) ? `${part} carries a control character` : '';

export class FakeAgentContext {
  /** Each agent's versions in force, by key; a test seeds them. */
  readonly texts = new Map<string, Partial<Record<Part, Version>>>();
  /** Each agent's memory folder, by key; a test seeds it. */
  readonly memory = new Map<string, MemoryFolder>();
  private now: () => Date;

  constructor(host: { now: () => Date }) {
    this.now = () => host.now();
  }

  owns = (method: string): boolean => /^agent\.(context|memory)(\.|$)/.test(method);

  call(method: string, p: Record<string, unknown>, ok: Ok, actor: Actor): unknown {
    const agent = agentOf(p);
    switch (method) {
      case 'agent.context': {
        const t = this.texts.get(agent) ?? {};
        return { agent, role: t.role ?? null, base: t.base ?? null };
      }
      case 'agent.context.set':
        return this.set(agent, p, ok, actor);
      case 'agent.memory':
        return { agent, notes: this.memory.get(agent)?.notes ?? [] };
      default:
        return this.remove(agent, p, ok);
    }
  }

  private set(agent: string, p: Record<string, unknown>, ok: Ok, actor: Actor): unknown {
    const given = PARTS.filter((k) => p[k] !== undefined && p[k] !== null);
    if (!given.length) throw invalidParams('name role, base or both, with the whole new text', 'role');
    for (const k of given) {
      const s = p[k];
      const why = typeof s === 'string' ? textRefusal(k, s) : `${k} is the whole new text, not empty`;
      if (why) throw invalidParams(why, k);
    }
    const changed = this.keep(agent, Object.fromEntries(given.map((k) => [k, p[k] as string])), actor.name);
    if (!changed.length) return ok(`${agent}'s text is that already`);
    return ok(`${agent}: ${changed.join(' and ')} replaced; applies when it next starts fresh`);
  }

  /** Keeps texts as agent's versions in force, by by; the parts that changed. */
  keep(agent: string, texts: Partial<Record<Part, string>>, by: string): Part[] {
    const t = this.texts.get(agent) ?? {};
    const changed = PARTS.filter((k) => texts[k] !== undefined && t[k]?.text !== texts[k]!.trim());
    for (const k of changed) t[k] = { text: texts[k]!.trim(), by, at: this.now().toISOString().slice(0, 19) + 'Z' };
    this.texts.set(agent, t);
    return changed;
  }

  private remove(agent: string, p: Record<string, unknown>, ok: Ok): unknown {
    if (typeof p.name !== 'string' || !p.name) throw invalidParams("name is a note's name as agent.memory gives it", 'name');
    const folder = this.memory.get(agent);
    let hit = folder?.notes.filter((n) => n.name === p.name || n.file === p.name) ?? [];
    if (!folder || !hit.length) throw new Failure('not_found', `${agent} has no memory note "${p.name}"`, { what: 'memory' });
    const named = hit.length;
    if (named > 1) hit = hit.filter((n) => n.file === p.name);
    if (hit.length !== 1) throw invalidParams(`${named} notes are named "${p.name}": name one by its file`, 'name');
    const note = hit[0]!;
    folder.notes = folder.notes.filter((n) => n !== note);
    // A line's first link names its note, ./ or not; a link out of the folder is never one.
    const link = (line: string) => {
      const f = /\]\(([^)]*)\)/.exec(line)?.[1]?.replace(/^\.\//, '');
      return f?.endsWith('.md') && !/[/\\]/.test(f) ? f : undefined;
    };
    folder.index = folder.index
      .split(/(?<=\n)/)
      .filter((line) => link(line) !== note.file)
      .join('');
    return ok(`${agent}'s memory note ${note.name} removed`);
  }
}

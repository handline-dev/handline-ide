// What the screens decide from daemon facts. Pure: no DOM, so tests run it in Node.
import type { AgentView, Prompt, PromptOption } from '@handline/client';
import { dotOfStatus } from '#src/status.ts';

export type Dot = 'running' | 'waking' | 'idle' | 'waiting' | 'blocked' | 'parked' | 'none';

/** Why an agent's mark is a warning, in the daemon's words: a blocker, the wall it hit, or the state's own detail; null when it says none. */
export const whyOf = (a: Pick<AgentView, 'blocker' | 'wall_reason' | 'state_detail'>): string | null =>
  [a.blocker, a.wall_reason, a.state_detail].map((t) => t?.trim()).find(Boolean) ?? null;

/** The rail dot: the daemon's status when it sends one, else its state word plus its block kinds and a handoff that is starting a new session. Unknown words read as idle. */
export function dotOf(a: Pick<AgentView, 'state' | 'has_dialog' | 'block' | 'handoff' | 'status'>): Dot {
  const s = a.status && dotOfStatus(a.status);
  if (s) return s;
  if (a.has_dialog || a.state === 'waiting') return 'waiting';
  if (a.block || a.state === 'stalled' || a.state === 'blocked') return 'blocked';
  if (a.state === 'no_terminal') return 'none';
  if (a.state === 'queued' || a.state === 'starting' || a.handoff?.phase === 'starting') return 'waking'; // a restarting agent is coming back, whatever state word it carries
  if (a.state === 'parked') return 'parked';
  if (a.state === 'running') return 'running';
  return 'idle';
}

/** Tasks waiting behind the one the agent is on: the owned tasks that are not done (queued or doing), less the current one. Null when the view carries no count. */
export function queuedBehind(a: AgentView): number | null {
  return a.task_count == null ? null : Math.max(0, a.task_count - (a.task ? 1 : 0));
}

/** An option that saves a standing rule ("Yes, and don't ask again"). */
export const isAlways = (o: PromptOption): boolean => /\balways\b|don.t ask again/i.test(o.label);

/** Options a card offers: a permission prompt offers yes and no, and "always" only as the daemon's own rule for an exact command (`rule`). */
export function offered(p: Prompt): PromptOption[] {
  return p.kind === 'permission'
    ? p.options.filter((o) => (o.answer === 'yes' || o.answer === 'no') && (!isAlways(o) || !!o.rule))
    : p.options;
}

/** What the card offers: a permission only yes and no; a dialog its options, minus the "always" ones when no rule can be kept. */
export function promptChoices(p: Prompt, rules: boolean): PromptOption[] {
  if (p.kind === 'dialog' && !rules) return p.options.filter((o) => !isAlways(o));
  if (p.kind === 'permission') return offered(p);
  return p.options;
}

/** A question's Cancel (Esc) is its own button, never a checkbox or a numbered row: the choices without it, and it. */
export function splitCancel(p: Prompt, choices: PromptOption[]): { cancel: PromptOption | undefined; opts: PromptOption[] } {
  const cancel = p.kind === 'question' ? choices.find((o) => o.key === 'esc') : undefined;
  return { cancel, opts: cancel ? choices.filter((o) => o !== cancel) : choices };
}

/** What a multi-select submits: the checked keys in the order the options are listed, not the order they were checked. */
export const checkedKeys = (opts: PromptOption[], picked: string[]): string[] => opts.map((o) => o.key).filter((k) => picked.includes(k));

export interface KV {
  getItem(k: string): string | null;
  setItem(k: string, v: string): void;
}

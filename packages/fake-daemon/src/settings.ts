// An agent's settings: agent.set.
import { badRequest, refused } from '#src/errors.ts';
import type { Call } from '#src/handlers.ts';

/** What a new session starts with: every value a string, as `hl agent set` takes it. */
export const defaultSettings = (model: string, effort: string): Record<string, string> => ({
  model,
  effort,
  weight: 'heavy',
  repo: '',
  browser: 'off',
  context_k: '300',
  idle_close: '',
  handoff_note: 'default',
  sandbox: 'on',
  sandbox_allow: '',
  mode: 'default',
  device: '',
  session_mode: '',
});

/** The keys an agent may change on itself; the rest are the founder's. */
export const OWN_KEYS = new Set(['model', 'effort', 'context_k', 'handoff_note']);
const ONE_OF: Record<string, string[]> = {
  effort: ['low', 'medium', 'high', 'xhigh', 'max'],
  weight: ['light', 'heavy'],
  sandbox: ['on', 'off'],
  browser: ['off', 'chrome'],
  handoff_note: ['default', 'on', 'off'],
  mode: ['default', 'auto', 'acceptEdits', 'plan'],
};
const SESSION_MODES = ['terminal', 'chat'];
const WHOLE = new Set(['context_k', 'idle_close']);
/** One line of sandbox_allow: `<day> <kind> <value>`, as the daemon checks it (go/data/team/sandbox.go). */
const LABEL = '[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?';
const ENTRY = new RegExp(
  `^\\d{4}-\\d{2}-\\d{2} (host (\\*\\.)?(${LABEL}\\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?|command [^*\\s][^\\n]{0,299}|write (/|~/)[^\\n]+|cert)$`,
);

function check(key: string, v: unknown): string {
  if (typeof v !== 'string') throw badRequest(`${key} takes a string`, key);
  if (!(key in defaultSettings('', ''))) throw badRequest(`${key} is not an agent setting`, key);
  if (key === 'session_mode') {
    if (v && !SESSION_MODES.includes(v)) throw badRequest(`session_mode must be one of ${SESSION_MODES.join(', ')}`, key);
    return v;
  }
  if (ONE_OF[key] && !ONE_OF[key].includes(v)) throw badRequest(`${key} is one of ${ONE_OF[key].join(', ')}`, key);
  if (WHOLE.has(key) && !/^[0-9]*$/.test(v)) throw badRequest(`${key} is a whole number`, key);
  if (key === 'context_k' && !v) throw badRequest('context_k is a whole number', key);
  if (key === 'model' && !v.trim()) throw badRequest('model is not empty', key);
  if (key === 'sandbox_allow' && v.split('\n').some((l) => l.trim() && !ENTRY.test(l.trim())))
    throw badRequest('sandbox_allow is one entry per line: <day> <host|command|write|cert> <value>', key);
  return v;
}

export const SETTINGS_HANDLERS: Record<string, (h: Call) => unknown> = {
  'agent.set': (h) => {
    const { c, p, actor, script, agent } = h;
    const s = agent();
    const values = p.values as Record<string, unknown> | undefined;
    if (!values || typeof values !== 'object' || !Object.keys(values).length)
      throw badRequest('values names the settings to change', 'values');
    const next = { ...s.settings };
    for (const [k, v] of Object.entries(values)) {
      if (c.caller.kind === 'agent' && !OWN_KEYS.has(k)) throw refused('role', `${k} is the founder's to set`);
      next[k] = check(k, v);
    }
    s.settings = next;
    if (typeof values.model === 'string' && s.state !== 'running') s.fx.model = values.model; // a working agent takes it when the work ends
    script.control(s, actor, 'agent.set', `${actor.name} set ${Object.keys(values).join(', ')}`);
    return script.accepted('Saved.');
  },
};

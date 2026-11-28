// An agent whose session is held on a lent machine: AgentView.held_on, and agent.restart_here while that machine is silent.
import type { Actor, AgentView, Timestamp } from '@handline/contract';
import type { FakeAdmin } from '#src/admin.ts';
import type { FakeChat } from '#src/chat.ts';
import { refused } from '#src/errors.ts';
import type { Script } from '#src/script.ts';
import type { Session } from '#src/session.ts';

interface Host {
  readonly sessions: Map<string, Session>;
  readonly admin: FakeAdmin;
  readonly script: Script;
  readonly chat: FakeChat;
  iso(): Timestamp;
}

/** The master's own machine, as node.status names it. */
const HERE = 'mac';

/** The lent machine the agent is placed on, silent while it is offline or not linked; null on the master's own. */
function heldOn(d: Host, agent: string): AgentView['held_on'] {
  const at = d.iso();
  const node = d.admin.lentNodes(at).find((n) => n.id === d.admin.placed.get(agent));
  if (!node) return null;
  const silent = node.online === false || d.admin.device(String(node.device)).connected === false;
  const since = silent ? Math.floor(Date.parse(String(node.last_seen)) / 1000) : null;
  return { node: String(node.id), machine: String(node.name), silent_since: since, restart_here: silent };
}

/** Sets held_on on every agent a seed placed on a lent machine. */
export function holdPlaced(d: Host): void {
  for (const s of d.sessions.values()) {
    const held = heldOn(d, s.fx.name);
    if (held) s.fx.view = { ...s.fx.view, held_on: held };
  }
}

/** agent.restart_here as the daemon answers it: refused unless that machine is silent; else the agent starts here, fresh. */
export function restartHere(d: Host, s: Session, actor: Actor, ok: (said: string) => object): object {
  const held = s.fx.view?.held_on;
  if (!held) throw refused('state', 'its session is not on another machine');
  if (!held.restart_here) throw refused('state', `${held.machine} is online and linked: it runs there`);
  d.admin.placed.delete(s.fx.name);
  s.fx.view = { ...s.fx.view, held_on: null };
  d.script.restartSession(s, true, actor); // its conversation stays on that machine
  const machine_moved = { from: held.machine, to: HERE, reason: `${held.machine} is silent; started here on a person's press` };
  d.chat.addItem(s.fx.name, { kind: 'machine_moved', machine_moved });
  return ok(`started on ${HERE}`);
}

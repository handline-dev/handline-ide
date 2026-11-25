// Keys written out as words (docs/design/people-workspaces.md, Contract): every who field holds a key, and the screens show it through
// the three lists. Pure.
import type { AgentView, Person, Workspace } from '@handline/client';

/** The three lists the app keeps cached: agents.list, people.list and workspaces.list. */
export interface Directory {
  agents: readonly Pick<AgentView, 'key' | 'name' | 'workspace' | 'hired_from'>[];
  people: readonly Person[];
  workspaces: readonly Workspace[];
}

export const EMPTY_DIRECTORY: Directory = { agents: [], people: [], workspaces: [] };

/** The daemon's own sender. */
const SYS = 'sys';

/** The word for an agent, as the daemon names it (T9201): its name, or `workspace/name` when another agent has that name too (every lead
 * is `lead`) and the reader is not in its workspace (`here`, a workspace key). */
function agentWord(d: Directory, a: Pick<AgentView, 'key' | 'name' | 'workspace'>, here?: string): string {
  if (a.workspace === here) return a.name;
  const ws = d.workspaces.find((w) => w.key === a.workspace);
  const twin = d.agents.some((x) => x.key !== a.key && x.name === a.name);
  return twin && ws ? `${ws.name}/${a.name}` : a.name;
}

/** What the screens show for a key: an agent's word (read from workspace `here` when the view is one workspace's), a person's display
 * name, a workspace's name, `task-board` for the daemon. A key the lists do not hold yet is shown as it is. */
export function nameOf(d: Directory, key: string, here?: string): string {
  const a = d.agents.find((x) => x.key === key);
  if (a?.hired_from) return nameOf(d, a.hired_from, here);
  if (a) return agentWord(d, a, here);
  const p = d.people.find((x) => x.key === key);
  if (p) return p.display_name;
  const w = d.workspaces.find((x) => x.key === key);
  if (w) return w.name;
  if (key === SYS) return 'task-board';
  // A key no list holds (an agent retired and gone, a person not read yet) is never shown raw: an id means nothing on a screen.
  // Only once the list is read: before that, every key is just not known yet.
  if (d.agents.length && /^a\d+$/.test(key)) return 'a removed agent';
  if (d.people.length && /^p\d+$/.test(key)) return 'someone';
  return key;
}

const REF = /<@([a-z]+[0-9]*)>/g;

/** The keys text names with `<@key>`, in order. */
export const refsIn = (text: string): string[] => [...text.matchAll(REF)].map((m) => m[1]);

/** Text as the screens show it: each stored `<@key>` is `@` and its name. */
export const renderRefs = (d: Directory, text: string): string =>
  text.includes('<@') ? text.replace(REF, (_, k: string) => `@${nameOf(d, k)}`) : text;

/** Text as a plain sentence: each stored `<@key>` is the name alone, for lines that are not chat (a proposal's change lines). */
export const plainRefs = (d: Directory, text: string): string =>
  text.includes('<@') ? text.replace(REF, (_, k: string) => nameOf(d, k)) : text;

/** Text as the app sends it: each `@word` that is an agent's word (`workspace/name`, or a name only one agent has) or a person's becomes `<@key>`;
 * any other `@word`, and one inside a path (`/@legacy/`), stays text. */
export function toRefs(d: Directory, text: string): string {
  // A person by handle, or by a one-word display name as renderRefs writes them; an agent's word wins over either.
  const keys = new Map<string, string>();
  for (const p of d.people) for (const w of [p.display_name, p.handle]) if (w && /^[A-Za-z0-9][\w.-]*$/.test(w)) keys.set(w, p.key);
  for (const a of d.agents) {
    keys.set(agentWord(d, a), a.key);
    const ws = d.workspaces.find((w) => w.key === a.workspace);
    if (ws) keys.set(`${ws.name}/${a.name}`, a.key);
  }
  return text.replace(/(^|[^A-Za-z0-9_.@</~-])@([A-Za-z0-9][A-Za-z0-9_./-]*)/g, (m, pre: string, word: string) => {
    const w = word.replace(/[./-]+$/, ''); // a sentence's full stop is not part of the name
    const k = keys.get(w);
    return k ? `${pre}<@${k}>${word.slice(w.length)}` : m;
  });
}

/** The agents grouped by workspace, in workspace order, each group's lead first and the rest as given. Agents whose workspace the list
 * does not hold yet come last, in a group of their own. */
export function byWorkspace<A extends Pick<AgentView, 'key' | 'workspace'>>(
  d: Pick<Directory, 'workspaces'>,
  agents: readonly A[],
): { workspace: Workspace | null; agents: A[] }[] {
  const groups = d.workspaces.map((w) => ({
    workspace: w,
    agents: agents.filter((a) => a.workspace === w.key).sort((x, y) => Number(y.key === w.lead) - Number(x.key === w.lead)),
  }));
  const known = new Set(d.workspaces.map((w) => w.key));
  const rest = agents.filter((a) => !known.has(a.workspace));
  return [...groups.filter((g) => g.agents.length > 0), ...(rest.length ? [{ workspace: null, agents: rest }] : [])];
}

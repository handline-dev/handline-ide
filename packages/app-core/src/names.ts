// Keys written out as words (docs/design/people-workspaces.md, Contract): every who field holds a key, and the screens show it through
// the three lists. Pure.
import type { AgentView, Person, Workspace } from '@handline/client';

/** The three lists the app keeps cached: agents.list, people.list and workspaces.list. */
export interface Directory {
  agents: readonly Pick<AgentView, 'key' | 'name' | 'workspace'>[];
  people: readonly Person[];
  workspaces: readonly Workspace[];
}

export const EMPTY_DIRECTORY: Directory = { agents: [], people: [], workspaces: [] };

/** The daemon's own sender. */
const SYS = 'sys';

/** The word for an agent: a lead by its workspace's name, another agent by its own name, or `workspace.name` when another workspace has
 * an agent of that name too. Agent names and workspace names never collide, so each word names one agent. */
function agentWord(d: Directory, a: Pick<AgentView, 'key' | 'name' | 'workspace'>): string {
  const ws = d.workspaces.find((w) => w.key === a.workspace);
  if (ws?.lead === a.key) return ws.name;
  const twin = d.agents.some(
    (x) => x.key !== a.key && x.name === a.name && d.workspaces.find((w) => w.key === x.workspace)?.lead !== x.key,
  );
  return twin && ws ? `${ws.name}.${a.name}` : a.name;
}

/** What the screens show for a key: an agent's word, a person's display name, a workspace's name, `task-board` for the daemon. A key
 * the lists do not hold yet is shown as it is. */
export function nameOf(d: Directory, key: string): string {
  const a = d.agents.find((x) => x.key === key);
  if (a) return agentWord(d, a);
  const p = d.people.find((x) => x.key === key);
  if (p) return p.display_name;
  const w = d.workspaces.find((x) => x.key === key);
  if (w) return w.name;
  return key === SYS ? 'task-board' : key;
}

const REF = /<@([a-z]+[0-9]*)>/g;

/** Text as the screens show it: each stored `<@key>` is `@` and its name. */
export const renderRefs = (d: Directory, text: string): string =>
  text.includes('<@') ? text.replace(REF, (_, k: string) => `@${nameOf(d, k)}`) : text;

/** Text as the app sends it: each `@word` that is an agent's word, a workspace's name or a person's handle becomes `<@key>`; any other
 * `@word` stays text. The app sends keys because a person has no workspace of their own for the daemon to read a bare name in. */
export function toRefs(d: Directory, text: string): string {
  const keys = new Map<string, string>();
  for (const a of d.agents) keys.set(agentWord(d, a), a.key);
  for (const p of d.people) if (p.handle) keys.set(p.handle, p.key);
  return text.replace(/(^|[^A-Za-z0-9_.@<-])@([A-Za-z0-9][A-Za-z0-9_.-]*)/g, (m, pre: string, word: string) => {
    const w = word.replace(/[.-]+$/, ''); // a sentence's full stop is not part of the name
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
    workspace: w as Workspace | null,
    agents: agents.filter((a) => a.workspace === w.key).sort((x, y) => Number(y.key === w.lead) - Number(x.key === w.lead)),
  }));
  const known = new Set(d.workspaces.map((w) => w.key));
  const rest = agents.filter((a) => !known.has(a.workspace));
  return [...groups.filter((g) => g.agents.length > 0), ...(rest.length ? [{ workspace: null, agents: rest }] : [])];
}

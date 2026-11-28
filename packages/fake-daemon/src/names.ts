// The fake's agents keep their dotted names as their keys. The words the app writes them out by come from the dotted name:
// the first segment is the workspace, the rest the agent's name,
// and the agent named just the segment is its lead.
import type { Person, Workspace } from '@handline/contract';

/** The one person: personal mode. The fake writes the person as human, as the daemon did before keys, so that is their key here. */
export const OWNER = 'human';
export const PEOPLE: Person[] = [{ key: OWNER, display_name: 'Founder', handle: null, role: 'owner', removed: false }];

export const segment = (key: string) => key.split('.')[0]!;

/** Workspace keys by segment, given out in the order the segments are first seen, so a key never changes within a run. */
const workspaceKeys = new Map<string, string>();
const workspaceKey = (seg: string) => {
  let k = workspaceKeys.get(seg);
  if (!k) workspaceKeys.set(seg, (k = `w${workspaceKeys.size + 1}`));
  return k;
};

/** An agent's identity fields of AgentView. */
export function identOf(key: string): { key: string; name: string; workspace: string; owner: string } {
  const seg = segment(key);
  return { key, name: key === seg ? 'lead' : key.slice(seg.length + 1), workspace: workspaceKey(seg), owner: OWNER };
}

/** workspaces.list over the agents' keys, oldest first; a workspace renamed or made at a folder has its name, folder or added folders
 * by segment. Its lead is the agent named by the segment; once that one is retired, its oldest other agent that is not a seat leads it,
 * as the daemon hands a retired lead's workspace to next_lead. Its session_mode is terminal unless set. */
export function workspacesOf(
  agents: readonly string[],
  seats: ReadonlySet<string> = new Set(),
  names: ReadonlyMap<string, string> = new Map(),
  folders: ReadonlyMap<string, string> = new Map(),
  added: ReadonlyMap<string, readonly string[]> = new Map(),
  plain: (path: string) => boolean = () => false,
  modes: ReadonlyMap<string, Workspace['session_mode']> = new Map(),
): Workspace[] {
  const segs = [...new Set(agents.map(segment))];
  return segs
    .map((seg) => {
      const leads = agents.filter((a) => segment(a) === seg && !seats.has(a));
      const lead = leads.includes(seg) ? seg : (leads[0] ?? null);
      const folder = folders.get(seg) ?? `/fake/${seg}`;
      const more = [...(added.get(seg) ?? [])];
      const all = [folder, ...more].filter((f) => f !== '');
      return {
        key: workspaceKey(seg),
        name: names.get(seg) ?? seg,
        folder,
        added_folders: more,
        folders: all.map((path, i) => ({ path, name: path.split('/').pop()!, git: !plain(path), default: i === 0 })),
        lead,
        next_lead: leads.find((a) => a !== lead) ?? null,
        session_mode: modes.get(seg) ?? 'terminal',
      };
    })
    .sort((a, b) => Number(a.key.slice(1)) - Number(b.key.slice(1)));
}

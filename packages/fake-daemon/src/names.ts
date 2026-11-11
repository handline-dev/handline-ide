// The fake's agents keep their dotted names as their keys. The words the app writes them out by come from the name the way the
// importer splits it (docs/design/people-workspaces.md, No migration): the first segment is the workspace, the rest the agent's name,
// and the agent named just the segment is its lead.
import type { Person, Workspace } from '@handline/contract';

/** The one person: personal mode. The fake writes the person as human, as the daemon did before keys, so that is their key here. */
export const OWNER = 'human';
export const PEOPLE: Person[] = [{ key: OWNER, display_name: 'Founder', handle: null, role: 'owner' }];

const segment = (key: string) => key.split('.')[0]!;

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

/** workspaces.list over the agents' keys. */
export function workspacesOf(agents: readonly string[]): Workspace[] {
  const segs = [...new Set(agents.map(segment))];
  return segs
    .map((seg) => ({
      key: workspaceKey(seg),
      name: seg,
      folder: `/fake/${seg}`,
      added_folders: [],
      lead: agents.includes(seg) ? seg : null,
    }))
    .sort((a, b) => Number(a.key.slice(1)) - Number(b.key.slice(1)));
}

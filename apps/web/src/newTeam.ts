// What a new team starts as (NewWorkspace.tsx): a tile per kind of team, made at once with no name asked; the founder renames it later.

export type Word = 'Empty' | 'Coding' | 'Writing' | 'Research' | 'Support' | 'Marketing';

/** A kind of team: its tile's word, the name its workspace is first given, and its lead's first role (none: the daemon's own). */
export type Starter = { word: Word; name: string; role?: string };

const lead = (work: string) =>
  `You lead a team that ${work}. In your first chat, ask what this team should work on, then plan it and hire members as the work needs them.`;

export const STARTERS: Starter[] = [
  { word: 'Empty', name: 'team' },
  { word: 'Coding', name: 'coding', role: lead('builds and ships software') },
  { word: 'Writing', name: 'writing', role: lead('writes and edits documents, posts and copy') },
  { word: 'Research', name: 'research', role: lead('researches questions and reports what it finds, with sources') },
  { word: 'Support', name: 'support', role: lead('answers customer questions and tracks the problems they raise') },
  { word: 'Marketing', name: 'marketing', role: lead('plans and runs marketing: campaigns, content and launches') },
];

/** The names a new workspace may not take, as the daemon refuses them: a workspace's, any agent's, a person's handle. */
export function takenNames(workspaces: { name: string }[], agents: { name: string }[], people: { handle: string | null }[]): string[] {
  return [...workspaces.map((w) => w.name), ...agents.map((a) => a.name), ...people.flatMap((p) => (p.handle ? [p.handle] : []))];
}

/** The workspace.create params a tile or a folder makes a team with. */
export function createParams(name: string, how: { folder?: string; role?: string }): Record<string, string> {
  return { name, ...(how.folder ? { folder: how.folder } : {}), ...(how.role ? { role: how.role } : {}) };
}

/** `base`, or `base-2`, `base-3` and so on past the names taken, as the daemon numbers a folder; kept to 32 letters. */
export function freeName(base: string, taken: Iterable<string>): string {
  const used = new Set(taken);
  for (let n = 1; ; n++) {
    const end = n === 1 ? '' : `-${n}`;
    const name = `${base.slice(0, 32 - end.length).replace(/-$/, '')}${end}`;
    if (!used.has(name)) return name;
  }
}

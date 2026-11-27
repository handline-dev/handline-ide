// What a new team starts as (NewWorkspace.tsx): a tile per kind of team, made at once with no name asked; the founder renames it later.

/** A kind of team: its tile's word, and the name its workspace is first given. */
export type Starter = { word: string; name: string };

/** The tiles, in order. */
export const STARTERS: Starter[] = [
  { word: 'Empty', name: 'team' },
  { word: 'Coding', name: 'coding' },
  { word: 'Writing', name: 'writing' },
  { word: 'Research', name: 'research' },
  { word: 'Support', name: 'support' },
  { word: 'Marketing', name: 'marketing' },
];

/** `base`, or `base-2`, `base-3` and so on past the names taken, as the daemon numbers a folder; kept to 32 letters. */
export function freeName(base: string, taken: Iterable<string>): string {
  const used = new Set(taken);
  for (let n = 1; ; n++) {
    const end = n === 1 ? '' : `-${n}`;
    const name = `${base.slice(0, 32 - end.length).replace(/-$/, '')}${end}`;
    if (!used.has(name)) return name;
  }
}

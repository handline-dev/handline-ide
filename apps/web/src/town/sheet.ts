// The town's people sheet and which person each agent is, apart from the 3D room, so any screen can draw an agent's face cheaply.
export const PEOPLE_SHEET = `${import.meta.env.BASE_URL}town/people.png`;
/** The sheet's cells: a row per person, a column per pose. */
export const SHEET = { cell: [140, 232], size: [420, 1392], walk: 220 } as const;
/** How many people the sheet holds. */
export const PEOPLE = SHEET.size[1] / SHEET.cell[1];

export const hashOf = (key: string) => [...key].reduce((n, ch) => (n * 31 + ch.charCodeAt(0)) >>> 0, 7);

/** A sheet row per person from its key, so it keeps it: the first from there on that no teammate has yet. */
export function figuresOf(people: readonly { key: string }[]): Map<string, number> {
  const taken = new Set<number>();
  return new Map(
    people.map((p) => {
      const start = hashOf(p.key) % PEOPLE;
      const figure = Array.from({ length: PEOPLE }, (_, i) => (start + i) % PEOPLE).find((f) => !taken.has(f)) ?? start;
      taken.add(figure);
      return [p.key, figure];
    }),
  );
}

/** Each agent's row as Town gives it: teammates in one workspace take different people, in the order agents.list has them. */
export function figureRows(agents: readonly { key: string; workspace?: string | null }[]): Map<string, number> {
  const by = new Map<string, { key: string }[]>();
  for (const a of agents) {
    const w = a.workspace ?? '';
    by.set(w, [...(by.get(w) ?? []), a]);
  }
  return new Map([...by.values()].flatMap((g) => [...figuresOf(g)]));
}

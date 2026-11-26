// The sidebar's agents that need the person but sit out of sight in its scrolled list: those above the view and those below it, in list
// order; one whose row is in sight anywhere is left out. A mark is a row (one agent) or a folded workspace's header (its agents), with where it is in the list.
export interface NeedsYouMark {
  keys: string[];
  top: number;
  bottom: number;
}

export function needsYouOut(marks: readonly NeedsYouMark[], view: { top: number; bottom: number }): { above: string[]; below: string[] } {
  const above: string[] = [];
  const below: string[] = [];
  const seen = new Set<string>();
  for (const m of marks) {
    if (m.bottom <= view.top) above.push(...m.keys);
    else if (m.top >= view.bottom) below.push(...m.keys);
    else m.keys.forEach((k) => seen.add(k));
  }
  // An agent in sight anywhere (pinned, or in its workspace) is in front of the person already.
  const out = (keys: string[]) => [...new Set(keys)].filter((k) => !seen.has(k));
  return { above: out(above), below: out(below) };
}

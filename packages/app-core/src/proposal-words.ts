// What an HR proposal is called in a list: a plain phrase per kind with the agent's name, not the daemon's `<kind> <agent>`. Pure.

type P = { kind?: string; agent: string; title: string; diff: string; changes?: readonly { kind: string; agent: string }[] | null };

const PHRASE: Record<string, (who: string) => string> = {
  add: (who) => `Add agent ${who}`,
  retire: (who) => `Retire ${who}`,
  rename: (who) => `Rename ${who}`,
  team: (who) => `Make team ${who}`,
  settings: (who) => `Change ${who}'s settings`,
  context: (who) => `Change ${who}'s prompt`,
  schedule: (who) => `Change ${who}'s schedule`,
  run: () => 'Run a command',
};

/** "Change ui's prompt"; a bundle is "3 changes"; a kind not known keeps the daemon's title. */
export function proposalTitle(p: P, nameOf: (key: string) => string): string {
  if (p.kind === 'bundle') {
    // Each step said as itself ("Change auditor's settings and Change auditor's prompt"), so the card says what it asks.
    const steps = [...new Set((p.changes ?? []).map((c) => (PHRASE[c.kind] ? PHRASE[c.kind](nameOf(c.agent)) : '')))];
    if (steps.length && steps.every(Boolean)) return steps.join(', then ');
    const n = p.diff.split('\n').filter((l) => l.trim()).length;
    return `${n} ${n === 1 ? 'change' : 'changes'}`;
  }
  const phrase = p.kind ? PHRASE[p.kind] : undefined;
  return phrase ? phrase(nameOf(p.agent)) : p.title;
}

/** One line of a change, read for a person: what it sets, from what (when the line says), to what. */
export type ChangeRow = { what: string; from?: string; to: string } | { text: string };

const cap = (w: string) => w.charAt(0).toUpperCase() + w.slice(1);
/** A proposal's change lines as rows: `model: sonnet -> opus` is Model from sonnet to opus, `effort: high` is Effort to high,
 *  `set model of x to opus` is Model to opus; a line in no such form is kept as it is. */
export function changeRows(diff: string): ChangeRow[] {
  return diff
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l): ChangeRow => {
      let m = /^([a-z_ ]+):\s*(.+?)\s*->\s*(.+)$/i.exec(l);
      if (m) return { what: cap(m[1].replace(/_/g, ' ')), from: m[2], to: m[3] };
      m = /^set ([a-z_ ]+?) of \S+ to (.+)$/i.exec(l);
      if (m) return { what: cap(m[1].replace(/_/g, ' ')), to: m[2] };
      m = /^([a-z_]+):\s*(.+)$/i.exec(l);
      if (m && m[1] !== 'run' && m[1] !== 'cwd') return { what: cap(m[1].replace(/_/g, ' ')), to: m[2] };
      return { text: l };
    });
}

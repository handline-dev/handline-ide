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
  sandbox: (who) => `Widen ${who}'s sandbox`,
  run: () => 'Run a command',
};

/** The same kind of step for several agents, said once with how many. */
const MANY: Record<string, (n: number) => string> = {
  add: (n) => `Add ${n} agents`,
  retire: (n) => `Retire ${n} agents`,
  rename: (n) => `Rename ${n} agents`,
  settings: (n) => `Change ${n} agents' settings`,
  context: (n) => `Change ${n} prompts`,
  schedule: (n) => `Change ${n} schedules`,
};

/** "Change ui's prompt"; a bundle is "3 changes"; a kind not known keeps the daemon's title. */
export function proposalTitle(p: P, nameOf: (key: string) => string): string {
  if (p.kind === 'bundle') {
    // Said by what it changes ("Retire audit-tools, change 2 prompts"), never by a count alone.
    const steps = p.changes ?? [];
    const byKind = new Map<string, string[]>();
    for (const c of steps) byKind.set(c.kind, [...(byKind.get(c.kind) ?? []), nameOf(c.agent)]);
    const parts = [...byKind].map(([kind, who]) => (who.length === 1 ? PHRASE[kind]?.(who[0]) : MANY[kind]?.(who.length)));
    if (parts.length && parts.every(Boolean)) return parts.map((x, i) => (i ? x!.charAt(0).toLowerCase() + x!.slice(1) : x)).join(', ');
    const n = p.diff.split('\n').filter((l) => l.trim()).length;
    return `${n} ${n === 1 ? 'change' : 'changes'}`;
  }
  // A sandbox ask's one line is what it lets the agent do: "Let web reach pypi.org".
  if (p.kind === 'sandbox' && p.diff.trim()) return `Let ${nameOf(p.agent)} ${p.diff.trim()}`;
  const phrase = p.kind ? PHRASE[p.kind] : undefined;
  return phrase ? phrase(nameOf(p.agent)) : p.title;
}

/** A decided proposal's title, where no line under it says why: a command run is called by its Why's first line (never the command,
 *  which can be cut at an id), any other by what it does; `plain` writes the ids inside as names. */
export function decidedTitle(p: P & { reason?: string | null }, nameOf: (key: string) => string, plain: (text: string) => string): string {
  const why = p.reason
    ?.split('\n')
    .find((l) => l.trim())
    ?.trim();
  if (p.kind === 'run') return why ? plain(why) : 'Run a command';
  return plain(proposalTitle(p, nameOf));
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

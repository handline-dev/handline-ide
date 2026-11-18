// What an HR proposal is called in a list: a plain phrase per kind with the agent's name, not the daemon's `<kind> <agent>`. Pure.

type P = { kind?: string; agent: string; title: string; diff: string };

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
    const n = p.diff.split('\n').filter((l) => l.trim()).length;
    return `${n} ${n === 1 ? 'change' : 'changes'}`;
  }
  const phrase = p.kind ? PHRASE[p.kind] : undefined;
  return phrase ? phrase(nameOf(p.agent)) : p.title;
}

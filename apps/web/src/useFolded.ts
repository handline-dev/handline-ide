import { useMemo } from 'react';
import { usePersisted } from '#src/persist/kv.ts';

/** Which teams are folded, kept per browser: how much of the tree fits depends on the screen. */
export function useFolded() {
  const [list, setList] = usePersisted<string[]>('folded', []);
  const folded: ReadonlySet<string> = useMemo(() => new Set(list), [list]);
  const toggleFold = (name: string) => setList((l) => (l.includes(name) ? l.filter((n) => n !== name) : [...l, name]));
  return { folded, toggleFold };
}

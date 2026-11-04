import { useState } from 'react';

/** Which teams are folded, kept per browser: how much of the tree fits depends on the screen. */
const FOLDED = 'web:folded';
export function useFolded() {
  const [folded, setFolded] = useState<ReadonlySet<string>>(() => {
    try {
      return new Set(JSON.parse(localStorage.getItem(FOLDED) ?? '[]') as string[]);
    } catch {
      return new Set();
    }
  });
  const toggleFold = (name: string) =>
    setFolded((f) => {
      const n = new Set(f);
      if (n.has(name)) n.delete(name);
      else n.add(name);
      try {
        localStorage.setItem(FOLDED, JSON.stringify([...n]));
      } catch {
        /* blocked storage: the fold just does not persist */
      }
      return n;
    });
  return { folded, toggleFold };
}

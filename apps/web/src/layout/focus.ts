import { useEffect } from 'react';

/** A thing to bring into view in a tab, asked from elsewhere (a file in Changes, an account in Usage), by the tab's kind and a
 * scope (the agent, or '' for an app-wide tab). Kept until the tab takes it, so it works whether the tab is open already or opens
 * with the ask. */
const pending = new Map<string, string>();
const listeners = new Set<() => void>();
const keyOf = (kind: string, scope: string) => `${kind}\n${scope}`;

export function focusIn(kind: string, scope: string, target: string) {
  pending.set(keyOf(kind, scope), target);
  listeners.forEach((l) => l());
}

/** In the tab: calls `show(target)` with each thing asked for, once its content is drawn (`ready`). */
export function useFocused(kind: string, scope: string, ready: boolean, show: (target: string) => void) {
  useEffect(() => {
    if (!ready) return;
    const key = keyOf(kind, scope);
    const take = () => {
      const target = pending.get(key);
      if (target === undefined) return;
      pending.delete(key);
      show(target);
    };
    take();
    listeners.add(take);
    return () => void listeners.delete(take);
  }, [kind, scope, ready, show]);
}

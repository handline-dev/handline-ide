import { useSyncExternalStore } from 'react';

/** A set of listeners: `emit` calls each, `subscribe` is useSyncExternalStore's, `count` says how many are on. */
export function listeners() {
  const all = new Set<() => void>();
  return {
    emit: () => all.forEach((l) => l()),
    subscribe: (l: () => void) => (all.add(l), () => void all.delete(l)),
    count: () => all.size,
  };
}

/** One value shared by every screen of this page: `get`, `set`, and `use` to draw it. */
export function createStore<T>(initial: T) {
  let value = initial;
  const { emit, subscribe } = listeners();
  const get = () => value;
  return {
    get,
    set: (next: T) => {
      value = next;
      emit();
    },
    // A use-name the React Compiler reads as a hook: a bare `.use()` is taken for React's `use` and memoised, so it never redrew.
    useValue: () => useSyncExternalStore(subscribe, get),
  };
}

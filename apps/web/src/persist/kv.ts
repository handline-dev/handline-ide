// Small values kept per browser (localStorage), for state that should outlive a reload: drafts, folds, layouts. Each value lives under
// `web:<name>` as JSON; a value that cannot be read (blocked storage, a value from another shape) reads as the fallback, and a write that
// cannot be made is dropped, so the app runs the same without storage. Bulky things (files) go in files.ts.
import { useState } from 'react';

const at = (name: string) => `web:${name}`;

export function read<T>(name: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(at(name));
    return raw == null ? fallback : (JSON.parse(raw) as T);
  } catch {
    return fallback;
  }
}

/** Writes `value`; undefined removes it. */
export function write<T>(name: string, value: T | undefined) {
  try {
    if (value === undefined) localStorage.removeItem(at(name));
    else localStorage.setItem(at(name), JSON.stringify(value));
  } catch {
    /* blocked or full storage: the value lives until the page closes */
  }
}

/** useState kept under `name`: read once, written on each change. */
export function usePersisted<T>(name: string, fallback: T) {
  const [value, setValue] = useState<T>(() => read(name, fallback));
  const set = (next: T | ((v: T) => T)) =>
    setValue((v) => {
      const n = typeof next === 'function' ? (next as (v: T) => T)(v) : next;
      write(name, n);
      return n;
    });
  return [value, set] as const;
}

import { useEffect, useRef } from 'react';

/** Calls `onEscape` on Escape while `on`, unless something nearer already took the key. */
export function useEscape(on: boolean, onEscape: () => void) {
  const latest = useRef(onEscape);
  useEffect(() => {
    latest.current = onEscape;
  });
  useEffect(() => {
    if (!on) return;
    const key = (e: KeyboardEvent) => e.key === 'Escape' && !e.defaultPrevented && latest.current();
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
  }, [on]);
}

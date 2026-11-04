import { useState } from 'react';

/** Drafts by agent, kept per browser: a half-written message waits through a tab switch or a reload. */
const DRAFTS = 'web:drafts';
const drafts: Record<string, string> = (() => {
  try {
    return JSON.parse(localStorage.getItem(DRAFTS) ?? '{}') as Record<string, string>;
  } catch {
    return {};
  }
})();
const keep = (agent: string, text: string) => {
  if (text) drafts[agent] = text;
  else delete drafts[agent];
  try {
    localStorage.setItem(DRAFTS, JSON.stringify(drafts));
  } catch {
    /* blocked storage: the draft lives until the page closes */
  }
};

/** An agent's draft; it outlives the composer that shows it. */
export function useDraft(agent: string) {
  const [draft, set] = useState(() => drafts[agent] ?? '');
  const [shown, setShown] = useState(agent);
  if (shown !== agent) {
    setShown(agent);
    set(drafts[agent] ?? '');
  }
  const setDraft = (next: string | ((d: string) => string)) =>
    set((d) => {
      const v = typeof next === 'function' ? next(d) : next;
      keep(agent, v);
      return v;
    });
  return [draft, setDraft] as const;
}

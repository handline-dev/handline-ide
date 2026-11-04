import { useEffect, useState } from 'react';
import { labelOf } from '#src/layout/kinds.tsx';
import { initial, reduce, type Action, type Node } from '#src/layout/tree.ts';

/** Each agent, and each page (Tasks, Requests, Usage) under `page/<kind>`, has its own workspace layout:
 * picking one in the sidebar brings back its panes and tabs. Kept in localStorage, so a reload keeps them. */
const KEY = 'web:layouts';
const PAGE = 'page/';
type Saved = { agent: string | null; layouts: Record<string, Node> };

/** `s` with a layout for its picked workspace, made when there is none yet: a page's starts on its own tab, fixed. */
const ensured = (s: Saved): Saved => {
  const k = s.agent ?? '';
  if (s.layouts[k]) return s;
  const page = k.startsWith(PAGE) ? k.slice(PAGE.length) : null;
  return { ...s, layouts: { ...s.layouts, [k]: page ? initial(page, labelOf(page)) : initial() } };
};

function load(): Saved {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) return ensured(JSON.parse(raw) as Saved);
  } catch {
    /* blocked storage or an old shape: start fresh */
  }
  return ensured({ agent: null, layouts: {} });
}

export function useAgentLayouts() {
  const [saved, setSaved] = useState(load);
  useEffect(() => {
    try {
      localStorage.setItem(KEY, JSON.stringify(saved));
    } catch {
      /* blocked: the layout just does not persist */
    }
  }, [saved]);

  // Before any agent is picked the workspace still works, under no agent ('').
  const key = saved.agent ?? '';
  const layout = saved.layouts[key];
  // Picking a workspace shows its main tab; its other tabs stay as they were left.
  const open = (space: string, opens: string, title: string) =>
    setSaved((s) => {
      const e = ensured({ ...s, agent: space });
      return { ...e, layouts: { ...e.layouts, [space]: reduce(e.layouts[space], { type: 'show', opens, title }) } };
    });
  const page = key.startsWith(PAGE) ? key.slice(PAGE.length) : null;
  return {
    agent: page === null ? saved.agent : null,
    page,
    layout,
    select: (agent: string) => open(agent, 'chat', 'Chat'),
    selectPage: (kind: string, title: string) => open(PAGE + kind, kind, title),
    dispatch: (action: Action) =>
      setSaved((s) => {
        const k = s.agent ?? '';
        return { ...s, layouts: { ...s.layouts, [k]: reduce(s.layouts[k], action) } };
      }),
  };
}

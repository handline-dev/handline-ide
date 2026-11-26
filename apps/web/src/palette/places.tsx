// The places the palette goes to, from what the app's navigation already knows: the pages, the settings' sections, the agents and the
// workspaces of this Mac.
import { byWorkspace } from '@handline/app-core';
import { useAgents, useDirectory, useNameOf, useNav } from '@handline/app-data';
import { Settings2 } from 'lucide-react';
import type { ReactNode } from 'react';
import { activityOf, ordered } from '#src/layout/agentList.ts';
import { chatOf, iconOf, labelOf, workspaceOf } from '#src/layout/kinds.tsx';
import { goAgent, goPage } from '#src/layout/openTab.tsx';
import { openSettings, SECTIONS } from '#src/settings/open.ts';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';

/** One row of the palette that opens something. `opens`: the tab it is, for a new tab or one beside (null: it opens no tab);
 * `here`: what a plain Enter or click does, the focused tab going there as it does from the sidebar. */
export type Entry = {
  key: string;
  icon: ReactNode;
  label: ReactNode;
  /** Said after the label, muted: whose it is, what kind of thing. */
  hint?: string;
  /** A second line: the words found. */
  note?: ReactNode;
  opens: string | null;
  here: () => void;
  /** Runs after any open: bringing a comment into view once its task is drawn. */
  then?: () => void;
};
/** A place also answers to its other words: a workspace's name beside its agent. */
export type Place = Entry & { label: string; words?: string };

/** A tab as an entry: the focused tab goes to it. */
export function tabEntry(opens: string, nameOf: (key: string) => string): Place {
  const label = labelOf(opens, nameOf);
  return { key: opens, icon: iconOf(opens), label, opens, here: () => goPage(opens, label) };
}

/** The pages, as the sidebar and the status row open them; Town and Settings cover the app and are no tab. */
const PAGES: { kind: string; tab: boolean }[] = [
  { kind: 'tasks', tab: true },
  { kind: 'requests', tab: true },
  { kind: 'usage', tab: true },
  { kind: 'town', tab: false },
  { kind: 'preferences', tab: false },
];
const word = (s: string) => s[0].toUpperCase() + s.slice(1);

export function usePages(): Place[] {
  return PAGES.map(({ kind, tab }) => {
    const label = labelOf(kind);
    return { key: `page:${kind}`, icon: iconOf(kind), label, opens: tab ? kind : null, here: () => goPage(kind, label) };
  });
}

/** Every place, in the order equals are listed: the pages, the agents as the sidebar orders them, the workspaces, the settings' sections. */
export function usePlaces(): Place[] {
  const dir = useDirectory();
  const nameOf = useNameOf();
  const order = useNav().nav.order;
  const pages = usePages();
  const groups = byWorkspace(dir, ordered(useAgents().data?.agents ?? [], order));
  const agents: Place[] = groups.flatMap(({ workspace, agents: inIt }) =>
    inIt.map((a) => ({
      key: `agent:${a.key}`,
      icon: <ActivityIcon act={activityOf(a)} />,
      label: a.name,
      hint: workspace?.name,
      words: workspace?.name,
      opens: chatOf(a.key),
      here: () => goAgent(a.key),
    })),
  );
  const workspaces: Place[] = dir.workspaces.map((w) => ({ ...tabEntry(workspaceOf(w.key), nameOf), hint: 'Workspace' }));
  const settings: Place[] = SECTIONS.map((section) => ({
    key: `settings:${section}`,
    icon: <Settings2 size={14} />,
    label: word(section),
    hint: 'Settings',
    words: 'settings',
    opens: null,
    here: () => openSettings({ section }),
  }));
  return [...pages, ...agents, ...workspaces, ...settings];
}

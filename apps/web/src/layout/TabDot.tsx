import { tabHasDot, type TabTarget } from '@handline/app-core';
import { useAgents, useTask, useUnread } from '@handline/app-data';
import { useMe } from '@handline/app-react';

/** The dot on a tab in the background that has something to read. */
export function TabDot({ target }: { target: NonNullable<TabTarget> }) {
  const agents = useAgents().data?.agents;
  const task = useTask(target.on === 'task' ? target.id : 0, { enabled: target.on === 'task' }).data;
  const inboxUnread = useUnread().length;
  const me = useMe();
  const agentUnread = target.on === 'agent' ? agents?.find((a) => a.key === target.name)?.founder_unread : undefined;
  if (!tabHasDot(target, { me, agentUnread, task, inboxUnread })) return null;
  return <span role="img" aria-label="Unread" className="size-1.5 shrink-0 rounded-full bg-accent" />;
}

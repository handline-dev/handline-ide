// What a tab shows, by what it opens: a tab bar's body (Workspace.tsx), and the town's nearby window, which shows one thing with no tabs.
import { Chat } from '#src/chat/Chat.tsx';
import { LocatedTab } from '#src/chat/Located.tsx';
import { Changes } from '#src/changes/Changes.tsx';
import { TabScope } from '#src/connection/clusters.tsx';
import { labelOf, mountKey, opened, ownedBy, planRoute, shellHandle } from '#src/layout/kinds.tsx';
import { PaneOpen, TabRoute, type How } from '#src/layout/openTab.tsx';
import { PageTab } from '#src/layout/pageTab.tsx';
import type { Tab } from '#src/layout/tree.ts';
import { ProposalView } from '#src/requests/ProposalView.tsx';
import { AgentPage } from '#src/settings/AgentPage.tsx';
import { DocTab } from '#src/tools/DocTab.tsx';
import { MarkdownTab } from '#src/tools/MarkdownTab.tsx';
import { PlanTab } from '#src/tools/Story.tsx';
import { TaskTab } from '#src/tools/TaskTab.tsx';
import { CodeTab } from '#src/tools/TaskCode.tsx';
import { TasksTab } from '#src/tools/TasksTab.tsx';
import { AnyShellTab, TerminalTab } from '#src/tools/TerminalTab.tsx';
import { ScreenTab } from '#src/tools/ScreenTab.tsx';
import { TextTab } from '#src/tools/TextTab.tsx';
import { WebTab } from '#src/tools/WebTab.tsx';
import { Boundary, brokenView } from '#src/ui/Boundary.tsx';
import { WorkspacePage } from '#src/workspace/WorkspacePage.tsx';

/** `tab` in `cluster` (tabIn has taken the cluster off what it opens); `who` the agent it draws with; `onOpen` opens what it links to,
 *  `onChoose` puts something else in the tab itself (a plan's own steps). */
export function TabBody({
  tab: current,
  cluster,
  who,
  onOpen,
  onChoose,
}: {
  tab: Tab | undefined;
  cluster: string;
  who: string | null;
  onOpen: (opens: string, how?: How) => void;
  onChoose: (opens: string, title: string) => void;
}) {
  const own = ownedBy(current?.opens ?? '');
  const shown = own?.kind ?? current?.opens;
  return (
    <Boundary key={`${current?.id}:${mountKey(current?.opens)}`} loading={null} failed={brokenView('This tab')}>
      <TabScope id={current?.id ?? null} cluster={cluster}>
        <PaneOpen.Provider value={onOpen}>
          {current?.opens?.startsWith('chat:') && <Chat key={current.opens} agent={current.opens.slice(5)} />}
          {shown === 'diff' && who && <Changes key={who} agent={who} />}
          {shown === 'shell' && who && <TerminalTab key={who} agent={who} />}
          {shown === 'screen' && who && <ScreenTab key={who} agent={who} />}
          {shellHandle(shown ?? '') && <AnyShellTab key={current!.id} agent={own ? who : null} handle={shellHandle(shown!)!} />}
          {shown === 'markdown' && who && <MarkdownTab key={current!.id} agent={who} tab={current!.id} />}
          {current?.opens === 'web' && <WebTab key={current.id} tab={current.id} agent={who} />}
          {current?.opens?.startsWith('web:') && <WebTab key={current.id} tab={current.id} agent={who} start={current.opens.slice(4)} />}
          {current?.opens?.startsWith('task:') && <TaskTab key={current.opens} id={Number(current.opens.slice(5))} />}
          {current?.opens?.startsWith('msg:') && <LocatedTab key={current.opens} request={Number(current.opens.slice(4))} />}
          {current?.opens?.startsWith('plan:') && (
            <TabRoute.Provider value={(opens) => onChoose(opens, labelOf(opens))}>
              <PlanTab key={mountKey(current.opens)} {...planRoute(current.opens)} />
            </TabRoute.Provider>
          )}
          {current?.opens?.startsWith('code:') && <CodeTab key={current.opens} id={Number(current.opens.slice(5))} />}
          {shown?.startsWith('file:') && who && <MarkdownTab key={current!.id} agent={who} tab={current!.id} start={shown.slice(5)} />}
          {current?.opens?.startsWith('text:') && <TextTab key={current.opens} id={current.opens.slice(5)} />}
          {current?.opens?.startsWith('proposal:') && <ProposalView key={current.opens} id={Number(current.opens.slice(9))} />}
          {current?.opens?.startsWith('doc:') && <DocTab key={current.opens} at={current.opens.slice(4)} />}
          <PageTab opens={current?.opens} />
          {opened(current?.opens ?? '', 'agent') && <AgentPage key={current!.opens} agent={opened(current!.opens!, 'agent')!} />}
          {opened(current?.opens ?? '', 'ws') && <WorkspacePage key={current!.opens} ws={opened(current!.opens!, 'ws')!} />}
          {shown === 'tasks' && <TasksTab key={own ? who : undefined} agent={who ?? null} named={!!own} />}
        </PaneOpen.Provider>
      </TabScope>
    </Boundary>
  );
}

import { byWorkspace, dotOf, queuedBehind } from '@handline/app-core';
import { useAgents, useDirectory, useNameOf, useNav } from '@handline/app-data';
import { AgentMenu } from '#src/AgentMenu.tsx';
import { newAgent } from '#src/NewAgent.tsx';
import { newWorkspace } from '#src/NewWorkspace.tsx';
import type { AgentView, Workspace } from '@handline/client';
import { JobList, type Job } from '#src/chat/agent/BackgroundItem.tsx';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { ago, useNow } from '#src/format/time.ts';
import { ChevronDown, ChevronLeft, ChevronRight, ListChecks, Pin, Plus, PanelLeft } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { activityOf, holdLine, ordered, splitPinned } from '#src/layout/agentList.ts';
import { chatOf, footerPages, pages } from '#src/layout/kinds.tsx';
import { useRequests } from '#src/requests/data.ts';
import { pageSpace } from '#src/layout/agentLayouts.ts';
import { Row } from '#src/sidebar/Row.tsx';
import { SetupCard } from '#src/onboarding/SetupCard.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { useScrollFade } from '#src/ui/ScrollFade.tsx';
import { DropArea, useDropTarget } from '#src/ui/dropTarget.tsx';
import { useKeptScroll } from '#src/ui/keptScroll.ts';

/** Top-left controls: collapse the sidebar, back, forward. They stay put when the sidebar is collapsed. */
export function SidebarControls({ onToggle }: { onToggle: () => void }) {
  return (
    <div className="flex items-center gap-0.5">
      <IconButton label="Toggle sidebar" onClick={onToggle}>
        <PanelLeft size={16} />
      </IconButton>
      <IconButton label="Back" onClick={() => history.back()}>
        <ChevronLeft size={16} />
      </IconButton>
      <IconButton label="Forward" onClick={() => history.forward()}>
        <ChevronRight size={16} />
      </IconButton>
    </div>
  );
}

export const sidebarWidth = { min: 180, max: 480, initial: 240 };

/** The newest message to the founder they have not read: one line of it, or nothing when all is read. */
function unreadNote(a: AgentView): string | undefined {
  if (!a.founder_unread || !a.last_to_founder) return undefined;
  return a.last_to_founder.text.replace(/\s+/g, ' ').trim();
}

/** Resting on an agent's row shows what runs behind its prompt, beside the row; a row with nothing running shows no card. */
function AgentRowCard({ jobs, children }: { jobs: Job[]; children: ReactNode }) {
  return (
    <HoverCard side="right" off={jobs.length === 0} trigger={<div>{children}</div>}>
      <JobList jobs={jobs} />
    </HoverCard>
  );
}

type Pick = {
  agent: string | null;
  onSelect: (name: string) => void;
  onSettings: (name: string) => void;
};

/** The agent row being dragged and the section it came from; a drop reorders only within that section. */
let dragged: { name: string; group: string } | null = null;

/** The rows of one section. `onMove(names)` hears the section's new order after a drag within it. */
function AgentRows({ agents, pick, onMove, here }: { agents: AgentView[]; pick: Pick; onMove: (names: string[]) => void; here?: string }) {
  const now = useNow();
  // A workspace's section (`here`) names its agents as that workspace does; Pinned, which mixes workspaces, as from outside.
  const nameOf = useNameOf(here);
  const names = agents.map((a) => a.key);
  const group = names.join('|');
  const [line, setLine] = useState<{ at: string; after: boolean } | null>(null);
  const drop = (target: string, after: boolean) => {
    if (!dragged || dragged.group !== group || dragged.name === target) return;
    const rest = names.filter((n) => n !== dragged!.name);
    const i = rest.indexOf(target) + (after ? 1 : 0);
    onMove([...rest.slice(0, i), dragged.name, ...rest.slice(i)]);
  };
  return agents.map((agent) => {
    const opens = chatOf(agent.key);
    return (
      <DropArea
        key={agent.key}
        className="relative flex flex-col"
        onDragStart={(e) => {
          e.stopPropagation();
          dragged = { name: agent.key, group };
        }}
        onDragEnd={() => {
          dragged = null;
          setLine(null);
        }}
        accepts={() => dragged?.group === group}
        onOver={(e) => {
          e.stopPropagation();
          const r = e.currentTarget.getBoundingClientRect();
          setLine({ at: agent.key, after: e.clientY > r.top + Math.min(r.height, 32) / 2 });
        }}
        onLeave={() => setLine((l) => (l?.at === agent.key ? null : l))}
        onDrop={(e) => {
          e.stopPropagation();
          drop(agent.key, line?.after ?? false);
        }}
      >
        {line?.at === agent.key && (
          <span className={`pointer-events-none absolute inset-x-1 h-0.5 rounded-full bg-accent ${line.after ? '-bottom-1' : '-top-1'}`} />
        )}
        <AgentMenu agent={agent} opens={opens} onShow={() => pick.onSelect(agent.key)} onSettings={() => pick.onSettings(agent.key)}>
          <AgentRowCard jobs={agent.background ?? []}>
            <Row
              opens={opens}
              space={agent.key}
              menu={false}
              title={nameOf(agent.key)}
              selected={pick.agent === agent.key}
              onShow={() => pick.onSelect(agent.key)}
              icon={<ActivityIcon act={activityOf(agent)} />}
              // Its queued tasks, and when it last worked: now while running, else since it stopped (state_since).
              trail={
                <span className="flex items-center gap-1.5">
                  {(queuedBehind(agent) ?? 0) > 0 && (
                    <span className="flex items-center gap-0.5" title="tasks queued">
                      <ListChecks size={12} />
                      {queuedBehind(agent)}
                    </span>
                  )}
                  {dotOf(agent) === 'running' ? 'now' : ago(agent.state_since, now)}
                </span>
              }
              // A mark that is not plain says why first; else the unread message.
              note={holdLine(agent) ?? unreadNote(agent)}
              unread={agent.founder_unread ?? 0}
            />
          </AgentRowCard>
        </AgentMenu>
      </DropArea>
    );
  });
}

/** A sidebar section under a full-width line: a heading (with an optional action), then its rows. */
export function Section({
  title,
  icon,
  action,
  accepts,
  onAgentDrop,
  folded,
  onFold,
  children,
}: {
  title: string;
  /** Folded: header only. onFold adds the chevron. */
  folded?: boolean;
  onFold?: () => void;
  icon?: ReactNode;
  action?: ReactNode;
  /** Whether an agent dragged from another section may land here, and what landing does. */
  accepts?: (name: string) => boolean;
  onAgentDrop?: (name: string) => void;
  children: ReactNode;
}) {
  const take = useDropTarget({
    accepts: () => !!dragged && !!accepts?.(dragged.name),
    onDrop: () => onAgentDrop?.(dragged!.name),
  });
  return (
    <section
      className={`-mx-2 flex flex-col gap-1 border-t border-border px-2 pt-3 transition-colors ${take.over ? 'bg-selected' : ''}`}
      {...take.handlers}
    >
      {/* Its buttons show while the pointer is on the heading or one has focus; their room is kept, so nothing moves. */}
      <div className="group flex h-7 items-center justify-between pl-2">
        <h2 className="flex items-center gap-1.5 text-xs font-medium text-muted">
          {onFold && (
            <button
              type="button"
              aria-label={folded ? `Unfold ${title}` : `Fold ${title}`}
              aria-expanded={!folded}
              onClick={onFold}
              className="-ml-1 rounded text-muted outline-none hover:text-fg focus-visible:text-fg"
            >
              {folded ? <ChevronRight size={12} /> : <ChevronDown size={12} />}
            </button>
          )}
          {icon}
          {title}
        </h2>
        <span className="opacity-0 group-hover:opacity-100 focus-within:opacity-100 [@media(hover:none)]:opacity-100">{action}</span>
      </div>
      {!folded && children}
    </section>
  );
}

/** The agents in sections: Pinned (when any are), then a flat list per workspace. */
function Agents({ pick, page, onNew }: { pick: Pick; page: string | null; onNew: (ws: Workspace) => void }) {
  const list = useAgents();
  const { nav, change, togglePin, toggleFold } = useNav();
  const first = list.data?.agents[0]?.key;
  // Nothing picked yet, neither agent nor page (a first visit): the first agent's workspace.
  useEffect(() => {
    if (!pick.agent && !page && first) pick.onSelect(first);
  }, [pick, page, first]);
  // A section's new order goes first in nav.order; the rest of the order stays as it was.
  const reorder = (names: string[]) => change({ order: [...names, ...nav.order.filter((n) => !names.includes(n))] });
  const { pinned, rest } = splitPinned(ordered(list.data?.agents ?? [], nav.order), nav.pins);
  // A section per workspace. With none, one Agents section still holds New agent, the loading rows and a dropped pin.
  const groups = byWorkspace(useDirectory(), rest);
  const sections = groups.length ? groups : [{ workspace: null, agents: [] }];
  return (
    <>
      {pinned.length > 0 && (
        <Section title="Pinned" icon={<Pin size={12} />} accepts={(name) => !nav.pins.includes(name)} onAgentDrop={togglePin}>
          <AgentRows
            agents={pinned}
            pick={pick}
            onMove={(names) => change({ pins: [...names, ...nav.pins.filter((n) => !names.includes(n))] })}
          />
        </Section>
      )}
      {sections.map(({ workspace, agents }, i) => (
        <Section
          key={workspace?.key ?? ''}
          title={workspace?.name ?? 'Agents'}
          // Dragging a pinned agent down here unpins it.
          accepts={(name) => nav.pins.includes(name)}
          onAgentDrop={togglePin}
          folded={!!workspace && nav.folded.includes(workspace.key)}
          onFold={workspace ? () => toggleFold(workspace.key) : undefined}
          action={
            workspace && (
              <IconButton label="New agent" onClick={() => onNew(workspace)}>
                <Plus size={14} />
              </IconButton>
            )
          }
        >
          {i === 0 && list.isPending && (
            // Shaped like the rows to come: status dot, name, time.
            <div aria-busy className="flex animate-pulse flex-col gap-1">
              {[55, 45, 60, 50, 40].map((w, j) => (
                <div key={j} className="flex h-7 items-center gap-1.5 px-2">
                  <span className="size-3 shrink-0 rounded-full bg-border" />
                  <span style={{ width: `${w}%` }} className="h-3 rounded bg-border" />
                  <span className="flex-1" />
                  <span className="h-2.5 w-6 rounded bg-border" />
                </div>
              ))}
            </div>
          )}
          {i === 0 && list.isError && <p className="px-2 text-sm text-danger">Agents did not load.</p>}
          <AgentRows agents={agents} pick={pick} onMove={reorder} here={workspace?.key} />
        </Section>
      ))}
      <div className="-mx-2 mt-1 border-t border-border">
        <button
          type="button"
          onClick={newWorkspace}
          className="flex h-9 w-full items-center gap-1.5 px-4 text-left text-sm text-muted outline-none hover:text-fg focus-visible:text-fg"
        >
          <Plus size={14} />
          Add workspace
        </button>
      </div>
    </>
  );
}

/** The pages (Tasks, Requests, Usage), then the agents by workspace. Picking a page or an agent brings up its own
 * workspace; dragging one opens its chat as a tab in the current one. Width is set by the splitter (see App). */
export function Sidebar({
  width,
  agent,
  page,
  onSelectPage,
  onSelectAgent,
  onAgentSettings,
}: {
  width: number;
  agent: string | null;
  page: string | null;
  onSelectPage: (kind: string, title: string) => void;
  onSelectAgent: (name: string) => void;
  onAgentSettings: (name: string) => void;
}) {
  const waiting = useRequests().count;
  // Only the agents scroll; the pages above and the buttons below stay put.
  const scroller = useRef<HTMLDivElement>(null);
  useKeptScroll(scroller, 'sidebar');
  const fade = useScrollFade(scroller);
  return (
    <aside style={{ width }} className="flex min-h-0 flex-1 flex-col gap-4 bg-surface px-2 pb-2">
      <nav className="flex flex-col gap-0.5">
        {pages.map((p) => (
          <Row
            key={p.kind}
            opens={p.kind}
            space={pageSpace(p.kind)}
            title={p.label}
            icon={p.icon}
            selected={page === p.kind}
            onShow={() => onSelectPage(p.kind, p.label)}
            trail={p.kind === 'requests' && waiting > 0 ? waiting : undefined}
          />
        ))}
      </nav>
      <div ref={scroller} style={fade} className="-mx-2 min-h-0 flex-1 overflow-y-auto px-2">
        <Agents
          page={page}
          pick={{ agent, onSelect: onSelectAgent, onSettings: onAgentSettings }}
          onNew={(ws) => void newAgent(ws).then((name) => name && onSelectAgent(name))}
        />
      </div>
      <SetupCard />
      <nav className="mt-auto flex gap-0.5 border-t border-border pt-2">
        {footerPages.map((p) => (
          <IconButton key={p.kind} label={p.label} onClick={() => onSelectPage(p.kind, p.label)}>
            {p.icon}
          </IconButton>
        ))}
      </nav>
    </aside>
  );
}

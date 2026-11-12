import { byWorkspace, dotOf, queuedBehind } from '@handline/app-core';
import { useAgents, useDirectory, useNameOf, useNav } from '@handline/app-data';
import { AgentMenu } from '#src/AgentMenu.tsx';
import { newAgent } from '#src/NewAgent.tsx';
import type { AgentView } from '@handline/client';
import { JobList, type Job } from '#src/chat/agent/BackgroundItem.tsx';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { ago, useNow } from '#src/format/time.ts';
import { ChevronLeft, ChevronRight, ListChecks, Pin, Plus, PanelLeft } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { activityOf, holdLine, ordered, splitPinned } from '#src/layout/agentList.ts';
import { chatOf, footerPages, pages } from '#src/layout/kinds.tsx';
import { useRequests } from '#src/requests/data.ts';
import { Row } from '#src/sidebar/Row.tsx';
import { SetupCard } from '#src/onboarding/SetupCard.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { useScrollFade } from '#src/ui/ScrollFade.tsx';
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
function AgentRows({ agents, pick, onMove }: { agents: AgentView[]; pick: Pick; onMove: (names: string[]) => void }) {
  const now = useNow();
  const nameOf = useNameOf();
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
      <div
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
        onDragOver={(e) => {
          if (!dragged || dragged.group !== group) return;
          e.preventDefault();
          e.stopPropagation();
          const r = e.currentTarget.getBoundingClientRect();
          setLine({ at: agent.key, after: e.clientY > r.top + Math.min(r.height, 32) / 2 });
        }}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as globalThis.Node | null)) setLine(null);
        }}
        onDrop={(e) => {
          if (!dragged || dragged.group !== group) return;
          e.preventDefault();
          e.stopPropagation();
          drop(agent.key, line?.after ?? false);
          setLine(null);
        }}
      >
        {line?.at === agent.key && (
          <span className={`pointer-events-none absolute inset-x-1 h-0.5 rounded-full bg-accent ${line.after ? '-bottom-1' : '-top-1'}`} />
        )}
        <AgentMenu agent={agent} opens={opens} onShow={() => pick.onSelect(agent.key)} onSettings={() => pick.onSettings(agent.key)}>
          <AgentRowCard jobs={agent.background ?? []}>
            <Row
              opens={opens}
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
      </div>
    );
  });
}

/** A sidebar section under a full-width line: a heading (with an optional action), then its rows. */
function Section({
  title,
  icon,
  action,
  accepts,
  onAgentDrop,
  children,
}: {
  title: string;
  icon?: ReactNode;
  action?: ReactNode;
  /** Whether an agent dragged from another section may land here, and what landing does. */
  accepts?: (name: string) => boolean;
  onAgentDrop?: (name: string) => void;
  children: ReactNode;
}) {
  const [over, setOver] = useState(false);
  return (
    <section
      className={`-mx-2 flex flex-col gap-1 border-t border-border px-2 pt-3 transition-colors ${over ? 'bg-selected' : ''}`}
      onDragOver={(e) => {
        if (!dragged || !accepts?.(dragged.name)) return;
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as globalThis.Node | null)) setOver(false);
      }}
      onDrop={(e) => {
        setOver(false);
        if (!dragged || !accepts?.(dragged.name)) return;
        e.preventDefault();
        onAgentDrop?.(dragged.name);
      }}
    >
      <div className="flex h-7 items-center justify-between pl-2">
        <h2 className="flex items-center gap-1.5 text-xs font-medium text-muted">
          {icon}
          {title}
        </h2>
        {action}
      </div>
      {children}
    </section>
  );
}

/** The agents in sections: Pinned (when any are), then a flat list per workspace. */
function Agents({ pick, page, onNew }: { pick: Pick; page: string | null; onNew: () => void }) {
  const list = useAgents();
  const { nav, change, togglePin } = useNav();
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
          action={
            i === 0 && (
              <IconButton label="New agent" onClick={onNew}>
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
          <AgentRows agents={agents} pick={pick} onMove={reorder} />
        </Section>
      ))}
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
          onNew={() => void newAgent().then((name) => name && onSelectAgent(name))}
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

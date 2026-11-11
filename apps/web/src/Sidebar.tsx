import { byWorkspace, dotOf, queuedBehind } from '@handline/app-core';
import { useAgents, useDirectory, useNameOf, useNav } from '@handline/app-data';
import { AgentMenu } from '#src/AgentMenu.tsx';
import { newAgent } from '#src/NewAgent.tsx';
import { AnimatePresence, motion } from 'framer-motion';
import { spring } from '#src/motion.ts';
import type { AgentView } from '@handline/client';
import { JobList, type Job } from '#src/chat/agent/BackgroundItem.tsx';
import { HoverCard } from '#src/ui/HoverCard.tsx';
import { ActivityIcon } from '#src/ui/activityIcon.tsx';
import { ago, useNow } from '#src/format/time.ts';
import { ChevronLeft, ChevronRight, ListChecks, LoaderCircle, Pin, Plus, PanelLeft } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { activityOf, holdLine, agentTree, pathTo, splitPinned, workingUnder, type AgentNode } from '#src/layout/agentTree.ts';
import { chatOf, footerPages, pages } from '#src/layout/kinds.tsx';
import { useRequests } from '#src/requests/data.ts';
import { Row } from '#src/sidebar/Row.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { useScrollFade } from '#src/ui/ScrollFade.tsx';
import { useFolded } from '#src/useFolded.ts';
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
  folded: ReadonlySet<string>;
  toggleFold: (name: string) => void;
};

/** The agent row being dragged and the sibling group it came from; a drop reorders only within that group. */
let dragged: { name: string; group: string } | null = null;

/** Rows of one sibling group. `onMove(name, names)` hears the group's new order after a drag within it. */
/** `under`: the agent drawn above this group; none at a section's top, where a pinned sub-agent names its parent. */
function AgentRows({
  nodes,
  pick,
  onMove,
  under = null,
}: {
  nodes: AgentNode<AgentView>[];
  pick: Pick;
  onMove: (names: string[]) => void;
  under?: string | null;
}) {
  const now = useNow();
  const nameOf = useNameOf();
  const names = nodes.map((n) => n.agent.key);
  const group = names.join('|');
  const [line, setLine] = useState<{ at: string; after: boolean } | null>(null);
  const drop = (target: string, after: boolean) => {
    if (!dragged || dragged.group !== group || dragged.name === target) return;
    const rest = names.filter((n) => n !== dragged!.name);
    const i = rest.indexOf(target) + (after ? 1 : 0);
    onMove([...rest.slice(0, i), dragged.name, ...rest.slice(i)]);
  };
  return nodes.map(({ agent, children }) => {
    const open = !pick.folded.has(agent.key);
    const opens = chatOf(agent.key);
    // Folded, a team still shows the branch to the picked agent.
    const shown = open ? children : pathTo(children, pick.agent);
    return (
      <div
        key={agent.key}
        className="relative flex flex-col rounded-lg border border-transparent has-[>div>div>[role=button]:hover]:border-border"
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
              // At a section's top its word (a lead is its workspace, a shared name says its workspace); under a parent its name.
              title={under === null ? nameOf(agent.key) : agent.name}
              context={under === null && agent.parent ? nameOf(agent.parent) : undefined}
              selected={pick.agent === agent.key}
              onShow={() => pick.onSelect(agent.key)}
              // Only a parent has the fold chevron; a leaf starts at its dot.
              lead={
                children.length > 0 && (
                  <span
                    className="grid size-3 shrink-0 place-items-center text-muted"
                    onClick={(e) => {
                      e.stopPropagation();
                      pick.toggleFold(agent.key);
                    }}
                  >
                    <ChevronRight size={12} className={`transition-transform duration-150 ${open ? 'rotate-90' : ''}`} />
                  </span>
                )
              }
              icon={<ActivityIcon act={activityOf(agent)} />}
              // When it last worked: now while running, else since it stopped (state_since).
              // how many below it are working (spinner and count), and its queued tasks.
              trail={
                <span className="flex items-center gap-1.5">
                  {workingUnder(children) > 0 && (
                    <span className="flex items-center gap-0.5 text-success" title="sub-agents working or starting">
                      <LoaderCircle size={11} strokeWidth={3.5} className="animate-spin" />
                      {workingUnder(children)}
                    </span>
                  )}
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
        <AnimatePresence initial={false}>
          {shown.length > 0 && (
            // Each level indents its whole block, so a hovered team's highlight starts at its own row.
            // Folding slides the block's height, clipped, so rows below move with it; the space above it is
            // padding inside, so it folds away with the height instead of jumping when the block goes.
            <motion.div
              key="children"
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: 'auto', opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={spring}
              className="overflow-hidden"
            >
              <div className="ml-3.5 flex flex-col gap-1 pt-1">
                <AgentRows nodes={shown} pick={pick} onMove={onMove} under={agent.key} />
              </div>
            </motion.div>
          )}
        </AnimatePresence>
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

/** The agents in sections: Pinned (when any are), each with its whole team, then a section per workspace. */
function Agents({ pick, page, onNew }: { pick: Pick; page: string | null; onNew: () => void }) {
  const list = useAgents();
  const { nav, change, togglePin } = useNav();
  const first = list.data?.agents[0]?.key;
  // Nothing picked yet, neither agent nor page (a first visit): the first agent's workspace.
  useEffect(() => {
    if (!pick.agent && !page && first) pick.onSelect(first);
  }, [pick, page, first]);
  // A sibling group's new order goes first in nav.order; the rest of the order stays as it was.
  const reorder = (names: string[]) => change({ order: [...names, ...nav.order.filter((n) => !names.includes(n))] });
  const { pinned, rest } = list.data ? splitPinned(agentTree(list.data.agents, nav.order), nav.pins) : { pinned: [], rest: [] };
  // The rest of the tree, a section per workspace: each top-level agent with the agents under it. With none, one Agents section still
  // holds New agent, the loading rows and a dropped pin.
  const nodeOf = new Map(rest.map((n) => [n.agent, n]));
  const groups = byWorkspace(
    useDirectory(),
    rest.map((n) => n.agent),
  ).map(({ workspace, agents }) => ({ workspace, nodes: agents.map((a) => nodeOf.get(a)!) }));
  const sections = groups.length ? groups : [{ workspace: null, nodes: [] }];
  return (
    <>
      {pinned.length > 0 && (
        <Section title="Pinned" icon={<Pin size={12} />} accepts={(name) => !nav.pins.includes(name)} onAgentDrop={togglePin}>
          <AgentRows
            nodes={pinned}
            pick={pick}
            // The pinned top level is the pins' order; a team inside one keeps the shared order.
            onMove={(names) =>
              names.every((n) => nav.pins.includes(n))
                ? change({ pins: [...names, ...nav.pins.filter((n) => !names.includes(n))] })
                : reorder(names)
            }
          />
        </Section>
      )}
      {sections.map(({ workspace, nodes }, i) => (
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
            // Shaped like the rows to come: status dot, name, time; some indented as sub-agents.
            <div aria-busy className="flex animate-pulse flex-col gap-1">
              {[
                [0, 55],
                [1, 45],
                [1, 60],
                [0, 50],
                [0, 40],
              ].map(([indent, w], j) => (
                <div key={j} className={`flex h-7 items-center gap-1.5 px-2 ${indent ? 'ml-3.5' : ''}`}>
                  <span className="size-3 shrink-0 rounded-full bg-border" />
                  <span style={{ width: `${w}%` }} className="h-3 rounded bg-border" />
                  <span className="flex-1" />
                  <span className="h-2.5 w-6 rounded bg-border" />
                </div>
              ))}
            </div>
          )}
          {i === 0 && list.isError && <p className="px-2 text-sm text-danger">Agents did not load.</p>}
          <AgentRows nodes={nodes} pick={pick} onMove={reorder} />
        </Section>
      ))}
    </>
  );
}

/** The pages (Tasks, Requests, Usage), then the agents as a tree. Picking a page or an agent brings up its own
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
  const fold = useFolded();
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
          pick={{ agent, onSelect: onSelectAgent, onSettings: onAgentSettings, ...fold }}
          onNew={() => void newAgent().then((name) => name && onSelectAgent(name))}
        />
      </div>
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

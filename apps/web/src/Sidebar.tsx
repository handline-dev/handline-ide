import { queuedBehind } from '@handline/app-core';
import { useAgents, useNav } from '@handline/app-data';
import { AgentMenu } from '#src/AgentMenu.tsx';
import { newAgent } from '#src/NewAgent.tsx';
import { AnimatePresence, motion } from 'framer-motion';
import { spring } from '#src/motion.ts';
import type { AgentView } from '@handline/client';
import { ago, useNow } from '#src/format/time.ts';
import { ChevronLeft, ChevronRight, Circle, CircleAlert, ListChecks, LoaderCircle, Pin, Plus, PanelLeft } from 'lucide-react';
import { useEffect, useRef, useState, type ReactNode } from 'react';
import {
  activityOf,
  agentTree,
  pathTo,
  splitPinned,
  shortName,
  workingUnder,
  type Activity,
  type AgentNode,
} from '#src/layout/agentTree.ts';
import { chatOf, footerPages, labelOf, pages } from '#src/layout/kinds.tsx';
import { startTabDrag } from '#src/ui/dnd.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { useScrollFade } from '#src/ui/ScrollFade.tsx';
import { useSpringOpen } from '#src/ui/springOpen.ts';
import { openProps, useOpenTab } from '#src/layout/openTab.tsx';
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

/** A sidebar row: click shows it in the workspace, drag drops it on a tab bar or a pane's edge. */
function Row({
  opens,
  title,
  icon,
  lead,
  selected = false,
  trail,
  note,
  context,
  unread = 0,
  onShow,
}: {
  opens: string;
  title: string;
  icon: ReactNode;
  lead?: ReactNode;
  selected?: boolean;
  /** Drawn at the right end, muted (a time). */
  trail?: ReactNode;
  /** A second line under the title, one line long (an unread message). */
  note?: string;
  /** Muted before the title: the team a pinned sub-agent belongs to. */
  context?: string;
  /** How many messages the note stands for, shown beside it when more than none. */
  unread?: number;
  onShow: () => void;
}) {
  const spring = useSpringOpen(onShow);
  const openTab = useOpenTab();
  return (
    <div
      {...spring.handlers}
      data-spring={spring.hovering || undefined}
      role="button"
      aria-current={selected || undefined}
      tabIndex={0}
      draggable
      onDragStart={(e) => startTabDrag(e, { open: { opens, title: labelOf(opens) } })}
      {...openProps((how) => (how === 'here' ? onShow() : openTab(opens, 'side')))}
      onKeyDown={(e) => {
        if (e.key !== 'Enter') return;
        e.preventDefault();
        if (e.metaKey || e.ctrlKey) openTab(opens, 'side');
        else onShow();
      }}
      className="group flex cursor-default flex-col rounded-md px-2 py-1 text-base text-fg outline-none select-none focus-visible:bg-hover aria-[current]:bg-selected data-[spring]:bg-hover"
    >
      <span className="flex h-5 items-center gap-1.5 group-aria-[current]:font-medium">
        {lead}
        <span className="grid size-4 shrink-0 place-items-center text-muted">{icon}</span>
        <span className="flex-1 truncate">
          {context && <span className="text-muted">{context} / </span>}
          {title}
        </span>
        {trail && <span className="shrink-0 text-xs text-muted tabular-nums">{trail}</span>}
      </span>
      {/* Under the title, from the icon's column on. */}
      {note && (
        <span className="flex items-center gap-1.5 pl-6 text-xs text-muted">
          <span className="flex-1 truncate">{note}</span>
          {unread > 0 && <span className="shrink-0 rounded-full bg-accent px-1.5 font-medium text-accent-fg tabular-nums">{unread}</span>}
        </span>
      )}
    </div>
  );
}

/** The newest message to the founder they have not read: one line of it, or nothing when all is read. */
function unreadNote(a: AgentView): string | undefined {
  if (!a.founder_unread || !a.last_to_founder) return undefined;
  return a.last_to_founder.text.replace(/\s+/g, ' ').trim();
}

const activityIcon: Record<Activity, ReactNode> = {
  // An arc turning at a steady size.
  working: <LoaderCircle size={14} strokeWidth={3.5} className="animate-spin text-success" aria-label="working" />,
  // Asleep is the hollow ring of the resting dot.
  asleep: <Circle size={12} strokeWidth={3.5} className="text-muted" aria-label="asleep" />,
  error: <CircleAlert size={16} className="text-danger" aria-label="needs attention" />,
  resting: <Circle size={12} className="fill-muted text-muted" aria-label="resting" />,
};

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
/** `under`: the agent drawn above this group, whose prefix the names drop; none at a section's top, so a
 * pinned legacy.caricat reads in full. */
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
  const names = nodes.map((n) => n.agent.name);
  const group = names.join('|');
  const [line, setLine] = useState<{ at: string; after: boolean } | null>(null);
  const drop = (target: string, after: boolean) => {
    if (!dragged || dragged.group !== group || dragged.name === target) return;
    const rest = names.filter((n) => n !== dragged!.name);
    const i = rest.indexOf(target) + (after ? 1 : 0);
    onMove([...rest.slice(0, i), dragged.name, ...rest.slice(i)]);
  };
  return nodes.map(({ agent, children }) => {
    const open = !pick.folded.has(agent.name);
    const opens = chatOf(agent.name);
    // Folded, a team still shows the branch to the picked agent.
    const shown = open ? children : pathTo(children, pick.agent);
    return (
      <div
        key={agent.name}
        className="relative flex flex-col rounded-lg border border-transparent has-[>div>[role=button]:hover]:border-border"
        onDragStart={(e) => {
          e.stopPropagation();
          dragged = { name: agent.name, group };
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
          setLine({ at: agent.name, after: e.clientY > r.top + Math.min(r.height, 32) / 2 });
        }}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as globalThis.Node | null)) setLine(null);
        }}
        onDrop={(e) => {
          if (!dragged || dragged.group !== group) return;
          e.preventDefault();
          e.stopPropagation();
          drop(agent.name, line?.after ?? false);
          setLine(null);
        }}
      >
        {line?.at === agent.name && (
          <span className={`pointer-events-none absolute inset-x-1 h-0.5 rounded-full bg-accent ${line.after ? '-bottom-1' : '-top-1'}`} />
        )}
        <AgentMenu agent={agent} onSettings={() => pick.onSettings(agent.name)}>
          <Row
            opens={opens}
            // At a section's top a sub-agent is a pin out of its team: its short name after the team's.
            title={under === null && agent.parent ? shortName(agent.name, agent.parent) : shortName(agent.name, under)}
            context={under === null && agent.parent ? agent.parent : undefined}
            selected={pick.agent === agent.name}
            onShow={() => pick.onSelect(agent.name)}
            // Only a parent has the fold chevron; a leaf starts at its dot.
            lead={
              children.length > 0 && (
                <span
                  className="grid size-3 shrink-0 place-items-center text-muted"
                  onClick={(e) => {
                    e.stopPropagation();
                    pick.toggleFold(agent.name);
                  }}
                >
                  <ChevronRight size={12} className={`transition-transform duration-150 ${open ? 'rotate-90' : ''}`} />
                </span>
              )
            }
            icon={activityIcon[activityOf(agent.state)]}
            // When it last worked: now while running, else since it stopped (state_since).
            // how many below it are working (spinner and count), and its queued tasks.
            trail={
              <span className="flex items-center gap-1.5">
                {workingUnder(children) > 0 && (
                  <span className="flex items-center gap-0.5 text-success" title="sub-agents working">
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
                {agent.state === 'running' ? 'now' : ago(agent.state_since, now)}
              </span>
            }
            note={unreadNote(agent)}
            unread={agent.founder_unread ?? 0}
          />
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
                <AgentRows nodes={shown} pick={pick} onMove={onMove} under={agent.name} />
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

/** The agents in two sections: Pinned (when any are), each with its whole team, then Agents, the rest of the tree. */
function Agents({ pick, page, onNew }: { pick: Pick; page: string | null; onNew: () => void }) {
  const list = useAgents();
  const { nav, change, togglePin } = useNav();
  const first = list.data?.agents[0]?.name;
  // Nothing picked yet, neither agent nor page (a first visit): the first agent's workspace.
  useEffect(() => {
    if (!pick.agent && !page && first) pick.onSelect(first);
  }, [pick, page, first]);
  // A sibling group's new order goes first in nav.order; the rest of the order stays as it was.
  const reorder = (names: string[]) => change({ order: [...names, ...nav.order.filter((n) => !names.includes(n))] });
  const { pinned, rest } = list.data ? splitPinned(agentTree(list.data.agents, nav.order), nav.pins) : { pinned: [], rest: [] };
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
      <Section
        title="Agents"
        // Dragging a pinned agent down here unpins it.
        accepts={(name) => nav.pins.includes(name)}
        onAgentDrop={togglePin}
        action={
          <IconButton label="New agent" onClick={onNew}>
            <Plus size={14} />
          </IconButton>
        }
      >
        {list.isPending && (
          // Shaped like the rows to come: status dot, name, time; some indented as sub-agents.
          <div aria-busy className="flex animate-pulse flex-col gap-1">
            {[
              [0, 55],
              [1, 45],
              [1, 60],
              [0, 50],
              [0, 40],
            ].map(([indent, w], i) => (
              <div key={i} className={`flex h-7 items-center gap-1.5 px-2 ${indent ? 'ml-3.5' : ''}`}>
                <span className="size-3 shrink-0 rounded-full bg-border" />
                <span style={{ width: `${w}%` }} className="h-3 rounded bg-border" />
                <span className="flex-1" />
                <span className="h-2.5 w-6 rounded bg-border" />
              </div>
            ))}
          </div>
        )}
        {list.isError && <p className="px-2 text-sm text-danger">Agents did not load.</p>}
        <AgentRows nodes={rest} pick={pick} onMove={reorder} />
      </Section>
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
  const scroller = useRef<HTMLElement>(null);
  useKeptScroll(scroller, 'sidebar');
  const fade = useScrollFade(scroller);
  return (
    <aside ref={scroller} style={{ width, ...fade }} className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto bg-surface px-2 pb-2">
      <nav className="flex flex-col gap-0.5">
        {pages.map((p) => (
          <Row
            key={p.kind}
            opens={p.kind}
            title={p.label}
            icon={p.icon}
            selected={page === p.kind}
            onShow={() => onSelectPage(p.kind, p.label)}
          />
        ))}
      </nav>
      <Agents
        page={page}
        pick={{ agent, onSelect: onSelectAgent, onSettings: onAgentSettings, ...fold }}
        onNew={() => void newAgent().then((name) => name && onSelectAgent(name))}
      />
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

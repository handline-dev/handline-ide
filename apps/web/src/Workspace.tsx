import { Chat } from '#src/chat/Chat.tsx';
import { Changes } from '#src/changes/Changes.tsx';
import { MarkdownTab } from '#src/tools/MarkdownTab.tsx';
import { TaskTab } from '#src/tools/TaskTab.tsx';
import { TasksTab } from '#src/tools/TasksTab.tsx';
import { WebTab } from '#src/tools/WebTab.tsx';
import { Usage } from '#src/usage/Usage.tsx';
import { PaneOpen, setShownWorkspace, type How } from '#src/layout/openTab.tsx';
import { ArrowDownToLine, ArrowLeftToLine, ArrowRightToLine, ArrowUpToLine, Columns2, LayoutGrid, X } from 'lucide-react';
import { useEffect, useRef, useState, type DragEvent, type ReactNode } from 'react';
import { canonical, choices, iconOf, labelOf } from '#src/layout/kinds.tsx';
import { toast } from '#src/ui/toast.tsx';
import { hasFixed, isOpen, panes, type Action, type Edge, type Node, type Pane, type Split, type Target } from '#src/layout/tree.ts';
import { Button } from '#src/ui/Button.tsx';
import { InsertLine, isTabDrag, readTabDrag, slotAt, startTabDrag, zoneAt, ZoneHighlight, type Zone } from '#src/ui/dnd.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { ContextMenu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { TabBar } from '#src/ui/Tabs.tsx';

export type Dispatch = (a: Action) => void;

type Context = { root: Node; agent: string | null; dispatch: Dispatch };

/** Applies a drop of any tab source (a tab moving, or a thing to open) at `to`. Something already open
 * here is not opened twice: its tab comes up, and a toast says why. */
function dropTo(e: DragEvent, to: Target, { root, agent, dispatch }: Context) {
  const drag = readTabDrag(e);
  if (!drag) return;
  e.preventDefault();
  if ('move' in drag) return dispatch({ type: 'move', from: drag.move.pane, tab: drag.move.tab, to });
  const opens = canonical(drag.open.opens, agent);
  if (isOpen(root, opens)) {
    toast(
      opens === 'chat'
        ? "This is this agent's own chat, already open here."
        : `${labelOf(opens, agent)} is already open here; an agent's chat cannot be open twice.`,
      { tone: 'warning' },
    );
    return dispatch({ type: 'show', opens, title: labelOf(opens, agent) });
  }
  dispatch({ type: 'open', opens, title: labelOf(opens, agent), to });
}

/** Opens `opens` from pane `from`: here, a tab in that pane; beside, a tab in the pane after it, or a new split to its right when
 * it is the last. Something already open is brought up where it is, not opened twice. */
function openFrom({ root, agent, dispatch }: Context, from: string, kind: string, how: How = 'here') {
  const opens = canonical(kind, agent);
  const title = labelOf(opens, agent);
  if (isOpen(root, opens)) return dispatch({ type: 'show', opens, title });
  if (how === 'here') return dispatch({ type: 'open', opens, title, to: { pane: from } });
  const all = panes(root);
  const next = all[all.findIndex((p) => p.id === from) + 1];
  dispatch({ type: 'open', opens, title, to: next ? { pane: next.id } : { pane: from, edge: 'right' } });
}

/** An Empty tab's body: pick what it shows, from the same list as the tab bar's +. */
function EmptyTab({ onPick }: { onPick: (kind: string) => void }) {
  return (
    <div className="m-auto flex flex-col items-center gap-3">
      <p className="text-sm text-muted">Open in this tab</p>
      <div className="flex w-60 flex-col gap-2">
        {choices.map((k) => (
          <Button key={k.kind} variant="soft" size="lg" className="justify-start" onClick={() => onPick(k.kind)}>
            {k.icon}
            {k.label}
          </Button>
        ))}
      </div>
    </div>
  );
}

/** The kinds of tab that draw something yet; the others (terminal, phones) stay off the + menu until they do. */
const BUILT = new Set(['diff', 'tasks', 'markdown', 'web']);

const splits: { edge: Edge | 'four'; label: string; icon: ReactNode }[] = [
  { edge: 'right', label: 'Split right', icon: <ArrowRightToLine size={14} /> },
  { edge: 'left', label: 'Split left', icon: <ArrowLeftToLine size={14} /> },
  { edge: 'down', label: 'Split down', icon: <ArrowDownToLine size={14} /> },
  { edge: 'up', label: 'Split up', icon: <ArrowUpToLine size={14} /> },
  { edge: 'four', label: 'Split in four', icon: <LayoutGrid size={14} /> },
];

/** Click splits the pane right; right-click picks where, or closes the pane. */
function SplitButton({ onSplit, onClose }: { onSplit: (edge: Edge | 'four') => void; onClose?: () => void }) {
  return (
    <ContextMenu
      items={
        <>
          {splits.map((s) => (
            <MenuItem key={s.edge} icon={s.icon} onClick={() => onSplit(s.edge)}>
              {s.label}
            </MenuItem>
          ))}
          {onClose && (
            <>
              <MenuSeparator />
              <MenuItem icon={<X size={14} />} onClick={onClose}>
                Close pane
              </MenuItem>
            </>
          )}
        </>
      }
    >
      <IconButton label="Split right (right-click for more)" onClick={() => onSplit('right')}>
        <Columns2 size={16} />
      </IconButton>
    </ContextMenu>
  );
}

/** One pane: its tab bar (a drop there inserts at the line) and its body (a drop at an edge splits there,
 * in the middle merges). */
function PaneView({
  root,
  pane,
  agent,
  dispatch,
  canClose,
  lead,
}: {
  root: Node;
  pane: Pane;
  agent: string | null;
  dispatch: Dispatch;
  canClose: boolean;
  lead?: ReactNode;
}) {
  const bar = useRef<HTMLDivElement>(null);
  const body = useRef<HTMLDivElement>(null);
  const [line, setLine] = useState<number | null>(null);
  const [zone, setZone] = useState<Zone | null>(null);
  const current = pane.tabs.find((t) => t.id === pane.active);
  const leave = (e: DragEvent, clear: () => void) => {
    if (!e.currentTarget.contains(e.relatedTarget as globalThis.Node | null)) clear();
  };

  return (
    <section className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex items-center border-b border-border bg-surface pr-2">
        {lead}
        <div
          ref={bar}
          className="relative min-w-0 flex-1 [&>[role=tablist]]:border-b-0"
          onDragOver={(e) => {
            if (!isTabDrag(e) || !bar.current) return;
            e.preventDefault();
            setLine(slotAt(e, bar.current).x);
          }}
          onDragLeave={(e) => leave(e, () => setLine(null))}
          onDrop={(e) => {
            setLine(null);
            if (bar.current) dropTo(e, { pane: pane.id, index: slotAt(e, bar.current).index }, { root, agent, dispatch });
          }}
        >
          <TabBar
            tabs={pane.tabs.map((t) => ({ ...t, title: t.opens ? labelOf(t.opens, agent) : t.title, icon: iconOf(t.opens) }))}
            active={pane.active}
            onSelect={(tab) => dispatch({ type: 'select', pane: pane.id, tab })}
            onClose={(tab) => dispatch({ type: 'closeTab', pane: pane.id, tab })}
            newTabKinds={agent ? choices.filter((c) => BUILT.has(c.kind)) : []}
            onAdd={agent ? (kind, how) => openFrom({ root, agent, dispatch }, pane.id, kind, how) : undefined}
            onTabDragStart={(tab, e) => startTabDrag(e, { move: { pane: pane.id, tab } })}
          />
          {line !== null && <InsertLine x={line} />}
        </div>
        {/* A page (Usage, Tasks) on its own has nothing to open beside it; an agent's workspace splits. */}
        {agent && (
          <SplitButton
            onSplit={(edge) => dispatch({ type: 'split', pane: pane.id, edge })}
            onClose={canClose ? () => dispatch({ type: 'closePane', pane: pane.id }) : undefined}
          />
        )}
      </div>
      <div
        ref={body}
        className="relative flex min-h-0 min-w-0 flex-1 bg-bg"
        onDragOver={(e) => {
          if (!isTabDrag(e) || !body.current) return;
          e.preventDefault();
          setZone(zoneAt(e, body.current));
        }}
        onDragLeave={(e) => leave(e, () => setZone(null))}
        onDrop={(e) => {
          setZone(null);
          if (!body.current) return;
          const z = zoneAt(e, body.current);
          dropTo(e, z === 'center' ? { pane: pane.id } : { pane: pane.id, edge: z }, { root, agent, dispatch });
        }}
      >
        <PaneOpen.Provider value={(opens, how) => openFrom({ root, agent, dispatch }, pane.id, opens, how)}>
          {current?.opens === 'chat' && agent && <Chat key={agent} agent={agent} />}
          {current?.opens?.startsWith('chat:') && <Chat key={current.opens} agent={current.opens.slice(5)} />}
          {current?.opens === 'diff' && agent && <Changes key={agent} agent={agent} />}
          {current?.opens === 'markdown' && agent && <MarkdownTab key={current.id} agent={agent} tab={current.id} />}
          {current?.opens === 'web' && <WebTab key={current.id} tab={current.id} />}
          {current?.opens?.startsWith('web:') && <WebTab key={current.id} tab={current.id} start={current.opens.slice(4)} />}
          {current?.opens?.startsWith('task:') && <TaskTab key={current.opens} id={Number(current.opens.slice(5))} />}
          {current?.opens?.startsWith('file:') && agent && (
            <MarkdownTab key={current.id} agent={agent} tab={current.id} start={current.opens.slice(5)} />
          )}
          {current?.opens === 'usage' && <Usage />}
          {current?.opens === 'tasks' && <TasksTab agent={agent ?? null} />}
        </PaneOpen.Provider>
        {current && !current.opens && (
          <EmptyTab onPick={(kind) => dispatch({ type: 'choose', pane: pane.id, tab: current.id, opens: kind, title: labelOf(kind) })} />
        )}
        {zone && <ZoneHighlight zone={zone} />}
      </div>
    </section>
  );
}

function SplitView({ node, dispatch, children }: { node: Split; dispatch: Dispatch; children: [ReactNode, ReactNode] }) {
  const box = useRef<HTMLDivElement>(null);
  const x = node.axis === 'x';
  const resize = (ratio: number) => dispatch({ type: 'resize', split: node.id, ratio });
  return (
    <div ref={box} className={`flex min-h-0 min-w-0 flex-1 ${x ? 'flex-row' : 'flex-col'}`}>
      <div className="flex min-h-0 min-w-0" style={{ flex: `${node.ratio} 1 0` }}>
        {children[0]}
      </div>
      <Splitter
        axis={node.axis}
        container={box}
        percent={node.ratio * 100}
        onMove={(o, size) => resize(o / size)}
        onStep={(d) => resize(node.ratio + d * 0.02)}
        label={x ? 'Resize columns' : 'Resize rows'}
      />
      <div className="flex min-h-0 min-w-0" style={{ flex: `${1 - node.ratio} 1 0` }}>
        {children[1]}
      </div>
    </div>
  );
}

/** The view of the layout tree (src/layout/tree.ts); every change is an Action through its reducer.
 * `lead` goes at the start of the top-left pane's tab bar (the sidebar controls' room when it is closed). */
export function Workspace({ root, agent, dispatch, lead }: { root: Node; agent: string | null; dispatch: Dispatch; lead?: ReactNode }) {
  const render = (node: Node, topLeft: boolean): ReactNode =>
    node.kind === 'pane' ? (
      <PaneView
        key={node.id}
        root={root}
        pane={node}
        agent={agent}
        dispatch={dispatch}
        canClose={root.kind === 'split' && !hasFixed(node)}
        lead={topLeft ? lead : undefined}
      />
    ) : (
      <SplitView key={node.id} node={node} dispatch={dispatch}>
        {[render(node.a, topLeft), render(node.b, false)]}
      </SplitView>
    );
  // Opened from outside a pane (the sidebar): from the first pane.
  useEffect(() => setShownWorkspace((opens, how) => openFrom({ root, agent, dispatch }, panes(root)[0]!.id, opens, how)));
  return <div className="flex min-h-0 min-w-0 flex-1">{render(root, true)}</div>;
}

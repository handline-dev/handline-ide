import { TabId } from '#src/persist/tabState.ts';
import { Chat } from '#src/chat/Chat.tsx';
import { Changes } from '#src/changes/Changes.tsx';
import { MarkdownTab } from '#src/tools/MarkdownTab.tsx';
import { TaskTab } from '#src/tools/TaskTab.tsx';
import { TasksTab } from '#src/tools/TasksTab.tsx';
import { RequestsTab } from '#src/requests/RequestsTab.tsx';
import { ShellTab, TerminalTab } from '#src/tools/TerminalTab.tsx';
import { useNameOf, useWrite } from '@handline/app-data';
import { WebTab } from '#src/tools/WebTab.tsx';
import { Usage } from '#src/usage/Usage.tsx';
import { PaneOpen, setShownWorkspace, type How } from '#src/layout/openTab.tsx';
import { ArrowDownToLine, ArrowLeftToLine, ArrowRightToLine, ArrowUpToLine, Columns2, LayoutGrid, X } from 'lucide-react';
import { useCallback, useEffect, useRef, useState, type DragEvent, type ReactNode } from 'react';
import { Boundary, brokenView } from '#src/ui/Boundary.tsx';
import { canonical, choices, iconOf, labelOf, shellHandle, shellOf } from '#src/layout/kinds.tsx';
import { toast } from '#src/ui/toast.tsx';
import {
  hasFixed,
  isOpen,
  panes,
  whereTo,
  type Action,
  type Edge,
  type Node,
  type Pane,
  type Split,
  type Target,
} from '#src/layout/tree.ts';
import { Button } from '#src/ui/Button.tsx';
import { InsertLine, isTabDrag, readTabDrag, slotAt, startTabDrag, zoneAt, ZoneHighlight, type Zone } from '#src/ui/dnd.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { ContextMenu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { TabDot } from '#src/layout/TabDot.tsx';
import { TabBar } from '#src/ui/Tabs.tsx';
import { tabTarget, type TabTarget } from '@handline/app-core';

/** The tab on screen is read as it shows, so only a background tab gets a dot. */
const dotOf = (target: TabTarget) => (target ? <TabDot target={target} /> : null);

export type Dispatch = (a: Action) => void;

type Context = { root: Node; agent: string | null; dispatch: Dispatch; nameOf: (key: string) => string };

/** Applies a drop of any tab source (a tab moving, or a thing to open) at `to`. Something already open
 * here is not opened twice: its tab comes up, and a toast says why. */
function dropTo(e: DragEvent, to: Target, { root, agent, dispatch, nameOf }: Context) {
  const drag = readTabDrag(e);
  if (!drag) return;
  e.preventDefault();
  if ('move' in drag) return dispatch({ type: 'move', from: drag.move.pane, tab: drag.move.tab, to });
  const opens = canonical(drag.open.opens, agent);
  // A tab already open moves to where it was dropped; a pinned one stays and is shown.
  for (const p of panes(root)) {
    const tab = p.tabs.find((t) => t.opens === opens);
    if (tab && tab.closable !== false) return dispatch({ type: 'move', from: p.id, tab: tab.id, to });
  }
  if (isOpen(root, opens)) {
    toast(opens === 'chat' ? "This is this agent's own chat, already open here." : `${labelOf(opens, nameOf)} is already open here.`, {
      tone: 'warning',
    });
    return dispatch({ type: 'show', opens, title: labelOf(opens, nameOf) });
  }
  dispatch({ type: 'open', opens, title: labelOf(opens, nameOf), to });
}

/** Opens `opens` from pane `from`, where `how` asks (whereTo). Something already open is brought up where it is, not opened twice. */
function openFrom({ root, agent, dispatch, nameOf }: Context, from: string, kind: string, how: How = 'here') {
  const opens = canonical(kind, agent);
  const title = labelOf(opens, nameOf);
  if (isOpen(root, opens)) return dispatch({ type: 'show', opens, title });
  dispatch({ type: 'open', opens, title, to: whereTo(root, from, how) });
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
const BUILT = new Set(['newshell', 'diff', 'tasks', 'markdown', 'web']);

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

/** The pane the person last clicked or focused in: Cmd+W closes its tab. */
let lastPane: string | null = null;

const MAC = /Mac|iPhone|iPad/.test(globalThis.navigator?.platform ?? '');
/** Cmd+W on a Mac, Ctrl+W elsewhere; in a terminal there Ctrl+W deletes a word, so it is Ctrl+Shift+W. */
function isCloseChord(e: KeyboardEvent): boolean {
  if (e.key.toLowerCase() !== 'w' || e.altKey) return false;
  if (MAC) return e.metaKey && !e.ctrlKey && !e.shiftKey;
  const inTerminal = e.target instanceof HTMLElement && !!e.target.closest('.xterm');
  return e.ctrlKey && !e.metaKey && e.shiftKey === inTerminal;
}
/** Cmd+Shift+T on a Mac, Ctrl+Shift+T elsewhere: Reopen Closed Tab. */
const isReopenChord = (e: KeyboardEvent) =>
  e.key.toLowerCase() === 't' && e.shiftKey && !e.altKey && (MAC ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey);

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
  const nameOf = useNameOf();
  const bar = useRef<HTMLDivElement>(null);
  const body = useRef<HTMLDivElement>(null);
  const [line, setLine] = useState<number | null>(null);
  const [zone, setZone] = useState<Zone | null>(null);
  const current = pane.tabs.find((t) => t.id === pane.active);
  const closeShells = useCloseShells(agent);
  const close = (ids: string[]) => {
    closeShells(pane.tabs.filter((t) => ids.includes(t.id)));
    ids.forEach((tab) => dispatch({ type: 'closeTab', pane: pane.id, tab }));
  };
  const openShell = useWrite('shell.open', agent ?? undefined);
  /** Places what was picked; a new Terminal first opens its shell, so the tab is made with its handle. */
  const pick = (kind: string, place: (opens: string) => void) => {
    if (kind !== 'newshell') return place(kind);
    if (agent)
      openShell
        .mutateAsync({ agent })
        .then((r) => place(shellOf(r.terminal)))
        .catch((e: Error) => toast(`The terminal did not open: ${e.message}`, { tone: 'warning' }));
  };
  const leave = (e: DragEvent, clear: () => void) => {
    if (!e.currentTarget.contains(e.relatedTarget as globalThis.Node | null)) clear();
  };

  return (
    <section
      data-pane={pane.id}
      onPointerDownCapture={() => (lastPane = pane.id)}
      onFocusCapture={() => (lastPane = pane.id)}
      className="flex min-h-0 min-w-0 flex-1 flex-col"
    >
      <div className="app-drag flex items-center border-b border-border bg-surface pr-2">
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
            if (bar.current) dropTo(e, { pane: pane.id, index: slotAt(e, bar.current).index }, { root, agent, dispatch, nameOf });
          }}
        >
          <TabBar
            tabs={pane.tabs.map((t) => ({
              ...t,
              title: t.opens ? labelOf(t.opens, nameOf) : t.title,
              icon: iconOf(t.opens),
              dot: t.id === pane.active ? null : dotOf(tabTarget(t.opens, agent)),
            }))}
            active={pane.active}
            onSelect={(tab) => dispatch({ type: 'select', pane: pane.id, tab })}
            onClose={(tab) => close([tab])}
            onCloseTabs={close}
            newTabKinds={agent ? choices.filter((c) => BUILT.has(c.kind)) : []}
            onAdd={
              agent ? (kind, how) => pick(kind, (opens) => openFrom({ root, agent, dispatch, nameOf }, pane.id, opens, how)) : undefined
            }
            onTabDragStart={(tab, e) => startTabDrag(e, { move: { pane: pane.id, tab } })}
          />
          {line !== null && <InsertLine x={line} />}
        </div>
        {/* A page (Usage, Tasks) on its own has nothing to open beside it; an agent's workspace splits. */}
        {agent && (
          <SplitButton
            onSplit={(edge) => dispatch({ type: 'split', pane: pane.id, edge })}
            onClose={
              canClose
                ? () => {
                    closeShells(pane.tabs);
                    dispatch({ type: 'closePane', pane: pane.id });
                  }
                : undefined
            }
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
          dropTo(e, z === 'center' ? { pane: pane.id } : { pane: pane.id, edge: z }, { root, agent, dispatch, nameOf });
        }}
      >
        <Boundary key={`${current?.id}:${current?.opens}`} loading={null} failed={brokenView('This tab')}>
          <TabId.Provider value={current?.id ?? null}>
            <PaneOpen.Provider value={(opens, how) => openFrom({ root, agent, dispatch, nameOf }, pane.id, opens, how)}>
              {current?.opens === 'chat' && agent && <Chat key={agent} agent={agent} />}
              {current?.opens?.startsWith('chat:') && <Chat key={current.opens} agent={current.opens.slice(5)} />}
              {current?.opens === 'diff' && agent && <Changes key={agent} agent={agent} />}
              {current?.opens === 'shell' && agent && <TerminalTab key={agent} agent={agent} />}
              {shellHandle(current?.opens ?? '') && agent && (
                <ShellTab key={current!.id} agent={agent} handle={shellHandle(current!.opens!)!} />
              )}
              {current?.opens === 'markdown' && agent && <MarkdownTab key={current.id} agent={agent} tab={current.id} />}
              {current?.opens === 'web' && <WebTab key={current.id} tab={current.id} agent={agent} />}
              {current?.opens?.startsWith('web:') && (
                <WebTab key={current.id} tab={current.id} agent={agent} start={current.opens.slice(4)} />
              )}
              {current?.opens?.startsWith('task:') && <TaskTab key={current.opens} id={Number(current.opens.slice(5))} />}
              {current?.opens?.startsWith('file:') && agent && (
                <MarkdownTab key={current.id} agent={agent} tab={current.id} start={current.opens.slice(5)} />
              )}
              {current?.opens === 'usage' && <Usage />}
              {current?.opens === 'requests' && <RequestsTab />}
              {current?.opens === 'tasks' && <TasksTab agent={agent ?? null} />}
            </PaneOpen.Provider>
          </TabId.Provider>
        </Boundary>
        {current && !current.opens && (
          <EmptyTab
            onPick={(kind) =>
              pick(kind, (opens) => dispatch({ type: 'choose', pane: pane.id, tab: current.id, opens, title: labelOf(opens) }))
            }
          />
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
export function Workspace({
  root,
  agent,
  dispatch,
  reopenClosed,
  lead,
}: {
  root: Node;
  agent: string | null;
  dispatch: Dispatch;
  /** Opens the last closed tab again; a tab whose pane is gone goes to this pane. */
  reopenClosed: (pane: string) => void;
  lead?: ReactNode;
}) {
  const nameOf = useNameOf();
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
  const closeShells = useCloseShells(agent);
  // Cmd+W (Ctrl+W elsewhere) closes the tab on screen in the pane last used, Cmd+Shift+T opens the last closed again; the desktop
  // shell's menu items send the same keys.
  useEffect(() => {
    const close = (e: KeyboardEvent) => {
      if (!isCloseChord(e) || document.querySelector('[role="dialog"], [role="alertdialog"]')) return; // nothing closes behind a modal
      const all = panes(root);
      const p = all.find((x) => x.id === lastPane) ?? all[0]!;
      e.preventDefault();
      closeShells(p.tabs.filter((t) => t.id === p.active));
      dispatch({ type: 'closeTab', pane: p.id, tab: p.active });
    };
    const reopen = (e: KeyboardEvent) => {
      if (!isReopenChord(e) || document.querySelector('[role="dialog"], [role="alertdialog"]')) return;
      e.preventDefault();
      reopenClosed(lastPane ?? panes(root)[0]!.id);
    };
    addEventListener('keydown', reopen);
    addEventListener('keydown', close);
    return () => {
      removeEventListener('keydown', close);
      removeEventListener('keydown', reopen);
    };
  }, [root, dispatch, closeShells, reopenClosed]);
  // A drag region takes no drop: while a tab is dragged it steps aside, so a drop on the tab bar's free space still lands.
  useEffect(() => {
    const on = () => (document.body.dataset.dragging = '');
    const off = () => delete document.body.dataset.dragging;
    addEventListener('dragstart', on, true);
    addEventListener('dragend', off, true);
    addEventListener('drop', off, true);
    return () => {
      removeEventListener('dragstart', on, true);
      removeEventListener('dragend', off, true);
      removeEventListener('drop', off, true);
    };
  }, []);
  // Opened from outside a pane (the sidebar): from the first pane.
  useEffect(() => setShownWorkspace((opens, how) => openFrom({ root, agent, dispatch, nameOf }, panes(root)[0]!.id, opens, how)));
  return <div className="flex min-h-0 min-w-0 flex-1">{render(root, true)}</div>;
}

/** Closes the shells of tabs being closed (a pinned tab never closes). */
function useCloseShells(agent: string | null) {
  const { mutate } = useWrite('shell.close', agent ?? undefined);
  return useCallback(
    (tabs: { opens?: string; closable?: boolean }[]) => {
      for (const t of tabs) {
        const h = t.closable !== false && shellHandle(t.opens ?? '');
        if (h) mutate({ terminal: h });
      }
    },
    [mutate],
  );
}

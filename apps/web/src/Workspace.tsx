import { WorkTab } from '#src/chat/agent/WorkTab.tsx';
import { MAC, primaryMod } from '#src/ui/keys.ts';
import { Chat } from '#src/chat/Chat.tsx';
import { Changes } from '#src/changes/Changes.tsx';
import { MarkdownTab } from '#src/tools/MarkdownTab.tsx';
import { TaskTab } from '#src/tools/TaskTab.tsx';
import { SpaceCluster, TabScope, spaceOf, tabIn } from '#src/connection/clusters.tsx';
import { PlanTab } from '#src/tools/Story.tsx';
import { CodeTab } from '#src/tools/TaskCode.tsx';
import { useTabKeys } from '#src/layout/tabKeys.ts';
import { DocTab } from '#src/tools/DocTab.tsx';
import { ProposalView } from '#src/requests/ProposalView.tsx';
import { AgentPage } from '#src/settings/AgentPage.tsx';
import { TextTab } from '#src/tools/TextTab.tsx';
import { TasksTab } from '#src/tools/TasksTab.tsx';
import { ShellTab, TerminalTab } from '#src/tools/TerminalTab.tsx';
import { useAgents, useNameOf, useWrite } from '@handline/app-data';
import { WebTab } from '#src/tools/WebTab.tsx';
import { WorkspacePage } from '#src/workspace/WorkspacePage.tsx';
import { PageTab } from '#src/layout/pageTab.tsx';
import { PaneOpen, setShownWorkspace, type How } from '#src/layout/openTab.tsx';
import { ArrowDownToLine, ArrowLeftToLine, ArrowRightToLine, ArrowUpToLine, Columns2, LayoutGrid, X } from 'lucide-react';
import { useDropTarget } from '#src/ui/dropTarget.tsx';
import { useCallback, useContext, useEffect, useRef, type DragEvent, type ReactNode } from 'react';
import { Boundary, brokenView } from '#src/ui/Boundary.tsx';
import { agentKind, choices, iconOf, pages, labelOf, opened, ownedBy, shellHandle, shellOf, withAgent } from '#src/layout/kinds.tsx';
import { toast } from '#src/ui/toast.tsx';
import { isOpen, panes, whereTo, type Action, type Edge, type Node, type Pane, type Split, type Target } from '#src/layout/tree.ts';
import { Button } from '#src/ui/Button.tsx';
import { isTabDrag, readTabDrag, slotAt, startTabDrag, zoneAt, showZone, showSlot, DROP_AREA } from '#src/ui/dnd.tsx';
import { IconButton } from '#src/ui/IconButton.tsx';
import { ContextMenu, MenuItem, MenuSeparator } from '#src/ui/Menu.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { dotOf } from '#src/layout/TabDot.tsx';
import { agentOf, carryIn } from '#src/layout/screenLayout.ts';
import { TabBar } from '#src/ui/Tabs.tsx';

export type Dispatch = (a: Action) => void;

/** Names as the agent's workspace reads them: its own agents by name alone (T9201). */
const useNamesHere = (agent: string | null) => useNameOf(useAgents().data?.agents.find((a) => a.key === agent)?.workspace);

/** `agent`: the agent's key with its cluster (`@c/bob`), the one an agent's kind asked for bare is opened naming. */
type Context = { root: Node; agent: string | null; dispatch: Dispatch; nameOf: (key: string) => string; focus: (pane: string) => void };

/** Applies a drop of any tab source (a tab moving, or a thing to open) at `to`. Something already open
 * here is not opened twice: its tab comes up, and a toast says why. */
function dropTo(e: DragEvent, to: Target, { root, agent, dispatch, nameOf }: Context) {
  const drag = readTabDrag(e);
  if (!drag) return;
  e.preventDefault();
  if ('move' in drag) return dispatch({ type: 'move', from: drag.move.pane, tab: drag.move.tab, tabs: drag.move.tabs, to });
  const opens = carryIn(drag.open.opens, agent);
  // A tab already open, pinned or not, moves to where it was dropped.
  for (const p of panes(root)) {
    const tab = p.tabs.find((t) => t.opens === opens);
    if (tab) return dispatch({ type: 'move', from: p.id, tab: tab.id, to });
  }
  if (isOpen(root, opens)) {
    toast(`${labelOf(opens, nameOf)} is already open here.`, { tone: 'warning' });
    return dispatch({ type: 'show', opens, title: labelOf(opens, nameOf) });
  }
  dispatch({ type: 'open', opens, title: labelOf(opens, nameOf), to });
}

/** Opens `opens` from pane `from`, where `how` asks (whereTo). Something already open is brought up where it is, not opened twice.
 * An agent's kind asked for bare (the chat's Agent terminal button) is opened naming the agent it was asked from: a tab that named
 * none would draw with whichever agent the focused tab shows, and once it is the focused tab itself, with none, and go blank. */
export function openFrom({ root, agent, dispatch, nameOf, focus }: Context, from: string, bare: string, how: How = 'here') {
  const opens = carryIn(bare, agent);
  const title = labelOf(opens, nameOf);
  // Open already (and asked for here): its pane takes the focus. Beside or below always opens a new pane there.
  const at = how === 'here' ? panes(root).find((p) => p.tabs.some((t) => t.opens === opens)) : undefined;
  if (at) {
    dispatch({ type: 'show', opens, title });
    return focus(at.id);
  }
  dispatch({ type: 'open', opens, title, to: whereTo(root, from, how) });
}

/** An Empty tab's body: pick what it shows, the pages first, then the tab bar's + list. */
function EmptyTab({ onPick }: { onPick: (kind: string) => void }) {
  return (
    <div className="m-auto flex flex-col items-center gap-3">
      <p className="text-sm text-muted">Open in this tab</p>
      <div className="flex w-60 flex-col gap-2">
        {[...pages, ...choices.filter((c) => c.kind !== 'tasks')].map((k) => (
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

/** Cmd+W on a Mac, Ctrl+W elsewhere; in a terminal there Ctrl+W deletes a word, so it is Ctrl+Shift+W. */
function isCloseChord(e: KeyboardEvent): boolean {
  if (e.key.toLowerCase() !== 'w' || e.altKey) return false;
  if (MAC) return e.metaKey && !e.ctrlKey && !e.shiftKey;
  const inTerminal = e.target instanceof HTMLElement && !!e.target.closest('.xterm');
  return e.ctrlKey && !e.metaKey && e.shiftKey === inTerminal;
}
/** Cmd+Shift+T on a Mac, Ctrl+Shift+T elsewhere: Reopen Closed Tab. */
const isReopenChord = (e: KeyboardEvent) => e.key.toLowerCase() === 't' && e.shiftKey && !e.altKey && primaryMod(e);

/** One pane: its tab bar (a drop there inserts at the line) and its body (a drop at an edge splits there,
 * in the middle merges). */
function PaneView({
  root,
  pane,
  agent,
  dispatch,
  focused,
  onFocus,
  lead,
}: {
  root: Node;
  pane: Pane;
  /** The agent the focused tab shows: the one a new tab's agent kind is for when this pane's own tab names none. */
  agent: string | null;
  dispatch: Dispatch;
  /** Whether this pane is the focused one; the others draw in the unfocused colours when there are several. */
  focused: boolean;
  onFocus: (pane: string) => void;
  lead?: ReactNode;
}) {
  const bar = useRef<HTMLDivElement>(null);
  const body = useRef<HTMLDivElement>(null);
  const { tab: current, cluster: there } = tabIn(pane.tabs.find((t) => t.id === pane.active));
  // An agent's tab (its chat, terminal, changes) names it; another tab draws with the agent of the focused one.
  const own = ownedBy(current?.opens ?? '');
  const shown = own?.kind ?? current?.opens;
  const who = agentOf(current?.opens) ?? agent;
  // `who` with its cluster: this tab's own when it names the agent, else the workspace's (the focused tab's).
  const workspaceCluster = useContext(SpaceCluster);
  const whoKey = who && spaceOf(agentOf(current?.opens) ? there : workspaceCluster, who);
  const nameOf = useNamesHere(who);
  const closeShells = useCloseShells(who);
  const close = (ids: string[]) => {
    closeShells(pane.tabs.filter((t) => ids.includes(t.id)));
    ids.forEach((tab) => dispatch({ type: 'closeTab', pane: pane.id, tab }));
  };
  const openShell = useWrite('shell.open');
  /** Places what was picked, for `of` (a page's workspace asks which agent); a new Terminal first opens its shell, so the tab is made
   * with its handle. */
  const pick = (kind: string, place: (opens: string) => void, of = who) => {
    const named = (k: string) => (of && agentKind(k) ? withAgent(k, of) : k);
    if (kind !== 'newshell') return place(named(kind));
    if (of)
      openShell
        .mutateAsync({ agent: of })
        .then((r) => place(named(shellOf(r.terminal))))
        .catch((e: Error) => toast(`The terminal did not open: ${e.message}`, { tone: 'warning' }));
  };
  // With no agent on screen an agent's kind asks which agent.
  const agents = useAgents().data?.agents;
  const agentChoices = agents?.map((a) => ({ key: a.key, label: nameOf(a.key) }));
  // A tab, or a thing to open, dropped among the tabs or on the pane (its middle, or an edge to split there).
  const onBar = useDropTarget({
    accepts: isTabDrag,
    onOver: (e) => {
      if (!bar.current) return;
      onFocus(pane.id);
      showSlot(bar.current, slotAt(e, bar.current).x);
    },
    onDrop: (e) =>
      bar.current &&
      dropTo(e, { pane: pane.id, index: slotAt(e, bar.current).index }, { root, agent: whoKey, dispatch, nameOf, focus: onFocus }),
  });
  const onBody = useDropTarget({
    accepts: isTabDrag,
    onOver: (e) => {
      if (!body.current) return;
      onFocus(pane.id); // the pane a tab is carried over is the one in focus
      showZone(body.current, zoneAt(e, body.current));
    },
    onDrop: (e) => {
      if (!body.current) return;
      const z = zoneAt(e, body.current);
      dropTo(e, z === 'center' ? { pane: pane.id } : { pane: pane.id, edge: z }, { root, agent: whoKey, dispatch, nameOf, focus: onFocus });
    },
  });

  return (
    <section
      data-pane={pane.id}
      onPointerDownCapture={() => onFocus(pane.id)}
      onFocusCapture={() => onFocus(pane.id)}
      className={`flex min-h-0 min-w-0 flex-1 flex-col bg-bg text-fg ${focused || root.kind === 'pane' ? '' : 'unfocused'}`}
    >
      <div className="app-drag flex items-center border-b border-border bg-surface pr-2">
        {lead}
        <div ref={bar} className="relative min-w-0 flex-1 [&>[role=tablist]]:border-b-0" {...onBar.handlers}>
          <TabBar
            tabs={pane.tabs.map((t) => ({
              ...t,
              title: t.opens ? labelOf(t.opens, nameOf) : t.title,
              icon: iconOf(t.opens),
              dot: t.id === pane.active ? null : dotOf(t.opens, who),
              // The screen's last Empty tab is what closing leaves: it has no close to offer.
              closable: !(root.kind === 'pane' && pane.tabs.length === 1 && !t.opens),
            }))}
            active={pane.active}
            onSelect={(tab) => dispatch({ type: 'select', pane: pane.id, tab })}
            onClose={(tab) => close([tab])}
            onCloseTabs={close}
            newTabKinds={choices
              .filter((c) => BUILT.has(c.kind))
              .flatMap((c) => (who || !agentKind(c.kind) ? [c] : agentChoices ? [{ ...c, agents: agentChoices }] : []))}
            onAdd={(kind, how, of) =>
              pick(kind, (opens) => openFrom({ root, agent: whoKey, dispatch, nameOf, focus: onFocus }, pane.id, opens, how), of ?? who)
            }
            onTabDragStart={(tab, tabs, e) => startTabDrag(e, { move: { pane: pane.id, tab, tabs } })}
          />
        </div>
        <SplitButton
          onSplit={(edge) => dispatch({ type: 'split', pane: pane.id, edge })}
          onClose={
            root.kind === 'split'
              ? () => {
                  closeShells(pane.tabs);
                  dispatch({ type: 'closePane', pane: pane.id });
                }
              : undefined
          }
        />
      </div>
      <div ref={body} className="relative flex min-h-0 min-w-0 flex-1 bg-bg" {...onBody.handlers}>
        <Boundary key={`${current?.id}:${current?.opens}`} loading={null} failed={brokenView('This tab')}>
          <TabScope id={current?.id ?? null} cluster={there}>
            <PaneOpen.Provider
              value={(opens, how) => openFrom({ root, agent: whoKey, dispatch, nameOf, focus: onFocus }, pane.id, opens, how)}
            >
              {current?.opens?.startsWith('chat:') && <Chat key={current.opens} agent={current.opens.slice(5)} />}
              {shown === 'diff' && who && <Changes key={who} agent={who} />}
              {shown === 'shell' && who && <TerminalTab key={who} agent={who} />}
              {shellHandle(shown ?? '') && who && <ShellTab key={current!.id} agent={who} handle={shellHandle(shown!)!} />}
              {shown === 'markdown' && who && <MarkdownTab key={current!.id} agent={who} tab={current!.id} />}
              {current?.opens === 'web' && <WebTab key={current.id} tab={current.id} agent={who} />}
              {current?.opens?.startsWith('web:') && (
                <WebTab key={current.id} tab={current.id} agent={who} start={current.opens.slice(4)} />
              )}
              {current?.opens?.startsWith('task:') && <TaskTab key={current.opens} id={Number(current.opens.slice(5))} />}
              {current?.opens?.startsWith('plan:') && <PlanTab key={current.opens} id={Number(current.opens.slice(5))} />}
              {opened(current?.opens ?? '', 'work') && <WorkTab key={current!.opens} at={opened(current!.opens!, 'work')!} />}
              {current?.opens?.startsWith('code:') && <CodeTab key={current.opens} id={Number(current.opens.slice(5))} />}
              {shown?.startsWith('file:') && who && <MarkdownTab key={current!.id} agent={who} tab={current!.id} start={shown.slice(5)} />}
              {current?.opens?.startsWith('text:') && <TextTab key={current.opens} id={current.opens.slice(5)} />}
              {current?.opens?.startsWith('proposal:') && <ProposalView key={current.opens} id={Number(current.opens.slice(9))} />}
              {current?.opens?.startsWith('doc:') && <DocTab key={current.opens} at={current.opens.slice(4)} />}
              <PageTab opens={current?.opens} />
              {opened(current?.opens ?? '', 'agent') && <AgentPage key={current!.opens} agent={opened(current!.opens!, 'agent')!} />}
              {opened(current?.opens ?? '', 'ws') && <WorkspacePage key={current!.opens} ws={opened(current!.opens!, 'ws')!} />}
              {current?.opens === 'tasks' && <TasksTab agent={who ?? null} />}
            </PaneOpen.Provider>
          </TabScope>
        </Boundary>
        {current && !current.opens && (
          <EmptyTab
            onPick={(kind) =>
              pick(kind, (opens) => dispatch({ type: 'choose', pane: pane.id, tab: current.id, opens, title: labelOf(opens) }))
            }
          />
        )}
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
  focus,
  onFocus,
  dispatch,
  reopenClosed,
  lead,
}: {
  root: Node;
  /** The agent the focused tab shows, or null. */
  agent: string | null;
  /** The focused pane: Cmd+W closes its tab, the sidebar opens into it. */
  focus: string;
  onFocus: (pane: string) => void;
  dispatch: Dispatch;
  /** Opens the last closed tab again; a tab whose pane is gone goes to this pane. */
  reopenClosed: (pane: string) => void;
  lead?: ReactNode;
}) {
  const nameOf = useNamesHere(agent);
  const render = (node: Node, topLeft: boolean): ReactNode =>
    node.kind === 'pane' ? (
      <PaneView
        key={node.id}
        root={root}
        pane={node}
        agent={agent}
        dispatch={dispatch}
        focused={node.id === focus}
        onFocus={onFocus}
        lead={topLeft ? lead : undefined}
      />
    ) : (
      <SplitView key={node.id} node={node} dispatch={dispatch}>
        {[render(node.a, topLeft), render(node.b, false)]}
      </SplitView>
    );
  const closeShells = useCloseShells(agent);
  // Cmd+W (Ctrl+W elsewhere) closes the tab on screen in the pane last used, Cmd+Shift+T opens the last closed again; the desktop
  // shell's menu items send the same keys. The tab keys move through its tabs (layout/tabKeys).
  useTabKeys(
    () => panes(root).find((x) => x.id === focus) ?? panes(root)[0],
    (pane, tab) => dispatch({ type: 'select', pane, tab }),
  );
  useEffect(() => {
    const close = (e: KeyboardEvent) => {
      if (!isCloseChord(e) || document.querySelector('[role="dialog"], [role="alertdialog"]')) return; // nothing closes behind a modal
      const all = panes(root);
      const p = all.find((x) => x.id === focus) ?? all[0];
      e.preventDefault();
      closeShells(p.tabs.filter((t) => t.id === p.active));
      dispatch({ type: 'closeTab', pane: p.id, tab: p.active });
    };
    const reopen = (e: KeyboardEvent) => {
      if (!isReopenChord(e) || document.querySelector('[role="dialog"], [role="alertdialog"]')) return;
      e.preventDefault();
      reopenClosed(focus);
    };
    addEventListener('keydown', reopen);
    addEventListener('keydown', close);
    return () => {
      removeEventListener('keydown', close);
      removeEventListener('keydown', reopen);
    };
  }, [root, focus, dispatch, closeShells, reopenClosed]);
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
  // Opened from outside a pane (the sidebar): from the focused pane.
  const workspaceCluster = useContext(SpaceCluster);
  const agentKey = agent && spaceOf(workspaceCluster, agent);
  useEffect(() =>
    setShownWorkspace((opens, how) => openFrom({ root, agent: agentKey, dispatch, nameOf, focus: onFocus }, focus, opens, how)),
  );
  return (
    <div {...{ [DROP_AREA]: '' }} className="flex min-h-0 min-w-0 flex-1">
      {render(root, true)}
    </div>
  );
}

/** Closes the shells of tabs being closed. */
function useCloseShells(agent: string | null) {
  const { mutate } = useWrite('shell.close', agent ?? undefined);
  return useCallback(
    (tabs: { opens?: string }[]) => {
      for (const t of tabs) {
        // Another agent's shell (`shell:<handle>|<agent>`) closes by its own handle.
        const opens = t.opens ?? '';
        const h = shellHandle(ownedBy(opens)?.kind ?? opens);
        if (h) mutate({ terminal: h });
      }
    },
    [mutate],
  );
}

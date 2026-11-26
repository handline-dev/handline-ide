// The town's windows around its picture (windows.ts holds them as data): tab bars docked down one side at a third, a half or two
// thirds of the width, the picture shrinking beside them so nothing hides behind, and tab bars floating over it. Each tab bar is the
// app's own (Workspace.tsx), so tabs, links and splits work in it as on the main screen; its handle (layout/paneGrip.tsx) carries the
// whole bar: let go on the picture's left or right edge it docks there, elsewhere on the picture it floats, on another bar it joins it.
import { AnimatePresence, motion } from 'framer-motion';
import { createContext, useContext, useEffect, useRef, useState, type DragEvent, type ReactNode, type RefObject } from 'react';
import { Workspace, type Grip, type IconFor } from '#src/Workspace.tsx';
import { labelOf } from '#src/layout/kinds.tsx';
import { useOpensOver } from '#src/layout/openTab.tsx';
import { PANE_PLACE, type At, type PaneDrop } from '#src/layout/paneGrip.tsx';
import { STOPS, holds, snapRatio, stepStop, type Action, type Pane } from '#src/layout/tree.ts';
import { spring } from '#src/motion.ts';
import { usePersisted } from '#src/persist/kv.ts';
import { isTabDrag, readTabDrag, showZone, zoneAt } from '#src/ui/dnd.tsx';
import { useDropTarget } from '#src/ui/dropTarget.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { FloatWindow, FLOAT_W } from '#src/town/FloatWindow.tsx';
import { NONE, focusIn, paneShowing, reduceWindows, type Moving, type Side, type WindowAction, type Windows } from '#src/town/windows.ts';

/** Below this share the dock is let go of and closes, as a split view's side pushed off the screen. */
const CLOSES = 0.18;
const SCENE = 'scene';

type Shown = { show: (opens: string, title: string) => void; shows: (opens: string) => boolean };
const TownShown = createContext<Shown>({ show: () => {}, shows: () => false });
/** Opens a tab in the town's windows (a person clicked, the founder's desk), and says what a tab bar shows now. */
export const useTownShown = () => useContext(TownShown);

/** Narrower than this, the town is a phone's: the dock goes under the picture. */
const PHONE = 640;
function useNarrow(area: RefObject<HTMLDivElement | null>) {
  const [narrow, setNarrow] = useState(false);
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    const seen = new ResizeObserver(([e]) => setNarrow(e.contentRect.width < PHONE));
    seen.observe(el);
    return () => seen.disconnect();
  }, [area]);
  return narrow;
}

const all = (p: Pane) => ({ from: p.id, tabs: p.tabs.map((t) => t.id), shown: p.active });
const HINT = 'Drag to move this tab bar; ← or → docks it on that side, ↑ floats it';

export function TownWindows({ iconFor, children }: { iconFor: IconFor; children: ReactNode }) {
  const [w, setW] = usePersisted<Windows>('town-windows', NONE);
  // The pane last clicked, or the thing last brought up; the keys go to it while it is there, else to the first pane (focusIn).
  const [chosen, setChosen] = useState('');
  const [want, setWant] = useState<string | null>(null);
  const focus = (want && paneShowing(w, want)?.id) || focusIn(w, chosen);
  const setFocus = (pane: string) => {
    setChosen(pane);
    setWant(null);
  };
  const area = useRef<HTMLDivElement>(null);
  const narrow = useNarrow(area);
  // The dock's share while its divider is dragged; it rests at the nearest stop when let go.
  const [drag, setDrag] = useState<number | null>(null);
  const act = (a: WindowAction) => {
    setW((v) => reduceWindows(v, a));
    // The keys follow what is brought up, and a tab picked by key keeps them in its pane.
    const asked = a.type === 'tree' ? a.action : a;
    if (asked.type === 'show' || asked.type === 'open') setWant(asked.opens);
    if (asked.type === 'select') setFocus(asked.pane);
  };
  // What opens from outside a tab bar while the town is up (the status row's bell) opens here, not behind it.
  useOpensOver((opens) => act({ type: 'show', opens, title: labelOf(opens) }));
  const rect = () => area.current?.getBoundingClientRect() ?? new DOMRect(0, 0, 1, 1);
  /** A float's top left for a tab bar let go at `at`, its handle under the pointer. */
  const floatAt = (at: At) => {
    const r = rect();
    return { x: (at.x - r.left - FLOAT_W / 2) / r.width, y: (at.y - r.top - 8) / r.height };
  };
  /** Where `what` goes, let go on the picture: its outer quarter on a side docks it there, anywhere else floats it at the pointer. */
  const onScene = (what: Moving, zone: PaneDrop['zone'], at: At) =>
    act(zone === 'left' || zone === 'right' ? { type: 'dock', what, side: zone } : { type: 'float', what, ...floatAt(at) });
  /** The handle of tab bar `pane` let go: `moved` is a float's own move, kept when it lands on nothing that takes it. */
  const placed = (pane: Pane, drop: PaneDrop | null, at: At, moved?: () => void) => {
    if (drop && 'place' in drop) return drop.zone === 'center' && moved ? moved() : onScene(all(pane), drop.zone, at);
    if (!drop || drop.pane === pane.id) return moved?.();
    const to = drop.zone === 'center' ? { pane: drop.pane } : { pane: drop.pane, edge: drop.zone };
    act({ type: 'tree', action: { type: 'move', from: pane.id, tab: pane.active, tabs: all(pane).tabs, to } });
  };
  const keyed = (pane: Pane) => (k: string) => {
    if (k === 'ArrowLeft' || k === 'ArrowRight') act({ type: 'dock', what: all(pane), side: k === 'ArrowLeft' ? 'left' : 'right' });
    if (k === 'ArrowUp') act({ type: 'float', what: all(pane), x: 0.3, y: 0.1 });
  };
  const dockGrip: Grip = (pane) => ({ onDrop: (drop, at) => placed(pane, drop, at), onKey: keyed(pane), hint: HINT });
  const tree = (a: Action) => act({ type: 'tree', action: a });
  // A tab or a link dragged onto the picture: docked on the side it is let go at, else floated there.
  const sceneDrop = useDropTarget({
    accepts: isTabDrag,
    onOver: (e) => {
      const z = zoneAt(e, e.currentTarget as HTMLElement);
      if (z === 'left' || z === 'right') showZone(e.currentTarget as HTMLElement, z);
    },
    onDrop: (e: DragEvent) => {
      const d = readTabDrag(e);
      if (!d) return;
      const what: Moving =
        'move' in d
          ? { from: d.move.pane, tabs: d.move.tabs ?? [d.move.tab], shown: d.move.tab }
          : { opens: d.open.opens, title: labelOf(d.open.opens) };
      onScene(what, zoneAt(e, e.currentTarget as HTMLElement), { x: e.clientX, y: e.clientY });
    },
  });
  // On a phone's width the dock is a sheet under the picture, as tall as its stop and never under half the height.
  const at: Side | 'bottom' = narrow ? 'bottom' : w.side;
  const share = drag ?? (at === 'bottom' ? Math.max(STOPS[1], w.stop) : w.stop);
  const [size, across] = at === 'bottom' ? ['height', 'width'] : ['width', 'height'];
  const dock = w.dock && (
    <motion.div
      key="dock"
      data-no-walk
      initial={{ [size]: 0, opacity: 0 }}
      animate={{ [size]: `${share * 100}%`, [across]: '100%', opacity: 1 }}
      exit={{ [size]: 0, opacity: 0 }}
      transition={drag === null ? spring : { duration: 0 }}
      className="flex min-h-0 min-w-0 shrink-0 overflow-hidden"
    >
      <Workspace
        root={w.dock}
        agent={null}
        focus={focus}
        onFocus={setFocus}
        dispatch={tree}
        reopenClosed={() => {}}
        quiet={!holds(w.dock, focus)}
        grip={dockGrip}
        iconFor={iconFor}
      />
    </motion.div>
  );
  const divider = w.dock && (
    <Splitter
      axis={at === 'bottom' ? 'y' : 'x'}
      container={area}
      percent={share * 100}
      onMove={(o, all) => setDrag(Math.min(0.8, Math.max(0.05, at === 'left' ? o / all : 1 - o / all)))}
      onStep={(d) => act({ type: 'stop', stop: stepStop(w.stop, at === 'left' ? d : (-d as -1 | 1)) })}
      onDragging={(on) => {
        if (on || drag === null) return;
        act(drag < CLOSES ? { type: 'closeDock' } : { type: 'stop', stop: snapRatio(drag) });
        setDrag(null);
      }}
      label="Resize the windows"
    />
  );
  const shown: Shown = {
    show: (opens, title) => act({ type: 'show', opens, title, pane: focus }),
    shows: (opens) => !!paneShowing(w, opens),
  };
  return (
    <TownShown.Provider value={shown}>
      <div ref={area} className={`relative flex min-h-0 min-w-0 flex-1 ${at === 'bottom' ? 'flex-col' : ''}`}>
        <AnimatePresence initial={false}>{at === 'left' && dock}</AnimatePresence>
        {at === 'left' && divider}
        <div {...{ [PANE_PLACE]: SCENE }} className="relative flex min-h-0 min-w-0 flex-1" {...sceneDrop.handlers}>
          {children}
        </div>
        {at !== 'left' && divider}
        <AnimatePresence initial={false}>{at !== 'left' && dock}</AnimatePresence>
        {/* While the divider is dragged, where it can rest. */}
        {drag !== null &&
          STOPS.map((stop) => (
            <span
              key={stop}
              aria-hidden
              style={at === 'bottom' ? { top: `${(1 - stop) * 100}%` } : { left: `${(at === 'left' ? stop : 1 - stop) * 100}%` }}
              className={`pointer-events-none absolute z-30 bg-accent/40 ${at === 'bottom' ? 'inset-x-0 h-px' : 'inset-y-0 w-px'}`}
            />
          ))}
        <AnimatePresence>
          {w.floats.map((f) => (
            <FloatWindow
              key={f.id}
              float={f}
              area={rect}
              focus={focus}
              onFocus={(pane) => {
                setFocus(pane);
                act({ type: 'raise', float: f.id });
              }}
              dispatch={tree}
              iconFor={iconFor}
              grip={(pane, moved) => ({ onDrop: (drop, at) => placed(pane, drop, at, moved), onKey: keyed(pane), hint: HINT })}
              onMoved={(x, y) => act({ type: 'moveFloat', float: f.id, x, y })}
            />
          ))}
        </AnimatePresence>
      </div>
    </TownShown.Provider>
  );
}

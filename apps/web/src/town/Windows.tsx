// The town's windows around its picture (windows.ts holds them as data): tab bars floating over it, as many as are wanted, and tab
// bars docked down one side at a third, a half or two thirds of the width, the picture shrinking beside them. What is opened floats
// (on a phone's width it goes in the dock, a sheet under the picture). Each tab bar is the app's own (Workspace.tsx), so tabs, links and
// splits work in it as on the main screen. A tab, a link or a whole bar (its handle, layout/paneGrip.tsx) let go on open space of the
// picture becomes a new floating tab bar there; on the picture's left or right edge it docks there; on another bar it joins it.
import { AnimatePresence, motion } from 'framer-motion';
import {
  createContext,
  useContext,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type DragEvent,
  type ReactNode,
  type RefObject,
} from 'react';
import { Workspace, type Grip, type IconFor } from '#src/Workspace.tsx';
import { labelOf } from '#src/layout/kinds.tsx';
import { useOpensOver } from '#src/layout/openTab.tsx';
import { PANE_PLACE, type At, type PaneDrop } from '#src/layout/paneGrip.tsx';
import { STOPS, holds, snapRatio, stepStop, type Action, type Pane } from '#src/layout/tree.ts';
import { spring } from '#src/motion.ts';
import { usePersisted } from '#src/persist/kv.ts';
import { DROP_AREA, isTabDrag, readTabDrag, showBox, showZone, sideAt } from '#src/ui/dnd.tsx';
import { useDropTarget } from '#src/ui/dropTarget.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { FloatWindow, FLOAT_H, FLOAT_W } from '#src/town/FloatWindow.tsx';
import {
  NONE,
  focusIn,
  paneShowing,
  reduceWindows,
  sheetOf,
  spotAway,
  type Moving,
  type Side,
  type WindowAction,
  type Windows,
} from '#src/town/windows.ts';

/** Below this share the dock is let go of and closes, as a split view's side pushed off the screen. */
const CLOSES = 0.18;
const SCENE = 'scene';

type Shown = { show: (opens: string, title: string) => void; shows: (opens: string) => boolean };
const TownShown = createContext<Shown>({ show: () => {}, shows: () => false });
/** Opens a tab in the town's windows (a person clicked, the founder's desk), and says what a tab bar shows now. */
export const useTownShown = () => useContext(TownShown);

/** Narrower than this, the town is a phone's: the dock goes under the picture. */
const PHONE = 640;
/** Whether the town is as narrow as a phone; `onNarrow` is called as it becomes so. */
function useNarrow(area: RefObject<HTMLDivElement | null>, onNarrow: () => void) {
  const [narrow, setNarrow] = useState(false);
  const became = useEffectEvent(onNarrow);
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    const seen = new ResizeObserver(([e]) => {
      // Not yet laid out (a width of none) is not narrow: the floats would join the sheet for good.
      const now = e.contentRect.width > 0 && e.contentRect.width < PHONE;
      setNarrow(now);
      if (now) became();
    });
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
  // On a phone's width nothing floats: what floated goes into the sheet under the picture.
  const narrow = useNarrow(area, () => setW(sheetOf));
  // Where the town was last pressed, as a share of its width: a window opened by that press goes to the other side.
  const pressed = useRef(0);
  // The dock's share while its divider is dragged; it rests at the nearest stop when let go.
  const [drag, setDrag] = useState<number | null>(null);
  const act = (a: WindowAction) => {
    setW((v) => reduceWindows(v, a));
    // The keys follow what is brought up, and a tab picked by key keeps them in its pane.
    const asked = a.type === 'tree' ? a.action : a;
    if (asked.type === 'show' || asked.type === 'open') setWant(asked.opens);
    if (asked.type === 'select') setFocus(asked.pane);
  };
  const rect = () => area.current?.getBoundingClientRect() ?? new DOMRect(0, 0, 1, 1);
  /** Where a new floating tab bar opens: away from what was clicked, each one more a little further in so none hides another. */
  const spot = () => spotAway(pressed.current, FLOAT_W / rect().width, w.floats.length);
  /** Opens a tab: in the tab bar with the keys, else in a new floating one (on a phone's width, the dock). */
  const open = (opens: string, title: string) =>
    narrow
      ? act({ type: 'show', opens, title, pane: w.dock && holds(w.dock, focus) ? focus : undefined })
      : act({ type: 'show', opens, title, pane: focus, at: spot() });
  // What opens from outside a tab bar while the town is up (the status row's bell) opens here, not behind it.
  useOpensOver((opens) => open(opens, labelOf(opens)));
  /** The box on screen of a float let go at `at`, its handle under the pointer, kept inside the town as FloatWindow keeps it. */
  const floatBox = (at: At) => {
    const r = rect();
    const width = Math.min(FLOAT_W, r.width);
    const height = Math.min(FLOAT_H, r.height - 24);
    const left = Math.min(r.right - width, Math.max(r.left, at.x - FLOAT_W / 2));
    const top = Math.min(r.bottom - height, Math.max(r.top, at.y - 8));
    return { left, top, width, height };
  };
  /** A float's top left, as a share of the town, for a tab bar let go at `at`. */
  const floatAt = (at: At) => {
    const r = rect();
    const b = floatBox(at);
    return { x: (b.left - r.left) / r.width, y: (b.top - r.top) / r.height };
  };
  /** Where `what` goes, let go on the picture: its left or right edge docks it there, anywhere else floats it at the pointer. */
  const onScene = (what: Moving, zone: PaneDrop['zone'], at: At) => {
    if (narrow) return act({ type: 'dock', what, side: w.side });
    act(zone === 'left' || zone === 'right' ? { type: 'dock', what, side: zone } : { type: 'float', what, ...floatAt(at) });
  };
  /** The handle of tab bar `pane` let go: `moved` is a float's own move, kept when it lands on nothing that takes it. */
  const placed = (pane: Pane, drop: PaneDrop | null, at: At, moved?: () => void) => {
    if (drop && 'place' in drop) return drop.zone === 'center' && moved ? moved() : onScene(all(pane), drop.zone, at);
    if (!drop || drop.pane === pane.id) return moved?.();
    const to = drop.zone === 'center' ? { pane: drop.pane } : { pane: drop.pane, edge: drop.zone };
    act({ type: 'tree', action: { type: 'move', from: pane.id, tab: pane.active, tabs: all(pane).tabs, to } });
  };
  const keyed = (pane: Pane) => (k: string) => {
    if (k === 'ArrowLeft' || k === 'ArrowRight') act({ type: 'dock', what: all(pane), side: k === 'ArrowLeft' ? 'left' : 'right' });
    if (k === 'ArrowUp' && !narrow) act({ type: 'float', what: all(pane), ...spot() });
  };
  const dockGrip: Grip = (pane) => ({ onDrop: (drop, at) => placed(pane, drop, at), onKey: keyed(pane), hint: HINT, floatBox });
  const tree = (a: Action) => act({ type: 'tree', action: a });
  // A tab or a link dragged onto the picture: docked on the edge it is let go at, else a new tab bar floating there, outlined while held.
  const sceneDrop = useDropTarget({
    accepts: isTabDrag,
    onOver: (e) => {
      const z = sideAt(e, e.currentTarget as HTMLElement);
      if (z === 'center') showBox(floatBox({ x: e.clientX, y: e.clientY }));
      else showZone(e.currentTarget as HTMLElement, z);
    },
    onDrop: (e: DragEvent) => {
      const d = readTabDrag(e);
      if (!d) return;
      const what: Moving =
        'move' in d
          ? { from: d.move.pane, tabs: d.move.tabs ?? [d.move.tab], shown: d.move.tab }
          : { opens: d.open.opens, title: labelOf(d.open.opens) };
      onScene(what, sideAt(e, e.currentTarget as HTMLElement), { x: e.clientX, y: e.clientY });
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
    show: open,
    shows: (opens) => !!paneShowing(w, opens),
  };
  return (
    <TownShown.Provider value={shown}>
      <div
        ref={area}
        {...{ [DROP_AREA]: '' }}
        onPointerDownCapture={(e) => {
          const r = rect();
          pressed.current = (e.clientX - r.left) / r.width;
        }}
        className={`relative flex min-h-0 min-w-0 flex-1 ${at === 'bottom' ? 'flex-col' : ''}`}
      >
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
              grip={(pane, moved) => ({ onDrop: (drop, at) => placed(pane, drop, at, moved), onKey: keyed(pane), hint: HINT, floatBox })}
              onMoved={(x, y) => act({ type: 'moveFloat', float: f.id, x, y })}
            />
          ))}
        </AnimatePresence>
      </div>
    </TownShown.Provider>
  );
}

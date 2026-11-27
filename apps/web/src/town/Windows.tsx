// The town's windows around its picture (windows.ts holds them as data): full-height tab bars floating over it, and tab bars docked on
// its left, its right or both, at a third, a half or two thirds of the width. What is opened floats (on a phone's width, a sheet under the
// picture). Each tab bar is the app's own (Workspace.tsx). A tab, a link or a whole bar (its handle, layout/paneGrip.tsx) let go on open
// space floats there; on the picture's left or right edge it docks there; on another bar it joins it. The handle's menu does the same.
import { AnimatePresence, motion } from 'framer-motion';
import { AppWindow, PanelLeft, PanelRight } from 'lucide-react';
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
import { MenuItem } from '#src/ui/Menu.tsx';
import { Splitter } from '#src/ui/Splitter.tsx';
import { FloatWindow } from '#src/town/FloatWindow.tsx';
import {
  DOCKS_MOST,
  FLOAT_W,
  NONE,
  focusIn,
  paneShowing,
  reduceWindows,
  sharesOf,
  sheetOf,
  sideOf,
  spotAway,
  type Moving,
  type Side,
  type WindowAction,
  type Windows,
} from '#src/town/windows.ts';

/** Below this share the dock is let go of and closes, as a split view's side pushed off the screen. */
const CLOSES = 0.18;
const SCENE = 'scene';

/** `narrow`: the town is as narrow as a phone. */
type Shown = { show: (opens: string, title: string) => void; shows: (opens: string) => boolean; narrow: boolean };
const TownShown = createContext<Shown>({ show: () => {}, shows: () => false, narrow: false });
/** Opens a tab in the town's windows (a person clicked, the founder's desk), says what a tab bar shows now and whether it is a phone's. */
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

const opposite = (s: Side): Side => (s === 'left' ? 'right' : 'left');
const all = (p: Pane) => ({ from: p.id, tabs: p.tabs.map((t) => t.id), shown: p.active });
const HINT = 'Drag or click to move this tab bar; ← or → docks it on that side, ↑ floats it';

type Place = Side | 'float';
const MOVES: { to: Place; label: string; icon: ReactNode }[] = [
  { to: 'left', label: 'Dock left', icon: <PanelLeft size={14} /> },
  { to: 'right', label: 'Dock right', icon: <PanelRight size={14} /> },
  { to: 'float', label: 'Float', icon: <AppWindow size={14} /> },
];

/** The menu of where a tab bar can go from `at` (its dock's side, or floating); none when nowhere (a phone's sheet). */
function moveMenu(at: Place, narrow: boolean, go: (to: Place) => void): ReactNode {
  const moves = MOVES.filter((m) => m.to !== at && !(narrow && m.to !== 'right'));
  if (!moves.length) return undefined;
  return moves.map((m) => (
    <MenuItem key={m.to} icon={m.icon} onClick={() => go(m.to)}>
      {m.label}
    </MenuItem>
  ));
}

export function TownWindows({ iconFor, children }: { iconFor: IconFor; children: ReactNode }) {
  const [w, setW] = usePersisted<Windows>('town-docks', NONE);
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
  // A dock's share while its divider is dragged; it rests at the nearest stop when let go.
  const [drag, setDrag] = useState<{ side: Side; share: number } | null>(null);
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
      ? act({ type: 'show', opens, title, pane: sideOf(w, focus) ? focus : undefined })
      : act({ type: 'show', opens, title, pane: focus, at: spot() });
  // What opens from outside a tab bar while the town is up (the status row's bell) opens here, not behind it.
  useOpensOver((opens) => open(opens, labelOf(opens)));
  /** The box on screen of a float let go at `at`, its handle under the pointer, the town's full height and kept inside it. */
  const floatBox = (at: At) => {
    const r = rect();
    const width = Math.min(FLOAT_W, r.width);
    const left = Math.min(r.right - width, Math.max(r.left, at.x - width / 2));
    return { left, top: r.top, width, height: r.height };
  };
  /** A float's left edge, as a share of the town's width, for a tab bar let go at `at`. */
  const floatAt = (at: At) => (floatBox(at).left - rect().left) / rect().width;
  /** Where `what` goes, let go on the picture: its left or right edge docks it there, anywhere else floats it at the pointer. */
  const onScene = (what: Moving, zone: PaneDrop['zone'], at: At) => {
    if (narrow) return act({ type: 'dock', what, side: 'right' });
    act(zone === 'left' || zone === 'right' ? { type: 'dock', what, side: zone } : { type: 'float', what, x: floatAt(at) });
  };
  /** The handle of tab bar `pane` let go: `moved` is a float's own move, kept when it lands on nothing that takes it. */
  const placed = (pane: Pane, drop: PaneDrop | null, at: At, moved?: () => void) => {
    if (drop && 'place' in drop) return drop.zone === 'center' && moved ? moved() : onScene(all(pane), drop.zone, at);
    if (!drop || drop.pane === pane.id) return moved?.();
    const to = drop.zone === 'center' ? { pane: drop.pane } : { pane: drop.pane, edge: drop.zone };
    act({ type: 'tree', action: { type: 'move', from: pane.id, tab: pane.active, tabs: all(pane).tabs, to } });
  };
  /** Tab bar `pane` sent to a side's dock or to float, by key or by its handle's menu. */
  const send = (pane: Pane) => (to: Place) => {
    if (to !== 'float') act({ type: 'dock', what: all(pane), side: narrow ? 'right' : to });
    else if (!narrow) act({ type: 'float', what: all(pane), x: spot() });
  };
  const keyed = (pane: Pane) => (k: string) => send(pane)(k === 'ArrowLeft' ? 'left' : k === 'ArrowRight' ? 'right' : 'float');
  const gripOf = (pane: Pane, at: Place, moved?: () => void) => ({
    onDrop: (drop: PaneDrop | null, to: At) => placed(pane, drop, to, moved),
    onKey: keyed(pane),
    hint: HINT,
    floatBox,
    menu: moveMenu(at, narrow, send(pane)),
  });
  const dockGrip: Grip = (pane) => gripOf(pane, sideOf(w, pane.id) ?? 'right');
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
  // On a phone's width the right dock is a sheet under the picture (sheetOf puts everything there), never under half the height.
  const raw = (side: Side) => (!w.docks[side] ? 0 : drag?.side === side ? drag.share : w.stops[side]);
  const shareOf = (side: Side) => (narrow ? Math.max(STOPS[1], raw('right')) : sharesOf(raw('left'), raw('right'))[side]);
  const [size, across] = narrow ? ['height', 'width'] : ['width', 'height'];
  const dock = (side: Side) => {
    const root = w.docks[side];
    if (!root || (narrow && side === 'left')) return null;
    return (
      <motion.div
        key={side}
        data-no-walk
        initial={{ [size]: 0, opacity: 0 }}
        animate={{ [size]: `${shareOf(side) * 100}%`, [across]: '100%', opacity: 1 }}
        exit={{ [size]: 0, opacity: 0 }}
        transition={drag?.side === side ? { duration: 0 } : spring}
        className="flex min-h-0 min-w-0 shrink-0 overflow-hidden"
      >
        <Workspace
          root={root}
          agent={null}
          focus={focus}
          onFocus={setFocus}
          dispatch={tree}
          reopenClosed={() => {}}
          quiet={!holds(root, focus)}
          grip={dockGrip}
          iconFor={iconFor}
        />
      </motion.div>
    );
  };
  const divider = (side: Side) => {
    if (!w.docks[side] || (narrow && side === 'left')) return null;
    const other = narrow || !w.docks[opposite(side)] ? 0 : shareOf(opposite(side));
    const toward = side === 'left' ? 1 : -1;
    return (
      <Splitter
        axis={narrow ? 'y' : 'x'}
        container={area}
        percent={shareOf(side) * 100}
        onMove={(o, all) => setDrag({ side, share: Math.min(DOCKS_MOST - other, Math.max(0.05, side === 'left' ? o / all : 1 - o / all)) })}
        onStep={(d) => act({ type: 'stop', side, stop: stepStop(w.stops[side], (d * toward) as -1 | 1) })}
        onDragging={(on) => {
          if (on || drag?.side !== side) return;
          act(drag.share < CLOSES ? { type: 'closeDock', side } : { type: 'stop', side, stop: snapRatio(drag.share) });
          setDrag(null);
        }}
        label="Resize the windows"
      />
    );
  };
  const shown: Shown = {
    show: open,
    shows: (opens) => !!paneShowing(w, opens),
    narrow,
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
        className={`relative flex min-h-0 min-w-0 flex-1 ${narrow ? 'flex-col' : ''}`}
      >
        <AnimatePresence initial={false}>{dock('left')}</AnimatePresence>
        {divider('left')}
        <div {...{ [PANE_PLACE]: SCENE }} className="relative flex min-h-0 min-w-0 flex-1" {...sceneDrop.handlers}>
          {children}
        </div>
        {divider('right')}
        <AnimatePresence initial={false}>{dock('right')}</AnimatePresence>
        {/* While a divider is dragged, where it can rest. */}
        {drag !== null &&
          STOPS.map((stop) => (
            <span
              key={stop}
              aria-hidden
              style={narrow ? { top: `${(1 - stop) * 100}%` } : { left: `${(drag.side === 'left' ? stop : 1 - stop) * 100}%` }}
              className={`pointer-events-none absolute z-30 bg-accent/40 ${narrow ? 'inset-x-0 h-px' : 'inset-y-0 w-px'}`}
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
              grip={(pane, moved) => gripOf(pane, 'float', moved)}
              onPlaced={(x, wide) => act({ type: 'placeFloat', float: f.id, x, w: wide })}
            />
          ))}
        </AnimatePresence>
      </div>
    </TownShown.Provider>
  );
}

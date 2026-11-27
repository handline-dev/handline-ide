// The city's view: how far in it is zoomed and where its picture sits in the pane (zoom.ts does the sums). In the close-up it stays on
// the founder; farther out it moves only to keep the founder in view, so a zoom keeps what is under the pointer still. The wheel zooms at
// once; a step by a button or a key eases there. Like the founder's place it lives outside React, and is kept while the founder is
// inside a building: only what draws by it follows it.
import { useEffect, useEffectEvent, useLayoutEffect, useState, useSyncExternalStore, type ReactNode } from 'react';
import { notMine, type Founder, type Ground } from '#src/town/founder.tsx';
import { eased, keepPoint, rangeOf, scaleAt, settle, stepped, zoomed, zooms, type Pair } from '#src/town/zoom.ts';

/** What the view is of: the picture's size, the pane's, and the founder's spot in the picture, if it is there. */
type Scene = { size: Pair; pane: Pair; spot: () => Pair | null };

let kept: { level: number; o: Pair } = { level: 1, o: [0, 0] };
/** How long a step by a button or a key takes to ease to its level. */
const EASE_MS = 150;

function cameraOf() {
  const all = new Set<() => void>();
  const s = { ...kept, scene: null as Scene | null, at: `${Math.round(kept.o[0])},${Math.round(kept.o[1])}` };
  /** Rests the picture where it belongs now; tells whoever follows it when it moved, or when `changed` already. */
  const rest = (changed = false) => {
    const v = s.scene;
    if (!v) return;
    s.o = settle(s.o, v.spot(), s.level, v.size, v.pane);
    kept = { level: s.level, o: s.o };
    const at = `${Math.round(s.o[0])},${Math.round(s.o[1])}`;
    if (at === s.at && !changed) return;
    s.at = at;
    all.forEach((l) => l());
  };
  /** To `level`, the picture's point under pane point `about` kept still: the founder when none is given and it is in view, else the
   *  pane's middle. */
  const to = (level: number, about?: Pair) => {
    const v = s.scene;
    if (!v || level === s.level) return;
    const range = rangeOf(v.size, v.pane);
    const from = scaleAt(s.level, range);
    const me = v.spot();
    const mine: Pair | null = me && [s.o[0] + me[0] * from, s.o[1] + me[1] * from];
    const seen = mine && mine[0] >= 0 && mine[1] >= 0 && mine[0] <= v.pane[0] && mine[1] <= v.pane[1];
    s.o = keepPoint(s.o, about ?? (seen ? mine : [v.pane[0] / 2, v.pane[1] / 2]), from, scaleAt(level, range));
    s.level = level;
    rest(true);
  };
  const range = () => (s.scene ? rangeOf(s.scene.size, s.scene.pane) : { min: 1, max: 1 });
  // A step by a button or a key eases to its level over a few frames; `goal` is where it is going, null when it is there.
  const glide = { goal: null as number | null, frame: 0 };
  /** Ends an easing where it is going (`settled`) or where it has got to. */
  const still = (settled: boolean) => {
    cancelAnimationFrame(glide.frame);
    if (settled && glide.goal !== null) to(glide.goal);
    glide.goal = null;
  };
  const ease = (level: number) => {
    still(false);
    if (!s.scene || level === s.level) return;
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) return to(level);
    const [from, start] = [s.level, performance.now()];
    glide.goal = level;
    const tick = (now: number) => {
      const k = Math.min(1, (now - start) / EASE_MS);
      to(eased(from, level, k));
      if (k < 1) glide.frame = requestAnimationFrame(tick);
      else glide.goal = null;
    };
    glide.frame = requestAnimationFrame(tick);
  };
  const aim = () => glide.goal ?? s.level;
  return {
    subscribe: (l: () => void) => (all.add(l), () => void all.delete(l)),
    level: () => s.level,
    /** The picture's top left in the pane, in whole pixels, as "x,y". */
    at: () => s.at,
    look: (scene: Scene) => {
      s.scene = scene;
      rest(true);
    },
    moved: () => rest(),
    /** Closer by `factor` (under 1 is farther), about a point of the pane, at once: the wheel's own steps are small. */
    zoom: (factor: number, about?: Pair) => {
      still(false);
      to(zoomed(s.level, factor, range()), about);
    },
    /** A press of + or -, from where the last press is going. */
    step: (dir: 1 | -1) => ease(stepped(aim(), dir)),
    /** The whole town, or from it back to the close-up on the founder. */
    toggle: () => ease(aim() === 0 ? 1 : 0),
    /** The view is going away: an easing ends where it was going, so that is what is kept. */
    leave: () => still(true),
  };
}

export type Camera = ReturnType<typeof cameraOf>;

/** The view of a picture of `size` in `pane` with the founder on `ground`: the camera, its level (0 the whole town, 1 the close-up), the
 *  scale to draw at, and whether there is anything to zoom. */
export function useCamera(founder: Founder, ground: Ground, [w, h]: Pair, [pw, ph]: Pair) {
  const [camera] = useState(cameraOf);
  useLayoutEffect(() => {
    // A pane not yet measured has no middle to keep the founder near.
    if (pw <= 0 || ph <= 0) return;
    camera.look({
      size: [w, h],
      pane: [pw, ph],
      spot: () => {
        const me = founder.get();
        return me ? ground.toPixel(me.at[0], 0, me.at[1]) : null;
      },
    });
    return founder.subscribe(camera.moved);
  }, [camera, founder, ground, w, h, pw, ph]);
  useEffect(() => camera.leave, [camera]);
  const level = useSyncExternalStore(camera.subscribe, camera.level);
  const range = rangeOf([w, h], [pw, ph]);
  const can = zooms(range);
  return { camera, level: can ? level : 1, scale: scaleAt(level, range), can, whole: can && level === 0 };
}

/** The wheel over `pane` zooms about the pointer, a trackpad's pinch (a wheel with Ctrl) faster for its small steps; not over a field
 *  or a list in the pane, which scrolls. */
export function useWheelZoom(pane: HTMLElement | null, camera: Camera) {
  useEffect(() => {
    if (!pane) return;
    const wheel = (e: WheelEvent) => {
      if (notMine(e.target)) return;
      e.preventDefault();
      const r = pane.getBoundingClientRect();
      const turned = e.deltaY * (e.deltaMode === WheelEvent.DOM_DELTA_LINE ? 16 : 1);
      camera.zoom(Math.exp(-turned * (e.ctrlKey ? 0.01 : 0.002)), [e.clientX - r.left, e.clientY - r.top]);
    };
    pane.addEventListener('wheel', wheel, { passive: false });
    return () => pane.removeEventListener('wheel', wheel);
  }, [pane, camera]);
}

/** The city's own keys, while it is shown: / calls `find` (the keys go to the Find field), M calls `toggle` (the whole town, or back). */
export function useCityKeys(on: { find: () => void; toggle: () => void }) {
  const key = useEffectEvent((e: KeyboardEvent) => {
    if (e.repeat || e.metaKey || e.ctrlKey || e.altKey || notMine(e.target)) return;
    if (e.key === '/') on.find();
    else if (e.code === 'KeyM') on.toggle();
    else return;
    e.preventDefault();
  });
  useEffect(() => {
    const down = (e: KeyboardEvent) => key(e);
    window.addEventListener('keydown', down);
    return () => window.removeEventListener('keydown', down);
  }, []);
}

/** The picture placed in the pane where the camera has it; only this follows each step and each turn of the wheel. */
export function Placed({ camera, children }: { camera: Camera; children: ReactNode }) {
  const [x, y] = useSyncExternalStore(camera.subscribe, camera.at).split(',').map(Number);
  return (
    <div className="absolute top-0 left-0" style={{ transform: `translate(${x}px, ${y}px)` }}>
      {children}
    </div>
  );
}

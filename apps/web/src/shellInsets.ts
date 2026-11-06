// Room for the Mac window buttons, which ignore page zoom and hide in full screen.
import { shell } from '#src/shell.ts';

/** Where the window buttons are in screen px (apps/electron main.ts trafficLightPosition, plus the buttons themselves). */
const LIGHTS = { right: 86, centre: 22 };

/** --lights-w: the room left of our controls; --controls-top: their top, so their middle is level with the buttons'. */
export function insetsFor(zoom: number, full: boolean): Record<string, string> {
  if (full) return { '--lights-w': '0px', '--controls-top': '6px' };
  return { '--lights-w': `${LIGHTS.right / zoom}px`, '--controls-top': `${LIGHTS.centre / zoom - 14}px` };
}

/** In the shell on a Mac, keeps the two variables current; in a browser it does nothing and the CSS defaults hold. */
export function trackShellInsets(): void {
  const s = shell();
  if (!s || !navigator.userAgent.includes('Mac')) return;
  let full = false;
  const set = () => {
    const zoom = s.zoom?.() || 1;
    for (const [k, v] of Object.entries(insetsFor(zoom, full))) document.documentElement.style.setProperty(k, v);
  };
  // The zoom may lag the resize it causes; read it again after.
  let later = 0;
  const settle = () => {
    set();
    clearTimeout(later);
    later = window.setTimeout(set, 150);
  };
  settle();
  addEventListener('resize', settle);
  s.onFullScreen?.((on) => {
    full = on;
    settle();
  });
}

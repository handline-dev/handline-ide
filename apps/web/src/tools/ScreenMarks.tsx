// What the agent does on its screen, drawn over the frame: a pointer that glides to each click and a press ring there; the field it
// types into outlined while it types.
import { MousePointer2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { Action } from '@handline/app-data';

const SHOWN_MS = 1500;
const TYPES = new Set(['type', 'select', 'upload']);

/** `fit`: where the frame is drawn in the view (its offset and scale), so a box in frame units lands on the picture. */
export function ScreenMarks({ action, fit }: { action: Action | null; fit: { scale: number; left: number; top: number } }) {
  const [fresh, setFresh] = useState(0);
  useEffect(() => {
    if (!action) return;
    const t = setTimeout(() => setFresh(action.n), SHOWN_MS);
    return () => clearTimeout(t);
  }, [action]);
  if (!action) return null;
  const { box } = action;
  const at = {
    left: fit.left + box.x * fit.scale,
    top: fit.top + box.y * fit.scale,
    width: box.width * fit.scale,
    height: box.height * fit.scale,
  };
  const cx = at.left + at.width / 2;
  const cy = at.top + at.height / 2;
  const live = fresh !== action.n;
  return (
    <div className="pointer-events-none absolute inset-0 overflow-hidden" aria-hidden>
      {TYPES.has(action.kind) && live && <span className="absolute rounded-sm outline-2 outline-accent" style={at} />}
      <span className="absolute" style={{ left: cx, top: cy, transition: 'left 300ms ease-out, top 300ms ease-out' }}>
        {live && <span key={action.n} className="screen-ring absolute -top-3 -left-3 size-6 rounded-full border-2 border-accent" />}
        <MousePointer2 size={18} className="fill-bg text-fg" />
      </span>
    </div>
  );
}

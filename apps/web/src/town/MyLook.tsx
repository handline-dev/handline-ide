// A click on the founder, standing still in the city or an office, opens the looks it can take (look.ts): the pick shows at once and stays
// on this device. Over the picture as a page element at the founder's box, as the office's other targets are (Targets.tsx).
import { useState } from 'react';
import { founderBox, useFounder, type Founder, type Ground } from '#src/town/founder.tsx';
import { LOOKS, pickLook, useLookIndex, type Look } from '#src/town/look.ts';
import { Popover } from '#src/ui/Popover.tsx';

/** Screen pixels per pixel of the art in a look's picture: the sheets draw each art pixel six sheet pixels wide. */
const ART = 6;
const ZOOM = 2;

/** A look standing facing the viewer, its art pixels kept sharp. */
function Standing({ look }: { look: Look }) {
  const [cw, ch] = look.sheet.cell;
  const [w, h] = [(cw / ART) * ZOOM, (ch / ART) * ZOOM];
  return (
    <svg width={w} height={h} viewBox={`0 ${look.row * ch} ${cw} ${ch}`} aria-hidden style={{ imageRendering: 'pixelated' }}>
      <image href={look.sheet.url} width={look.sheet.size[0]} height={look.sheet.size[1]} />
    </svg>
  );
}

/** The founder's own target on a picture drawn `scale` times its pixels: shown only while it stands still out of its chair. */
export function MyLook({ founder, ground, scale }: { founder: Founder; ground: Ground; scale: number }) {
  const [open, setOpen] = useState(false);
  const picked = useLookIndex();
  const at = useFounder(founder, (me) => {
    if (!me || me.moving || me.seated) return null;
    const b = founderBox(ground, me.at);
    return [b.x, b.y, b.w, b.h].map((n) => Math.round(n * scale)).join(',');
  });
  if (!at && !open) return null;
  const [left, top, width, height] = (at ?? '0,0,0,0').split(',').map(Number);
  const target = (
    <button
      type="button"
      aria-label="You"
      title="You"
      style={{ left, top, width, height }}
      className="pointer-events-auto absolute cursor-pointer rounded-sm outline-accent hover:outline focus-visible:outline data-[popup-open]:outline"
    />
  );
  return (
    <Popover trigger={target} align="center" open={open} onOpenChange={setOpen}>
      <p className="-mt-1 mb-1.5 text-xs font-medium text-muted">Your look</p>
      <div className="-m-1.5 flex flex-wrap gap-1" role="group" aria-label="Your look">
        {LOOKS.map((look, i) => (
          <button
            key={i}
            type="button"
            aria-label={`Look ${i + 1}`}
            aria-pressed={i === picked}
            className={`flex items-end justify-center rounded-md px-1 pt-1 hover:bg-hover ${i === picked ? 'bg-hover outline outline-accent' : ''}`}
            onClick={() => {
              pickLook(i);
              setOpen(false);
            }}
          >
            <Standing look={look} />
          </button>
        ))}
      </div>
    </Popover>
  );
}

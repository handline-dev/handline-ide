// The city's picture (street.ts builds the city, scene3d.ts and shade.ts draw it) with what CityView.tsx needs to walk and click on it:
// the ground, each building's door, the building site, and where every tagged thing is in the picture. Drawn once for a layout and kept.
import { useMemo, useState } from 'react';
import { WebGLRenderer } from 'three';
import type { City } from '#src/town/city.ts';
import { leavesOf, type Leaves } from '#src/town/door.tsx';
import type { Ground } from '#src/town/founder.tsx';
import { groundOf } from '#src/town/ground.ts';
import { boxesOf, type Box } from '#src/town/people.ts';
import { DOOR } from '#src/town/room.ts';
import { drawRoom } from '#src/town/scene3d.ts';
import { shade } from '#src/town/shade.ts';
import { cityRoom, type Door } from '#src/town/street.ts';
import { nearestFree } from '#src/town/walk.ts';

export type Drawn = {
  url: string;
  w: number;
  h: number;
  ground: Ground;
  doors: (Door & { leaves: Leaves | null })[];
  /** Where the founder stands before the building site's board. */
  site: Door | null;
  boxes: Map<string, Box>;
};

let renderer: WebGLRenderer | null = null;
let kept: { sig: string; drawn: Drawn } | null = null;

/** The city drawn, again only when its layout, its windows or its cars change. */
export function useDrawn(city: City, cars: number): Drawn {
  const lots = city.lots.map((l) => `${l.key}:${l.x}:${l.y}:${l.w}:${l.h}:${l.empty}:${l.seats.join(',')}`).join(' ');
  const sig = `${city.w}x${city.h} ${cars} ${city.site.x}:${city.site.y} ${lots}`;
  const lay = () => ({ sig, city, cars });
  const [laid, setLaid] = useState(lay);
  if (laid.sig !== sig) setLaid(lay());
  return useMemo(() => drawCity(laid), [laid]);
}

/** The city for a layout: the kept drawing when it shows the same, else drawn now and kept. */
function drawCity({ sig, city, cars }: { sig: string; city: City; cars: number }): Drawn {
  if (kept?.sig === sig) return kept.drawn;
  const room = cityRoom(city, cars);
  renderer ??= new WebGLRenderer({ antialias: false, alpha: true });
  const frame = drawRoom(renderer, room);
  const pic = shade(frame);
  const canvas = document.createElement('canvas');
  [canvas.width, canvas.height] = [pic.w, pic.h];
  canvas.getContext('2d')?.putImageData(new ImageData(pic.rgba, pic.w, pic.h), 0, 0);
  const ground = groundOf(pic, frame, room, { speed: 2.4, stride: 0.2, grid: { cell: 0.1, body: 0.15, top: 0 } });
  // A doorway's spot is in the building's front, so the founder walks to the nearest it can stand on.
  const doors = room.doors.map((d) => ({
    ...d,
    in: nearestFree(ground.floor, d.in) ?? d.at,
    leaves: leavesOf(room.blocks, `${DOOR}:${d.key}`, 'left'),
  }));
  const drawn = { url: canvas.toDataURL(), w: pic.w, h: pic.h, ground, doors, site: room.site, boxes: boxesOf(pic, frame) };
  kept = { sig, drawn };
  return drawn;
}
